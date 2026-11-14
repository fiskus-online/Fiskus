package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/app"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/household"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/relay"
	"github.com/fiskus-online/fiskus/backend/internal/store"
	"github.com/fiskus-online/fiskus/backend/internal/store/memstore"
)

// kópia pre telefón v teste: iba verzia (memstore sa do súboru nedá uložiť)
type fakeSource struct {
	mu      sync.Mutex
	changes int64
}

func (f *fakeSource) PhoneSnapshot(_ context.Context, version int64) ([]byte, error) {
	return []byte("kopia " + strconv.FormatInt(version, 10)), nil
}
func (f *fakeSource) Changes(context.Context) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.changes, nil
}

type mapState struct{ m sync.Map }

func (s *mapState) Get(_ context.Context, k string) (string, error) {
	v, ok := s.m.Load(k)
	if !ok {
		return "", store.ErrNotFound
	}
	return v.(string), nil
}
func (s *mapState) Set(_ context.Context, k, v string) error { s.m.Store(k, v); return nil }

// domácnosť s účtom (rovnaké id v počítači aj na telefóne, ako po kópii)
func household3(t *testing.T, a *app.App) (int64, int64) {
	t.Helper()
	ctx := auth.WithActor(context.Background(), auth.Local)
	h, err := a.CreateCompany(ctx, store.Company{Name: "Domácnosť Novákovci", Bookkeeping: store.BookkeepingHousehold})
	if err != nil {
		t.Fatal(err)
	}
	acc, err := a.SaveHouseholdAccount(ctx, h.ID, household.Account{Name: "Účet", Kind: household.Bank, Opening: money.MustParse("500")})
	if err != nil {
		t.Fatal(err)
	}
	return h.ID, acc.ID
}

func jsonCall(t *testing.T, srv *httptest.Server, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var rd *strings.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	} else {
		rd = strings.NewReader("")
	}
	res, out := call(t, srv, method, path, "application/json", rd)
	return res.StatusCode, out
}

