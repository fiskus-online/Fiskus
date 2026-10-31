package postman

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
)

// fakeSAPI je prístupový bod podľa SAPI-SK 1.0 (dokumentácia ePošťák, Verteco).
type fakeSAPI struct {
	mu       sync.Mutex
	tokens   int
	expire   bool // ďalšia požiadavka s tokenom dostane 401
	sent     []map[string]any
	keys     map[string]string // Idempotency-Key → providerDocumentId
	inbox    map[string]string // documentId → UBL
	acked    []string
	reject   bool
	badLogin bool
	invalid  bool // validátor nájde chybu
	validate int  // počet kontrol
}

func (f *fakeSAPI) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.expire {
			f.expire = false
			http.Error(w, `{"error":{"code":"TOKEN_EXPIRED","message":"token vypršal"}}`, 401)
			return false
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer tok-") {
			http.Error(w, `{"error":{"code":"UNAUTHORIZED"}}`, 401)
			return false
		}
		if r.Header.Get("X-Peppol-Participant-Id") != "0245:2120000003" {
			http.Error(w, `{"error":{"code":"FORBIDDEN","message":"cudzí účastník"}}`, 403)
			return false
		}
		return true
	}
	mux.HandleFunc("POST /sapi/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.badLogin || r.Header.Get("Content-Type") != "application/json" || in["grant_type"] != "client_credentials" || in["client_secret"] != "tajne" {
			http.Error(w, `{"error":{"code":"INVALID_CLIENT"}}`, 401)
			return
		}
		f.tokens++
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok-" + strings.Repeat("x", f.tokens), "token_type": "Bearer", "expires_in": 900})
	})
	mux.HandleFunc("POST /sapi/v1/document/send", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.mu.Lock()
		defer f.mu.Unlock()
		key := r.Header.Get("Idempotency-Key")
		if len(key) != 36 {
			http.Error(w, `{"error":{"code":"IDEMPOTENCY_KEY_REQUIRED"}}`, 400)
			return
		}
		w.WriteHeader(202)
		if f.reject {
			_ = json.NewEncoder(w).Encode(map[string]any{"providerDocumentId": "x", "status": "REJECTED",
				"messages": []any{map[string]any{"code": "BR-CO-15", "message": "Súčet nesedí"}}})
			return
		}
		id, dup := f.keys[key]
		if !dup {
			id = "prov-" + key[:8]
			f.keys[key] = id
			f.sent = append(f.sent, in)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"providerDocumentId": id, "status": "ACCEPTED"})
	})
	mux.HandleFunc("POST /sapi/v1/document/validate", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.validate++
		if f.invalid {
			_, _ = io.WriteString(w, `{"valid":false,"errors":["BR-CO-15: Invoice total amount with VAT (BT-112) nesedí","PEPPOL-EN16931-R001: chýba BT-23"],"warnings":[]}`)
			return
		}
		_, _ = io.WriteString(w, `{"valid":true,"errors":[],"warnings":["drobnosť"]}`)
	})
	mux.HandleFunc("GET /sapi/v1/document/sent/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		switch r.PathValue("id") {
		case "prov-doruc":
			_, _ = io.WriteString(w, `{"providerDocumentId":"prov-doruc","status":"DELIVERED","sentAt":"2027-01-04T08:59:00Z","deliveredAt":"2027-01-04T09:00:00Z"}`)
		case "prov-zle":
			_, _ = io.WriteString(w, `{"document":{"status":"undeliverable","messages":[{"message":"príjemca nie je v sieti Peppol"}]}}`)
		default:
			http.Error(w, `{"error":{"code":"NOT_FOUND"}}`, 404)
		}
	})
	mux.HandleFunc("GET /sapi/v1/document/receive", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.URL.Query().Get("status") != "RECEIVED" {
			t.Errorf("stav %q", r.URL.Query().Get("status"))
		}
		// po jednom na stranu, nech sa overí stránkovanie
		var ids []string
		for id := range f.inbox {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		start := 0
		if p := r.URL.Query().Get("pageToken"); p != "" {
			for i, id := range ids {
				if id == p {
					start = i
				}
			}
		}
		out := map[string]any{"documents": []any{}}
		if start < len(ids) {
			out["documents"] = []any{map[string]any{"documentId": ids[start], "senderParticipantId": "0245:1234567890", "creationDateTime": "2026-12-01T08:00:00Z"}}
			if start+1 < len(ids) {
				out["nextPageToken"] = ids[start+1]
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("GET /sapi/v1/document/receive/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{"documentId": r.PathValue("id")}, "payload": f.inbox[r.PathValue("id")], "payloadFormat": "XML"})
	})
	mux.HandleFunc("POST /sapi/v1/document/receive/{id}/acknowledge", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.acked = append(f.acked, r.PathValue("id"))
		delete(f.inbox, r.PathValue("id"))
		_, _ = io.WriteString(w, `{"status":"ACKNOWLEDGED"}`)
	})
	return mux
}

func TestSAPI(t *testing.T) {
	f := &fakeSAPI{keys: map[string]string{}, inbox: map[string]string{"d1": "<Invoice>1</Invoice>", "d2": "<Invoice>2</Invoice>"}}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	set := Settings{Mode: ModeSAPI, URL: srv.URL + "/sapi/v1/", ClientID: "klient", Token: "tajne", Participant: "0245:2120000003"}
	set.Normalize()
	p, err := New(set)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	inv := Outgoing{Sender: "0245:2120000003", Receiver: "0245:1234567890", FileName: "FA1.xml", Data: []byte(`<?xml version="1.0"?><Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"/>`)}
	cn := Outgoing{Sender: inv.Sender, Receiver: inv.Receiver, FileName: "DB1.xml", Data: []byte(`<?xml version="1.0"?><CreditNote xmlns="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"/>`)}

	s1, err := p.Send(ctx, inv)
	if err != nil || s1.MessageID == "" || s1.Status != "accepted" {
		t.Fatalf("odoslanie: %+v %v", s1, err)
	}
	// rovnaký dokument znova (napr. po výpadku) sa nezdvojí
	if s2, err := p.Send(ctx, inv); err != nil || s2.MessageID != s1.MessageID {
		t.Fatalf("opakované odoslanie: %+v %v", s2, err)
	}
	// vypršaný token: nový a požiadavka znova
	f.expire = true
	if _, err := p.Send(ctx, cn); err != nil {
		t.Fatalf("po vypršaní tokenu: %v", err)
	}
	if len(f.sent) != 2 || f.tokens != 2 {
		t.Fatalf("odoslané %d, tokenov %d", len(f.sent), f.tokens)
	}
	meta := func(i int) map[string]any { return f.sent[i]["metadata"].(map[string]any) }
	if !strings.Contains(meta(0)["documentTypeId"].(string), "Invoice-2::Invoice##") || !strings.Contains(meta(1)["documentTypeId"].(string), "CreditNote-2::CreditNote##") ||
		meta(0)["processId"] != sapiProcess || meta(0)["receiverParticipantId"] != "0245:1234567890" || f.sent[0]["payload"] != string(inv.Data) || f.sent[0]["payloadFormat"] != "XML" {
		t.Errorf("telo: %v", f.sent[0])
	}
	// zamietnutie poskytovateľom
	f.reject = true
	if _, err := p.Send(ctx, Outgoing{Sender: inv.Sender, Receiver: inv.Receiver, Data: []byte("<Invoice>iná</Invoice>")}); err == nil || !strings.Contains(err.Error(), "Súčet nesedí") {
		t.Errorf("zamietnutie: %v", err)
	}
	f.reject = false

	// schránka so stránkovaním, potvrdenie
	msgs, err := p.Inbox(ctx)
	if err != nil || len(msgs) != 2 || string(msgs[1].Data) != "<Invoice>2</Invoice>" || msgs[0].Received.Year() != 2026 {
		t.Fatalf("schránka: %+v %v", msgs, err)
	}
	if err := p.Ack(ctx, "d1"); err != nil || len(f.acked) != 1 {
		t.Fatalf("potvrdenie: %v", err)
	}
	if msgs, _ := p.Inbox(ctx); len(msgs) != 1 {
		t.Errorf("po potvrdení %d", len(msgs))
	}

	// zlé prihlasovacie údaje — zrozumiteľná chyba, tajomstvo sa v nej neobjaví
	bad := set
	bad.Token = "zle-tajne"
	pb, _ := New(bad)
	if _, err := pb.Inbox(ctx); err == nil || !strings.Contains(err.Error(), "prihlasovacie údaje") || strings.Contains(err.Error(), "zle-tajne") {
		t.Errorf("zlé údaje: %v", err)
	}
	// bez DIČ firmy sa poštár nevytvorí
	nodic := set
	nodic.Participant = ""
	if _, err := New(nodic); err == nil {
		t.Error("bez DIČ")
	}
}

// Poskytovateľ s validátorom a stavom doručenia (Verteco).
func TestSAPIValidateAndDelivery(t *testing.T) {
	f := &fakeSAPI{keys: map[string]string{}, inbox: map[string]string{}}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	p := SAPI{BaseURL: srv.URL + "/sapi/v1", ClientID: "k", ClientSecret: "tajne", Participant: "0245:2120000003"}
	ctx := context.Background()
	inv := Outgoing{Sender: p.Participant, Receiver: "0245:1234567890", Data: []byte("<Invoice>v</Invoice>")}
	f.invalid = true
	_, err := p.Send(ctx, inv)
	var ie *InvalidError
	if !errors.As(err, &ie) || len(ie.Rules) != 2 || !strings.HasPrefix(ie.Rules[0], "BR-CO-15") || len(f.sent) != 0 {
		t.Fatalf("neplatná e-faktúra sa nemá odoslať: %v (odoslaných %d)", err, len(f.sent))
	}
	f.invalid = false
	if _, err := p.Send(ctx, inv); err != nil || len(f.sent) != 1 || f.validate != 2 {
		t.Fatalf("platná: %v, odoslaných %d, kontrol %d", err, len(f.sent), f.validate)
	}
	for _, tt := range []struct {
		id, status, delivered, errText string
		err                            error
	}{
		{"prov-doruc", "delivered", "2027-01-04T09:00:00Z", "", nil},
		{"prov-zle", "undeliverable", "", "príjemca nie je v sieti Peppol", nil},
		{"neznamy", "", "", "", ErrNoTracking},
	} {
		d, err := p.Delivery(ctx, tt.id)
		if !errors.Is(err, tt.err) || d.Status != tt.status || d.Delivered != tt.delivered || d.Error != tt.errText {
			t.Errorf("%s: %+v %v", tt.id, d, err)
		}
	}
}

func TestIdempotencyKey(t *testing.T) {
	a := idempotencyKey(Outgoing{Sender: "s", Receiver: "r", Data: []byte("x")})
	b := idempotencyKey(Outgoing{Sender: "s", Receiver: "r", Data: []byte("y")})
	if len(a) != 36 || a[14] != '5' || a == b || a != idempotencyKey(Outgoing{Sender: "s", Receiver: "r", Data: []byte("x")}) {
		t.Errorf("%s %s", a, b)
	}
}
