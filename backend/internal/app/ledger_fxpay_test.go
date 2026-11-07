package app

import (
	"errors"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/bank"
	"github.com/fiskus-online/fiskus/backend/internal/chart"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/fx"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

func TestFXPaymentBook(t *testing.T) {
	// faktúra 492 CZK = 19,68 € v účtovníctve
	inv := invoice.Invoice{Currency: "CZK", Summary: vat.Summary{Total: m.MustParse("492")},
		Local: &vat.Summary{Total: m.MustParse("19.68")}}
	pay := func(ref int64, date, amt string) invoice.Payment {
		return invoice.Payment{Date: invoice.MustDate(date), Amount: m.MustParse(amt), Source: invoice.PaymentBank, Ref: ref}
	}
	tests := []struct {
		name  string
		pays  []invoice.Payment
		tx    int64
		year  int
		rv    string // prepočet k 31. 12. minulého roka ("" = nie)
		want  string
		found bool
	}{
		{"celá úhrada", []invoice.Payment{pay(1, "2026-11-10", "492")}, 1, 2026, "", "19.68", true},
		{"prvá polovica", []invoice.Payment{pay(1, "2026-11-10", "246"), pay(2, "2026-11-20", "246")}, 1, 2026, "", "9.84", true},
		{"tretina a zvyšok", []invoice.Payment{pay(1, "2026-11-10", "164"), pay(2, "2026-11-20", "328")}, 2, 2026, "", "13.12", true},
		{"tri tretiny bez halierov navyše", []invoice.Payment{pay(1, "2026-11-01", "164"), pay(2, "2026-11-02", "164"), pay(3, "2026-11-03", "164")}, 3, 2026, "", "6.56", true},
		{"po prepočte k 31. 12.", []invoice.Payment{pay(1, "2026-12-10", "246"), pay(2, "2027-01-15", "246")}, 2, 2027, "10.25", "10.25", true},
		{"iný pohyb", []invoice.Payment{pay(1, "2026-11-10", "492")}, 9, 2026, "", "0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := inv
			x.Payments = tt.pays
			rv, has := m.Zero, tt.rv != ""
			if has {
				rv = m.MustParse(tt.rv)
			}
			got, ok := fxPaymentBook(&x, tt.tx, tt.year, rv, has)
			if ok != tt.found || (ok && !got.Equal(m.MustParse(tt.want))) {
				t.Fatalf("%v %v, chcem %s %v", got, ok, tt.want, tt.found)
			}
		})
	}
}

func TestForeignPartialPayments(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	if err := a.Store.SaveRates(ctx, []fx.Rate{rate("CZK", "2026-10-22", "25")}); err != nil {
		t.Fatal(err)
	}
	d := validDraft() // 400 + 92 = 492 CZK = 19,68 €
	d.Currency = "CZK"
	d.IssueDate, d.DeliveryDate = invoice.MustDate("2026-10-26"), invoice.MustDate("2026-10-23")
	dr, _ := a.SaveDraft(ctx, c.ID, d)
	inv, err := a.Issue(ctx, c.ID, dr.ID)
	if err != nil {
		t.Fatal(err)
	}
	txs, _, err := a.Store.SaveBankTransactions(ctx, c.ID, []bank.Transaction{
		{Account: "SK3112000000198742637541", Date: invoice.MustDate("2026-11-10"), Amount: m.MustParse("10"), Currency: "EUR", Direction: bank.In, Fingerprint: "a", Status: bank.StatusOpen},
		{Account: "SK3112000000198742637541", Date: invoice.MustDate("2026-11-20"), Amount: m.MustParse("9.50"), Currency: "EUR", Direction: bank.In, Fingerprint: "b", Status: bank.StatusOpen},
	})
	if err != nil {
		t.Fatal(err)
	}
	// faktúra v CZK sa ponúkne pri ručnom párovaní eurovej platby
	docs, _ := a.OpenDocuments(ctx, c.ID, txs[0].ID)
	offered := false
	for _, d := range docs {
		if d.ID == inv.ID && d.Currency == "CZK" && d.Amount.Equal(m.MustParse("492")) {
			offered = true
		}
	}
	if !offered {
		t.Fatalf("doklady: %+v", docs)
	}
	if _, err := a.MatchForeign(ctx, c.ID, txs[0].ID, "invoice", inv.ID, m.MustParse("500")); !fieldErr(err, "foreignAmount") {
		t.Fatalf("viac ako zostatok: %v", err)
	}
	if _, err := a.MatchForeign(ctx, c.ID, txs[0].ID, "invoice", inv.ID, m.MustParse("246")); err != nil {
		t.Fatal(err)
	}
	got, _ := a.Store.GetInvoice(ctx, c.ID, inv.ID)
	if got.Status != invoice.StatusIssued || !got.Remaining().Equal(m.MustParse("246")) || !got.Payments[0].EUR.Equal(m.MustParse("10")) {
		t.Fatalf("po prvej platbe: %s %v %+v", got.Status, got.Remaining(), got.Payments)
	}
	// zvyšok (bez sumy = celý zostatok)
	if _, err := a.MatchForeign(ctx, c.ID, txs[1].ID, "invoice", inv.ID, m.Zero); err != nil {
		t.Fatal(err)
	}
	if got, _ = a.Store.GetInvoice(ctx, c.ID, inv.ID); got.Status != invoice.StatusPaid {
		t.Fatalf("uhradená: %s", got.Status)
	}
	book, err := a.Ledger(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"2026-11-10": "221 MD 10.00, 311 D 9.84, 663 D 0.16",
		"2026-11-20": "221 MD 9.50, 311 D 9.84, 563 MD 0.34",
	}
	n := 0
	for _, ps := range book.Postings {
		if ps.Doc != chart.DocBank {
			continue
		}
		n++
		if w := want[ps.Date.String()]; lineStr(ps.Lines) != w {
			t.Errorf("%s: %s %q, chcem %s", ps.Date, lineStr(ps.Lines), ps.Problem, w)
		}
	}
	if n != 2 {
		t.Fatalf("bankové zápisy: %d", n)
	}
	// spárovaný pohyb sa druhýkrát nepáruje
	if _, err := a.MatchForeign(ctx, c.ID, txs[1].ID, "invoice", inv.ID, m.Zero); !errors.Is(err, ErrAlreadyMatched) {
		t.Fatalf("druhé spárovanie: %v", err)
	}
}