func TestPhoneSync(t *testing.T) {
	ctx := context.Background()
	box := relay.NewMemServer()
	rs := httptest.NewServer(box)
	defer rs.Close()

	desk, _ := edition.For(edition.Desktop)
	d := app.New(memstore.New(), desk)
	src := &fakeSource{}
	d.SyncSource, d.SyncRelayURL = src, rs.URL
	dsrv := httptest.NewServer((&Server{App: d}).Handler())
	defer dsrv.Close()
	hid, accID := household3(t, d)

	p := app.New(memstore.New(), edition.ForBrowser())
	pdb := &memBrowserDB{}
	p.Browser, p.SyncState, p.SyncRelayURL = pdb, &mapState{}, rs.URL
	psrv := httptest.NewServer((&Server{App: p}).Handler())
	defer psrv.Close()
	household3(t, p)

	// kontakt iba v počítači — id kontaktov na telefóne a v počítači sa rozídu
	if _, err := d.SaveContact(auth.WithActor(ctx, auth.Local), hid, store.Contact{Name: "Elektrárne"}); err != nil {
		t.Fatal(err)
	}

	// 1. spárovanie v počítači pošle prvú kópiu
	code, out := jsonCall(t, dsrv, "POST", "/api/sync/phones", map[string]string{"name": "Mobil"})
	if code != 201 {
		t.Fatalf("spárovanie: %d %v", code, out)
	}
	link := out["link"].(string)
	if !strings.HasPrefix(link, app.SyncAppURL+"/#pripojit&") {
		t.Fatalf("odkaz: %s", link)
	}
	// telefón v sieti nesmie spárovať ďalší telefón
	lanReq := httptest.NewRequest("POST", "/api/sync/phones", strings.NewReader(`{}`))
	if !lanDenied(lanReq) {
		t.Fatal("párovanie z telefónu v sieti musí byť zakázané")
	}

	// 2. telefón sa pripojí a stiahne kópiu
	if code, out := jsonCall(t, psrv, "POST", "/api/sync/join", map[string]string{"link": link}); code != 200 || out["role"] != "phone" {
		t.Fatalf("pripojenie: %d %v", code, out)
	}
	reload := func() {
		t.Helper()
		version := strings.TrimPrefix(string(pdb.staged), "kopia ")
		pctx := auth.WithActor(ctx, auth.Local)
		if err := p.Store.SetSetting(pctx, app.SyncSnapshotKey, version); err != nil {
			t.Fatal(err)
		}
		// výsledky prehratia idú v kópii z počítača
		_, frag, _ := strings.Cut(link, "#")
		pr, _ := relay.ParsePairing(frag)
		k, _ := relay.Derive(pr.Secret)
		if v, err := d.Store.GetSetting(pctx, "sync.applied."+k.Channel); err == nil {
			_ = p.Store.SetSetting(pctx, "sync.applied."+k.Channel, v)
		}
		if err := p.SyncPhoneStart(pctx); err != nil {
			t.Fatal(err)
		}
	}
	phone := func() map[string]any {
		t.Helper()
		code, out := jsonCall(t, psrv, "POST", "/api/sync/run", nil)
		if code != 200 || out["phone"] == nil {
			t.Fatalf("synchronizácia telefónu: %d %v", code, out)
		}
		return out["phone"].(map[string]any)
	}
	if st := phone(); st["staged"] != true || st["lastError"] != nil {
		t.Fatalf("kópia: %v", st)
	}
	reload()
	if st := phone(); st["staged"] != false || st["version"] == nil {
		t.Fatalf("po načítaní: %v", st)
	}

	// 3. kópia na telefóne: úprava účtu nie, nový kontakt a pohyb áno
	base := fmt.Sprintf("/api/companies/%d", hid)
	if code, out := jsonCall(t, psrv, "PUT", fmt.Sprintf("%s/household/accounts/%d", base, accID), map[string]any{"name": "Iný"}); code != 409 || !strings.Contains(fmt.Sprint(out["error"]), "v počítači") {
		t.Fatalf("úprava na telefóne: %d %v", code, out)
	}
	code, out = jsonCall(t, psrv, "POST", base+"/contacts", map[string]any{"name": "Škola"})
	if code != 201 {
		t.Fatalf("kontakt: %d %v", code, out)
	}
	localContact := int64(out["id"].(float64))
	entry := map[string]any{"date": "2026-11-13", "accountId": accID, "amount": "-12.50", "category": "Deti a škola", "payee": "Škola", "contactId": localContact}
	if code, out := jsonCall(t, psrv, "POST", base+"/household/entries", entry); code != 201 {
		t.Fatalf("pohyb: %d %v", code, out)
	}
	if st := phone(); st["pending"] != float64(2) {
		t.Fatalf("fronta: %v", st)
	}

	// 4. počítač prehrá frontu a pošle novú kópiu
	if err := d.SyncRun(auth.WithActor(ctx, auth.System)); err != nil {
		t.Fatal(err)
	}
	dctx := auth.WithActor(ctx, auth.Local)
	contacts, _ := d.Store.ListContacts(dctx, hid, "")
	var school int64
	for _, c := range contacts {
		if c.Name == "Škola" {
			school = c.ID
		}
	}
	if school == 0 || school == localContact {
		t.Fatalf("kontakt v počítači: %d (na telefóne %d) %+v", school, localContact, contacts)
	}
	entries, _ := d.HouseholdEntries(dctx, hid, 2026)
	if len(entries) != 1 || entries[0].ContactID != school || entries[0].Amount.String() != "-12.50" {
		t.Fatalf("pohyb v počítači: %+v", entries)
	}
	st, _ := d.SyncStatus(dctx)
	if len(st.Phones) != 1 || st.Phones[0].Received != 2 || st.Phones[0].LastError != "" {
		t.Fatalf("stav v počítači: %+v", st.Phones)
	}
	// telefón stiahne kópiu, záznamy sú vybavené
	if st := phone(); st["staged"] != true {
		t.Fatalf("nová kópia: %v", st)
	}
	reload()
	if st := phone(); st["pending"] != float64(0) || len(st["failed"].([]any)) != 0 {
		t.Fatalf("po prijatí: %v", st)
	}

	// 5. záznam, ktorý počítač odmietne (účet medzitým zmazaný)
	delete(entry, "contactId") // kópia v teste nie je skutočná kópia počítača
	if code, out := jsonCall(t, psrv, "POST", base+"/household/entries", entry); code != 201 {
		t.Fatalf("pohyb 2: %d %v", code, out)
	}
	phone()
	if err := d.DeleteHouseholdEntry(dctx, hid, entries[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteHouseholdAccount(dctx, hid, accID); err != nil {
		t.Fatal(err)
	}
	if err := d.SyncRun(auth.WithActor(ctx, auth.System)); err != nil {
		t.Fatal(err)
	}
	phone()
	reload()
	failed := phone()["failed"].([]any)
	if len(failed) != 1 || failed[0].(map[string]any)["error"] == "" {
		t.Fatalf("odmietnutý záznam: %v", failed)
	}
	id := failed[0].(map[string]any)["id"].(string)
	if code, out := jsonCall(t, psrv, "DELETE", "/api/sync/failed/"+id, nil); code != 200 || len(out["phone"].(map[string]any)["failed"].([]any)) != 0 {
		t.Fatalf("skrytie: %d %v", code, out)
	}

	// 6. zrušenie spárovania zmaže schránku
	st, _ = d.SyncStatus(dctx)
	if code, _ := jsonCall(t, dsrv, "DELETE", "/api/sync/phones/"+st.Phones[0].ID, nil); code != 200 {
		t.Fatalf("odobratie: %d", code)
	}
	if code, _ := jsonCall(t, psrv, "POST", "/api/sync/leave", nil); code != 204 {
		t.Fatalf("odpojenie: %d", code)
	}
	if code, _ := jsonCall(t, psrv, "PUT", fmt.Sprintf("%s/household/accounts/%d", base, accID), map[string]any{"name": "Iný"}); code == 409 {
		t.Fatal("po odpojení je Fiškus v prehliadači znova samostatný")
	}
}

// Bloček z telefónu: v kópii sa nespracuje (eKasa v prehliadači nejde), iba
// ide do fronty; počítač ho spracuje.
func TestPhoneReceiptQueued(t *testing.T) {
	ctx := context.Background()
	rs := httptest.NewServer(relay.NewMemServer())
	defer rs.Close()
	ek := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"returnValue":0,"receipt":{"receiptId":"O-11112222333344445555666677778888","issueDate":"27.10.2026 17:02:00","totalPrice":0.99,
 "items":[{"name":"MLIEKO 1L","quantity":1,"vatRate":5,"price":0.99}],"organization":{"name":"Potraviny Jana","ico":"35793783"}}}`))
	}))
	defer ek.Close()

	desk, _ := edition.For(edition.Desktop)
	d := app.New(memstore.New(), desk)
	d.SyncSource, d.SyncRelayURL, d.EKasaURL = &fakeSource{}, rs.URL, ek.URL
	dsrv := httptest.NewServer((&Server{App: d}).Handler())
	defer dsrv.Close()
	hid, accID := household3(t, d)

	p := app.New(memstore.New(), edition.ForBrowser())
	p.Browser, p.SyncState, p.SyncRelayURL = &memBrowserDB{}, &mapState{}, rs.URL
	psrv := httptest.NewServer((&Server{App: p}).Handler())
	defer psrv.Close()
	household3(t, p)

	path := fmt.Sprintf("/api/companies/%d/receipts/phone", hid)
	body := map[string]any{"qr": "O-11112222333344445555666677778888", "accountId": accID, "photo": []byte{0xff, 0xd8}}
	// samostatný Fiškus v prehliadači: iba v nainštalovanom (eKasa)
	if code, out := jsonCall(t, psrv, "POST", path, body); code != 501 || out["install"] == nil {
		t.Fatalf("bez spárovania: %d %v", code, out)
	}
	_, out := jsonCall(t, dsrv, "POST", "/api/sync/phones", map[string]string{"name": "Mobil"})
	if code, out := jsonCall(t, psrv, "POST", "/api/sync/join", map[string]string{"link": out["link"].(string)}); code != 200 {
		t.Fatalf("pripojenie: %d %v", code, out)
	}
	code, out := jsonCall(t, psrv, "POST", path, body)
	if code != 202 || out["queued"] != true {
		t.Fatalf("bloček na telefóne: %d %v", code, out)
	}
	pctx := auth.WithActor(ctx, auth.Local)
	if es, _ := p.HouseholdEntries(pctx, hid, 2026); len(es) != 0 {
		t.Fatalf("telefón bloček nespracuje: %+v", es)
	}
	_, out = jsonCall(t, psrv, "POST", "/api/sync/run", nil)
	st := out["phone"].(map[string]any)
	items := st["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["title"] != "Bloček: eKasa" || items[0].(map[string]any)["status"] != "sent" {
		t.Fatalf("fronta: %v", st)
	}
	if err := d.SyncRun(auth.WithActor(ctx, auth.System)); err != nil {
		t.Fatal(err)
	}
	es, _ := d.HouseholdEntries(pctx, hid, 2026)
	if len(es) != 1 || es[0].Payee != "Potraviny Jana" || es[0].AccountID != accID {
		t.Fatalf("bloček v počítači: %+v", es)
	}
	if ph, err := d.HouseholdPhoto(pctx, hid, es[0].ID); err != nil || len(ph.Data) != 2 {
		t.Fatalf("fotka v počítači: %v", err)
	}
}
