package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/store"
	"github.com/fiskus-online/fiskus/backend/internal/store/memstore"
)

func TestSyncWritable(t *testing.T) {
	cases := []struct {
		pattern   string
		writable  bool
		kind      string
		localOnly bool
	}{
		{"POST /api/companies/{cid}/household/entries", true, "household/entries", false},
		{"POST /api/companies/{cid}/household/obligations/{id}/pay", true, "", false},
		{"POST /api/companies/{cid}/contacts", true, "contacts", false},
		{"PUT /api/companies/{cid}/contacts/{id}", false, "", false},
		{"DELETE /api/companies/{cid}/household/entries/{id}", false, "", false},
		{"POST /api/companies/{cid}/invoices/{id}/issue", false, "", false},
		{"POST /api/companies/{cid}/invoices/preview", false, "", true},
		{"POST /api/calc/{kind}", false, "", true},
		{"POST /api/browser/backup", false, "", true},
		{"POST /api/sync/run", false, "", true},
	}
	for _, c := range cases {
		kind, ok := SyncWritable(c.pattern)
		if ok != c.writable || kind != c.kind || SyncLocalOnly(c.pattern) != c.localOnly {
			t.Errorf("%s: %v %q %v", c.pattern, ok, kind, SyncLocalOnly(c.pattern))
		}
	}
}

func TestRemapRefs(t *testing.T) {
	ids := map[string]int64{"k1": 77}
	cases := []struct {
		name string
		body string
		refs map[string]string
		want string
		err  bool
	}{
		{"bez odkazov", `{"contactId":5}`, nil, `{"contactId":5}`, false},
		{"kontakt z telefónu", `{"amount":"-1","contactId":5}`, map[string]string{"contactId": "k1"}, `{"amount":"-1","contactId":77}`, false},
		{"kontakt, ktorý počítač neprijal", `{"contactId":5}`, map[string]string{"contactId": "k2"}, "", true},
	}
	for _, c := range cases {
		got, err := remapRefs(json.RawMessage(c.body), c.refs, ids)
		if c.err != (err != nil) || (!c.err && string(got) != c.want) {
			t.Errorf("%s: %s %v", c.name, got, err)
		}
	}
}

func TestFindRefs(t *testing.T) {
	outbox := []phoneItem{
		{SyncItem: SyncItem{ID: "k1", Kind: "contacts", LocalID: 9}},
		{SyncItem: SyncItem{ID: "p1", Kind: "household/entries", LocalID: 9}},
	}
	cases := []struct {
		body string
		want map[string]string
	}{
		{`{"contactId":9}`, map[string]string{"contactId": "k1"}},
		{`{"contactId":3}`, nil}, // kontakt z kópie počítača
		{`{"payee":"x"}`, nil},
		{`[1,2]`, nil},
	}
	for _, c := range cases {
		got := findRefs(json.RawMessage(c.body), outbox)
		if len(got) != len(c.want) || got["contactId"] != c.want["contactId"] {
			t.Errorf("%s: %v", c.body, got)
		}
	}
}

type stateMap map[string]string

func (m stateMap) Get(_ context.Context, k string) (string, error) {
	v, ok := m[k]
	if !ok {
		return "", store.ErrNotFound
	}
	return v, nil
}
func (m stateMap) Set(_ context.Context, k, v string) error { m[k] = v; return nil }

type stageDB struct{}

func (stageDB) Export(context.Context) ([]byte, error)     { return nil, nil }
func (stageDB) StageRestore(context.Context, []byte) error { return nil }

// Po načítaní novej kópie sa záznamy, ktoré počítač ešte neprijal, zapíšu
// znova; nový záznam odkazujúci na znova zapísaný kontakt dostane odkaz na
// záznam fronty (nie číslo z kópie).
func TestPhoneReapply(t *testing.T) {
	ctx := auth.WithActor(context.Background(), auth.Local)
	a := New(memstore.New(), edition.ForBrowser())
	a.Browser, a.SyncState = stageDB{}, stateMap{}
	next := int64(40)
	var calls []string
	a.SyncPattern = func(method, path string) string {
		if strings.HasSuffix(path, "/contacts") {
			return "POST /api/companies/{cid}/contacts"
		}
		return "POST /api/companies/{cid}/household/entries"
	}
	a.Replay = func(_ context.Context, _ auth.Actor, method, path string, body []byte) (int, []byte) {
		next++
		calls = append(calls, path+" "+string(body))
		b, _ := json.Marshal(map[string]int64{"id": next})
		return 201, b
	}
	if _, err := a.SyncJoin(ctx, "pripojit&k=AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE&n=PC"); err != nil {
		t.Fatal(err)
	}
	if !a.SyncIsPhone(ctx) {
		t.Fatal("telefón s kópiou")
	}
	// kontakt zapísaný v kópii pod id 5
	if err := a.SyncCapture(ctx, "POST /api/companies/{cid}/contacts", "POST", "/api/companies/1/contacts", []byte(`{"name":"Škola"}`), []byte(`{"id":5}`)); err != nil {
		t.Fatal(err)
	}
	// nová kópia z počítača (kontakt ešte neprijal)
	_ = a.Store.SetSetting(ctx, SyncSnapshotKey, "1000")
	if err := a.SyncPhoneStart(ctx); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 {
		t.Fatalf("znova zapísané: %v", calls)
	}
	// pohyb s kontaktom pod novým id 41
	if err := a.SyncCapture(ctx, "POST /api/companies/{cid}/household/entries", "POST", "/api/companies/1/household/entries", []byte(`{"contactId":41}`), []byte(`{"id":42}`)); err != nil {
		t.Fatal(err)
	}
	ps, _, _ := a.loadPhone(ctx)
	if len(ps.Outbox) != 2 || ps.Version != 1000 || ps.Outbox[0].LocalID != 41 || ps.Outbox[1].Refs["contactId"] != ps.Outbox[0].ID {
		t.Fatalf("fronta: %+v", ps.Outbox)
	}
	// odpojenie: znova samostatný
	if err := a.SyncLeave(ctx); err != nil || a.SyncIsPhone(ctx) {
		t.Fatalf("odpojenie: %v", err)
	}
}

func TestSyncTitle(t *testing.T) {
	cases := []struct {
		kind, pattern, body, want string
	}{
		{"household/entries", "POST /api/companies/{cid}/household/entries", `{"payee":"Pekáreň","amount":"-7.90"}`, "Pohyb v domácnosti: Pekáreň -7,90 €"},
		{"contacts", "POST /api/companies/{cid}/contacts", `{"name":"ZŠ Moyzesova"}`, "Nový kontakt: ZŠ Moyzesova"},
		{"", "POST /api/companies/{cid}/household/obligations/{id}/pay", `{"amount":"32.40"}`, "Úhrada záväzku: 32,40 €"},
		{"invoices", "POST /api/companies/{cid}/invoices", `{"lines":[]}`, "Koncept faktúry"},
	}
	for _, c := range cases {
		if got := syncTitle(c.kind, c.pattern, []byte(c.body)); got != c.want {
			t.Errorf("%s: %q", c.want, got)
		}
	}
}
