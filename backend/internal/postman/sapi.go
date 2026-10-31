package postman

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// SAPI je národné rozhranie SAPI-SK 1.0 — spoločný štandard slovenských
// prístupových bodov Peppol (ePošťák, Verteco…), takže Fiškus nie je
// viazaný na jedného poskytovateľa. Základná adresa sa líši podľa
// poskytovateľa (napr. https://epostak.sk/sapi/v1, https://peppol.verteco.digital/sapi):
//
//	POST {base}/auth/token                         client_credentials → access_token (15 min)
//	POST {base}/document/send                      Idempotency-Key, X-Peppol-Participant-Id → 202 providerDocumentId
//	GET  {base}/document/receive?status=RECEIVED   → documents[], nextPageToken
//	GET  {base}/document/receive/{id}              → metadata, payload (UBL XML)
//	POST {base}/document/receive/{id}/acknowledge
type SAPI struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	Participant  string // 0245:DIČ firmy, za ktorú sa posiela a prijíma
	Client       *http.Client
}

// Identifikátory dokumentu a procesu Peppol BIS Billing 3.0.
const (
	sapiInvoiceType = "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2::Invoice##urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0::2.1"
	sapiCreditType  = "urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2::CreditNote##urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0::2.1"
	sapiProcess     = "urn:fdc:peppol.eu:2017:poacc:billing:01:1.0"
)

// tokeny sa držia v pamäti (poskytovateľ sa vytvára pri každej akcii)
var sapiTokens = struct {
	sync.Mutex
	m map[string]sapiToken
}{m: map[string]sapiToken{}}

type sapiToken struct {
	value string
	exp   time.Time
}

func (s SAPI) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func (s SAPI) tokenKey() string {
	h := sha256.Sum256([]byte(s.BaseURL + "\x00" + s.ClientID + "\x00" + s.ClientSecret))
	return hex.EncodeToString(h[:])
}

// sapiError prečíta chybu {"error":{"code","message"}} (aj iné bežné tvary).
func sapiError(status string, data []byte) error {
	var e struct {
		Error any    `json:"error"`
		Msg   string `json:"message"`
	}
	_ = json.Unmarshal(data, &e)
	msg := e.Msg
	switch v := e.Error.(type) {
	case string:
		msg = v
	case map[string]any:
		if m, ok := v["message"].(string); ok && m != "" {
			msg = m
		} else if c, ok := v["code"].(string); ok {
			msg = c
		}
	}
	if msg == "" {
		msg = status
	}
	return fmt.Errorf("poštár: %s", msg)
}

