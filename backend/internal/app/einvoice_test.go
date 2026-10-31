package app

import (
	"context"
	"errors"
	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/postman"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/ubl"
)

func TestEInvoiceExchange(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	c.DIC, c.Street, c.City, c.Zip, c.IBAN = "2020372640", "Hlavná 1", "Zvolen", "960 01", "SK3112000000198742637541"
	if err := a.Store.UpdateCompany(ctx, c); err != nil {
		t.Fatal(err)
	}
	d := validDraft()
	d.Customer = invoice.Party{Name: "Odberateľ s.r.o.", City: "Bratislava", Country: "SK", DIC: "2120000003"}
	inv, err := a.SaveDraft(ctx, c.ID, d)
	if err != nil {
		t.Fatal(err)
	}
	if inv, err = a.Issue(ctx, c.ID, inv.ID); err != nil {
		t.Fatal(err)
	}

	// bez nastavenia: e-faktúra sa iba stiahne
	if _, err := a.SendEInvoice(ctx, c.ID, inv.ID); !errors.Is(err, postman.ErrOff) {
		t.Fatalf("vypnuté: %v", err)
	}
	dir := t.TempDir()
	set := postman.Settings{Mode: postman.ModeFolder, OutDir: filepath.Join(dir, "von"), InDir: filepath.Join(dir, "dnu"), Provider: "Môj poštár"}
	if _, err := a.SetEInvoiceSettings(ctx, c.ID, postman.Settings{Mode: postman.ModeFolder}); err == nil {
		t.Fatal("neúplné nastavenie")
	}
	if _, err := a.SetEInvoiceSettings(ctx, c.ID, set); err != nil {
		t.Fatal(err)
	}

	// odoslanie
	e, err := a.SendEInvoice(ctx, c.ID, inv.ID)
	if err != nil || e.PeppolID != "0245:2120000003" || e.MessageID == "" || e.Error != "" {
		t.Fatalf("odoslanie: %+v %v", e, err)
	}
	if _, err := os.Stat(filepath.Join(set.OutDir, e.MessageID)); err != nil {
		t.Fatal("súbor v priečinku odoslaných chýba")
	}
	if list, _ := a.EInvoices(ctx, c.ID, store.EInvoiceFilter{Direction: "out", DocID: inv.ID}); len(list) != 1 {
		t.Fatalf("zoznam odoslaných %+v", list)
	}

	// prijatie: e-faktúra od dodávateľa v schránke
	xml, err := ubl.Write(inv, ubl.Seller{Name: "Dodávateľ s.r.o.", Street: "Dlhá 2", City: "Žilina", ICO: "36631124", DIC: "2021774654",
		ICDPH: "SK2021774654", IBAN: "SK3112000000198742637541", VATPayer: true})
	if err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(set.InDir, 0o755)
	_ = os.WriteFile(filepath.Join(set.InDir, "od-dodavatela.xml"), xml, 0o644)
	_ = os.WriteFile(filepath.Join(set.InDir, "pokazena.xml"), []byte("<html/>"), 0o644)
	box, err := a.EInvoiceInbox(ctx, c.ID)
	if err != nil || len(box) != 2 {
		t.Fatalf("schránka %+v %v", box, err)
	}
	var good InboxItem
	for _, it := range box {
		if it.ID == "od-dodavatela.xml" {
			good = it
		} else if it.Error == "" {
			t.Error("nečitateľný súbor bez chyby")
		}
	}
	if good.Supplier != "Dodávateľ s.r.o." || good.Number != inv.Number || good.Total != "492.00" {
		t.Fatalf("prehľad %+v", good)
	}
	draft, err := a.TakeEInvoice(ctx, c.ID, "od-dodavatela.xml")
	if err != nil || draft.Purchase.Supplier.ICO != "36631124" || draft.Purchase.Summary.Total.String() != "492.00" {
		t.Fatalf("prevzatie: %+v %v", draft.Purchase, err)
	}
	if box, _ := a.EInvoiceInbox(ctx, c.ID); len(box) != 1 {
		t.Fatal("prevzatá zostala v schránke")
	}
	in, _ := a.EInvoices(ctx, c.ID, store.EInvoiceFilter{Direction: "in"})
	if len(in) != 1 || in[0].Partner != "Dodávateľ s.r.o." || in[0].PeppolID != "0245:2021774654" {
		t.Fatalf("zoznam prijatých %+v", in)
	}
	again, err := a.ReopenEInvoice(ctx, c.ID, in[0].ID)
	if err != nil || again.Purchase.SupplierNumber != inv.Number {
		t.Fatalf("znova otvoriť: %v", err)
	}
	if _, err := a.TakeEInvoice(ctx, c.ID, "neexistuje.xml"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	// token sa nevracia
	if s, _ := a.EInvoiceSettings(ctx, c.ID); s.Token != "" || s.Provider != "Môj poštár" {
		t.Fatalf("%+v", s)
	}
}

// Na serveri sú priečinky disk servera — nastaví ich iba správca inštalácie.
func TestEInvoiceFolderOnServer(t *testing.T) {
	o := newOffice(t)
	dir := t.TempDir()
	set := postman.Settings{Mode: postman.ModeFolder, OutDir: filepath.Join(dir, "a"), InDir: filepath.Join(dir, "b")}
	ctxAdmin := as(o.admin)
	u, err := o.a.AddMember(ctxAdmin, o.alfa.ID, NewMember{Email: "spravca@alfa.sk", Name: "Správca Alfa", Password: "spravca-heslo-1", Role: auth.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	full, _ := o.a.Store.GetUser(ctx, u.User.ID)
	firmAdmin := actorOf(full, auth.Actor{Channel: auth.ChannelWeb})
	if _, err := o.a.SetEInvoiceSettings(as(firmAdmin), o.alfa.ID, set); !errors.Is(err, ErrForbidden) {
		t.Fatalf("správca firmy nastavil priečinky servera: %v", err)
	}
	api := postman.Settings{Mode: postman.ModeHTTP, URL: "https://api.postar.sk", Token: "t"}
	if _, err := o.a.SetEInvoiceSettings(as(firmAdmin), o.alfa.ID, api); err != nil {
		t.Fatalf("API poštára môže nastaviť správca firmy: %v", err)
	}
	if _, err := o.a.SetEInvoiceSettings(ctxAdmin, o.alfa.ID, set); err != nil {
		t.Fatal(err)
	}
}

// SAPI-SK: participant je 0245:DIČ firmy, overenie spojenia nič neprevezme.
func TestEInvoiceSAPI(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	var got postman.Settings
	a.Postman = func(s postman.Settings) (postman.Provider, error) {
		got = s
		return postman.New(s)
	}
	if err := a.CheckEInvoice(ctx, c.ID); !errors.Is(err, postman.ErrOff) {
		t.Fatalf("vypnuté: %v", err)
	}
	set := postman.Settings{Mode: postman.ModeSAPI, URL: "https://127.0.0.1:1/sapi/v1", ClientID: "id", Token: "tajne", Provider: "ePošťák"}
	if _, err := a.SetEInvoiceSettings(ctx, c.ID, set); !fieldErr(err, "url") {
		// https na 127.0.0.1 je platná adresa; overenie zlyhá až pri spojení
		if err != nil {
			t.Fatal(err)
		}
	}
	pub, _ := a.EInvoiceSettings(ctx, c.ID)
	if pub.Token != "" || !pub.TokenSet || pub.ClientID != "id" {
		t.Fatalf("verejné nastavenie %+v", pub)
	}
	if err := a.CheckEInvoice(ctx, c.ID); err == nil || !strings.Contains(err.Error(), "DIČ") {
		t.Errorf("bez DIČ: %v", err)
	}
	c.DIC = "2120000003"
	if err := a.Store.UpdateCompany(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := a.CheckEInvoice(ctx, c.ID); err == nil || !strings.Contains(err.Error(), "neodpovedá") {
		t.Errorf("spojenie: %v", err)
	}
	if got.Participant != "0245:2120000003" {
		t.Errorf("participant %q (DIČ %q)", got.Participant, c.DIC)
	}
}

// trackingPostman je poštár, ktorý vie povedať stav doručenia.
type trackingPostman struct {
	states map[string]postman.Delivery
	asked  int
}

func (p *trackingPostman) Send(context.Context, postman.Outgoing) (postman.Sent, error) {
	return postman.Sent{MessageID: "m" + strconv.Itoa(len(p.states)+1), Status: "accepted"}, nil
}
func (p *trackingPostman) Inbox(context.Context) ([]postman.Message, error) { return nil, nil }
func (p *trackingPostman) Ack(context.Context, string) error                { return nil }
func (p *trackingPostman) Delivery(_ context.Context, id string) (postman.Delivery, error) {
	p.asked++
	d, ok := p.states[id]
	if !ok {
		return postman.Delivery{}, postman.ErrNoTracking
	}
	return d, nil
}

func TestEInvoiceTracking(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	c.DIC = "2020372640"
	_ = a.Store.UpdateCompany(ctx, c)
	tp := &trackingPostman{states: map[string]postman.Delivery{}}
	a.Postman = func(postman.Settings) (postman.Provider, error) { return tp, nil }
	if _, err := a.SetEInvoiceSettings(ctx, c.ID, postman.Settings{Mode: postman.ModeSAPI, URL: "https://p.example/sapi", ClientID: "k", Token: "t"}); err != nil {
		t.Fatal(err)
	}
	add := func(msg, status string, created time.Time) store.EInvoice {
		e := store.EInvoice{Direction: "out", DocNumber: "FA-" + msg, MessageID: msg, Status: status, Created: created}
		if err := a.Store.AddEInvoice(ctx, c.ID, &e); err != nil {
			t.Fatal(err)
		}
		return e
	}
	now := a.now()
	ok := add("m1", "accepted", now)
	bad := add("m2", "accepted", now)
	old := add("m3", "accepted", now.AddDate(0, 0, -40))
	done := add("m4", "delivered", now)
	tp.states["m1"] = postman.Delivery{Status: "delivered", Delivered: "2027-01-04T09:00:00Z"}
	tp.states["m2"] = postman.Delivery{Status: "undeliverable", Error: "príjemca nie je v sieti"}
	tp.states["m3"] = postman.Delivery{Status: "delivered"}
	tp.states["m4"] = postman.Delivery{Status: "delivered"}

	got, err := a.TrackEInvoice(ctx, c.ID, ok.ID)
	if err != nil || got.Status != "delivered" || got.Delivered != "2027-01-04T09:00:00Z" {
		t.Fatalf("ručne: %+v %v", got, err)
	}
	tp.asked = 0
	a.TrackAllEInvoices(ctx)
	// m1 a m4 sú doručené, m3 je staršia ako 30 dní — pýta sa iba na m2
	if tp.asked != 1 {
		t.Errorf("otázok %d", tp.asked)
	}
	if e, _ := a.Store.GetEInvoice(ctx, c.ID, bad.ID); e.Status != "undeliverable" {
		t.Errorf("nedoručiteľná: %+v", e)
	}
	if e, _ := a.Store.GetEInvoice(ctx, c.ID, old.ID); e.Status != "accepted" {
		t.Errorf("stará: %+v", e)
	}
	_ = done
	// prijatá e-faktúra stav nemá
	in := store.EInvoice{Direction: "in", MessageID: "x", Created: now}
	_ = a.Store.AddEInvoice(ctx, c.ID, &in)
	if _, err := a.TrackEInvoice(ctx, c.ID, in.ID); !errors.Is(err, ErrNoTrack) {
		t.Errorf("prijatá: %v", err)
	}
	// nedoručenie je v audite
	entries, _, _ := a.Store.ListAudit(ctx, audit.Filter{Category: audit.CatInvoices})
	found := false
	for _, e := range entries {
		found = found || strings.Contains(e.Summary, "nebola doručená")
	}
	if !found {
		t.Error("nedoručenie chýba v audite")
	}
}
