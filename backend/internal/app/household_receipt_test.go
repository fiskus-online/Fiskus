package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/household"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

const groceryJSON = `{"returnValue":0,"receipt":{"receiptId":"O-11112222333344445555666677778888","receiptNumber":7,
 "issueDate":"27.10.2026 17:02:00","totalPrice":6.29,
 "items":[{"name":"ROZOK KLASIK","quantity":6,"vatRate":5,"price":0.60},{"name":"MLIEKO 1,5% 1L","quantity":1,"vatRate":5,"price":0.99},
  {"name":"SAMPON HEAD","quantity":1,"vatRate":23,"price":4.20},{"name":"ZLAVA","quantity":1,"vatRate":23,"price":-0.50},{"name":"TASKA","quantity":1,"vatRate":23,"price":1.00}],
 "organization":{"name":"Lidl Slovenská republika, v.o.s.","ico":"35793783"}}}`

func TestHouseholdReceiptQR(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, groceryJSON)
	}))
	defer srv.Close()
	a.EKasaURL = srv.URL
	h, _ := a.CreateCompany(ctx, store.Company{Name: "Domácnosť", Bookkeeping: store.BookkeepingHousehold})
	_, _ = a.SaveHouseholdAccount(ctx, h.ID, household.Account{Name: "Účet", Kind: household.Bank})
	// nová domácnosť má účet Hotovosť — bloček sa zapíše naň
	accs, _ := a.Store.ListHHAccounts(ctx, h.ID)
	var cash household.Account
	for _, x := range accs {
		if x.Kind == household.Cash {
			cash = x
		}
	}
	if cash.Name != "Hotovosť" {
		t.Fatalf("predvolený účet: %+v", accs)
	}

	if _, err := a.HouseholdReceiptQR(ctx, h.ID, "nie je bloček"); !fieldErr(err, "qr") {
		t.Fatalf("zlý QR: %v", err)
	}
	d, err := a.HouseholdReceiptQR(ctx, h.ID, "O-11112222333344445555666677778888")
	if err != nil {
		t.Fatal(err)
	}
	e := d.Entry
	if d.From != "ekasa" || e.AccountID != cash.ID || e.Date.String() != "2026-10-27" || !e.Amount.Equal(m.MustParse("-6.29")) ||
		e.Payee != "Lidl Slovenská republika, v.o.s." || len(e.Split) != 5 || e.Source != "ekasa:O-11112222333344445555666677778888" {
		t.Fatalf("návrh: %+v", d)
	}
	want := []string{"Potraviny", "Potraviny", "Drogéria a domácnosť", "Potraviny", "Potraviny"} // zľava a taška = kategória obchodu
	for i, w := range want {
		if e.Split[i].Category != w {
			t.Errorf("položka %d %s: %s, chcem %s", i, e.Split[i].Note, e.Split[i].Category, w)
		}
	}
	if e.Split[0].Quantity != "6" || !e.Split[0].Amount.Equal(m.MustParse("-0.60")) {
		t.Fatalf("položka: %+v", e.Split[0])
	}
	// používateľ preradí tašku do drogérie a uloží
	e.Split[4].Category = "Drogéria a domácnosť"
	saved, err := a.SaveHouseholdEntry(ctx, h.ID, e)
	if err != nil || saved.Category != "Drogéria a domácnosť" {
		t.Fatalf("uloženie: %+v %v", saved, err) // drogéria 5,20 > potraviny 1,09
	}
	// údaje bločku sa uložia s pohybom a pri úprave sa nestratia
	if saved.Receipt == nil || saved.Receipt.Seller.Name == "" || len(saved.Receipt.Items) != 5 {
		t.Fatalf("údaje bločku: %+v", saved.Receipt)
	}
	edit := saved
	edit.Receipt, edit.Note = nil, "nákup na víkend"
	if again, err := a.SaveHouseholdEntry(ctx, h.ID, edit); err != nil || again.Receipt == nil {
		t.Fatalf("úprava stratila bloček: %+v %v", again.Receipt, err)
	}
	if manual, err := a.SaveHouseholdEntry(ctx, h.ID, household.Entry{Date: invoice.MustDate("2025-09-01"), AccountID: cash.ID, Amount: m.MustParse("-5"), Category: "Potraviny", Receipt: saved.Receipt}); err != nil || manual.Receipt != nil {
		t.Fatalf("ručný pohyb bez bločku: %+v %v", manual.Receipt, err)
	}
	if err := a.SaveHouseholdPhoto(ctx, h.ID, saved.ID, "image/jpeg", []byte{0xff, 0xd8}); err != nil {
		t.Fatal(err)
	}
	if p, err := a.HouseholdPhoto(ctx, h.ID, saved.ID); err != nil || len(p.Data) != 2 {
		t.Fatalf("fotka: %v", err)
	}
	if err := a.SaveHouseholdPhoto(ctx, h.ID, saved.ID, "text/plain", []byte("x")); !fieldErr(err, "photo") {
		t.Fatalf("zlý typ fotky: %v", err)
	}
	// druhýkrát ten istý bloček
	if _, err := a.HouseholdReceiptQR(ctx, h.ID, "O-11112222333344445555666677778888"); !fieldErr(err, "qr") {
		t.Fatalf("duplicitný bloček: %v", err)
	}
	dup := e
	dup.ID = 0
	if _, err := a.SaveHouseholdEntry(ctx, h.ID, dup); !fieldErr(err, "source") {
		t.Fatalf("duplicitné uloženie: %v", err)
	}
	// nabudúce si taška pamätá drogériu
	if c := household.SuggestItem("TASKA", []household.Entry{saved}, household.DefaultCategories); c != "Drogéria a domácnosť" {
		t.Fatalf("história položky: %q", c)
	}
	// prehľad počíta položky po kategóriách
	ov, _ := a.HouseholdSummary(ctx, h.ID, 2026, 10)
	for _, c := range ov.Categories {
		if c.Name == "Potraviny" && !c.Total.Equal(m.MustParse("1.09")) {
			t.Fatalf("potraviny %v", c.Total)
		}
	}
}

func TestHouseholdReceiptPhoto(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"content":[{"type":"tool_use","name":"zapis_blocek","input":{"store":"Kaufland","date":"2026-10-20","total":"12.40",
			"items":[{"name":"Kuracie prsia","amount":"7.90","category":"Potraviny"},{"name":"Prací gél","amount":"4.50","category":"Neexistujúca"}]}}]}`))
	}))
	defer srv.Close()
	a.AIBaseURL = srv.URL
	h, _ := a.CreateCompany(ctx, store.Company{Name: "Domácnosť", Bookkeeping: store.BookkeepingHousehold})
	_, _ = a.SaveHouseholdAccount(ctx, h.ID, household.Account{Name: "Účet"})
	jpeg := []byte{0xff, 0xd8, 0xff, 0xe0, 0, 0}
	if _, err := a.HouseholdReceiptPhoto(ctx, h.ID, jpeg); !fieldErr(err, "photo") {
		t.Fatalf("bez kľúča: %v", err)
	}
	t.Setenv("FISKUS_ANTHROPIC_API_KEY", "sk-env")
	d, err := a.HouseholdReceiptPhoto(ctx, h.ID, jpeg)
	if err != nil {
		t.Fatal(err)
	}
	if d.From != "photo" || d.Entry.Payee != "Kaufland" || len(d.Entry.Split) != 2 || d.Entry.Split[1].Category != "Drogéria a domácnosť" ||
		!d.Entry.Amount.Equal(m.MustParse("-12.40")) || d.Entry.Source == "" {
		t.Fatalf("návrh z fotky: %+v", d)
	}
}