func (s SAPI) token(ctx context.Context, fresh bool) (string, error) {
	key := s.tokenKey()
	sapiTokens.Lock()
	t, ok := sapiTokens.m[key]
	sapiTokens.Unlock()
	if ok && !fresh && time.Now().Before(t.exp) {
		return t.value, nil
	}
	body, _ := json.Marshal(map[string]string{"client_id": s.ClientID, "client_secret": s.ClientSecret, "grant_type": "client_credentials"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL+"/auth/token", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := s.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("poštár neodpovedá: %w", err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusBadRequest {
			return "", errors.New("poštár: neplatné prihlasovacie údaje API (client ID alebo tajomstvo)")
		}
		return "", sapiError(res.Status, data)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.AccessToken == "" {
		return "", errors.New("poštár: neplatná odpoveď na prihlásenie")
	}
	if out.ExpiresIn <= 0 {
		out.ExpiresIn = 900
	}
	// rezerva minúta, aby token nevypršal uprostred požiadavky
	exp := time.Now().Add(time.Duration(out.ExpiresIn)*time.Second - time.Minute)
	sapiTokens.Lock()
	sapiTokens.m[key] = sapiToken{out.AccessToken, exp}
	sapiTokens.Unlock()
	return out.AccessToken, nil
}

func (s SAPI) do(ctx context.Context, method, path string, body any, hdr map[string]string) ([]byte, error) {
	var raw []byte
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			return nil, err
		}
	}
	for attempt := 0; ; attempt++ {
		tok, err := s.token(ctx, attempt > 0)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, method, s.BaseURL+path, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Peppol-Participant-Id", s.Participant)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		res, err := s.client().Do(req)
		if err != nil {
			return nil, fmt.Errorf("poštár neodpovedá: %w", err)
		}
		data, err := io.ReadAll(io.LimitReader(res.Body, 20<<20))
		res.Body.Close()
		if err != nil {
			return nil, err
		}
		if res.StatusCode == http.StatusUnauthorized && attempt == 0 {
			continue // token mohol vypršať — raz nový
		}
		if res.StatusCode >= 300 {
			return nil, sapiError(res.Status, data)
		}
		return data, nil
	}
}

// idempotencyKey je odvodený z obsahu: opakované odoslanie toho istého
// dokumentu (napr. po výpadku spojenia) poskytovateľ nezdvojí.
func idempotencyKey(o Outgoing) string {
	h := sha256.Sum256(append([]byte(o.Sender+"\x00"+o.Receiver+"\x00"), o.Data...))
	h[6] = h[6]&0x0f | 0x50 // tvar UUID (verzia 5, variant RFC 4122)
	h[8] = h[8]&0x3f | 0x80
	x := hex.EncodeToString(h[:16])
	return x[:8] + "-" + x[8:12] + "-" + x[12:16] + "-" + x[16:20] + "-" + x[20:32]
}

// documentType rozlíši faktúru a dobropis podľa koreňového prvku UBL.
func documentType(data []byte) string {
	head := data
	if len(head) > 2048 {
		head = head[:2048]
	}
	if bytes.Contains(head, []byte("<CreditNote")) || bytes.Contains(head, []byte(":CreditNote ")) || bytes.Contains(head, []byte(":CreditNote>")) {
		return sapiCreditType
	}
	return sapiInvoiceType
}

func (s SAPI) Send(ctx context.Context, o Outgoing) (Sent, error) {
	key := idempotencyKey(o)
	body := map[string]any{
		"metadata": map[string]string{
			"documentId":            key,
			"documentTypeId":        documentType(o.Data),
			"processId":             sapiProcess,
			"senderParticipantId":   o.Sender,
			"receiverParticipantId": o.Receiver,
			"creationDateTime":      time.Now().UTC().Format(time.RFC3339),
		},
		"payload":         string(o.Data),
		"payloadFormat":   "XML",
		"payloadEncoding": "UTF-8",
		"checksum":        fmt.Sprintf("%x", sha256.Sum256(o.Data)),
	}
	data, err := s.do(ctx, http.MethodPost, "/document/send", body, map[string]string{"Idempotency-Key": key})
	if err != nil {
		return Sent{}, err
	}
	var out struct {
		ProviderDocumentID string `json:"providerDocumentId"`
		Status             string `json:"status"`
		Messages           []any  `json:"messages"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.ProviderDocumentID == "" {
		return Sent{}, errors.New("poštár: neplatná odpoveď na odoslanie")
	}
	if strings.EqualFold(out.Status, "REJECTED") {
		return Sent{}, fmt.Errorf("poštár e-faktúru odmietol: %s", messagesText(out.Messages))
	}
	return Sent{MessageID: out.ProviderDocumentID, Status: strings.ToLower(firstNonEmpty(out.Status, "accepted"))}, nil
}

// messagesText poskladá texty hlásení poskytovateľa (reťazce alebo {message}).
func messagesText(ms []any) string {
	var parts []string
	for _, m := range ms {
		switch v := m.(type) {
		case string:
			parts = append(parts, v)
		case map[string]any:
			if t, ok := v["message"].(string); ok {
				parts = append(parts, t)
			} else if t, ok := v["text"].(string); ok {
				parts = append(parts, t)
			}
		}
	}
	if len(parts) == 0 {
		return "bez udania dôvodu"
	}
	return strings.Join(parts, "; ")
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

type sapiMeta struct {
	DocumentID       string `json:"documentId"`
	SenderID         string `json:"senderParticipantId"`
	CreationDateTime string `json:"creationDateTime"`
}

func (s SAPI) Inbox(ctx context.Context) ([]Message, error) {
	list := []Message{}
	page := ""
	for i := 0; i < 50; i++ { // najviac 5 000 dokumentov naraz
		q := url.Values{"status": {"RECEIVED"}, "limit": {"100"}}
		if page != "" {
			q.Set("pageToken", page)
		}
		data, err := s.do(ctx, http.MethodGet, "/document/receive?"+q.Encode(), nil, nil)
		if err != nil {
			return nil, err
		}
		var res struct {
			Documents     []sapiMeta `json:"documents"`
			NextPageToken string     `json:"nextPageToken"`
		}
		if err := json.Unmarshal(data, &res); err != nil {
			return nil, errors.New("poštár: neplatný zoznam prijatých")
		}
		for _, d := range res.Documents {
			if d.DocumentID == "" {
				continue
			}
			m, err := s.get(ctx, d)
			if err != nil {
				return nil, err
			}
			list = append(list, m)
		}
		if res.NextPageToken == "" || res.NextPageToken == page {
			break
		}
		page = res.NextPageToken
	}
	return list, nil
}

func (s SAPI) get(ctx context.Context, d sapiMeta) (Message, error) {
	data, err := s.do(ctx, http.MethodGet, "/document/receive/"+url.PathEscape(d.DocumentID), nil, nil)
	if err != nil {
		return Message{}, err
	}
	var doc struct {
		Metadata sapiMeta `json:"metadata"`
		Payload  string   `json:"payload"`
	}
	if err := json.Unmarshal(data, &doc); err != nil || doc.Payload == "" {
		return Message{}, fmt.Errorf("poštár: prijatý dokument %s je prázdny", d.DocumentID)
	}
	when := firstNonEmpty(doc.Metadata.CreationDateTime, d.CreationDateTime)
	t, _ := time.Parse(time.RFC3339, when)
	if t.IsZero() {
		t = time.Now()
	}
	return Message{ID: d.DocumentID, Name: d.DocumentID + ".xml", Received: t, Data: []byte(doc.Payload)}, nil
}

func (s SAPI) Ack(ctx context.Context, id string) error {
	_, err := s.do(ctx, http.MethodPost, "/document/receive/"+url.PathEscape(id)+"/acknowledge", nil, nil)
	return err
}

// Check overí prihlasovacie údaje a prístup k schránke firmy (nič nemení).
func (s SAPI) Check(ctx context.Context) error {
	if _, err := s.token(ctx, true); err != nil {
		return err
	}
	_, err := s.do(ctx, http.MethodGet, "/document/receive?limit=1&status=RECEIVED", nil, nil)
	return err
}
