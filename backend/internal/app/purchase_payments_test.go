package app

import (
	"errors"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/bank"
	"github.com/fiskus-online/fiskus/backend/internal/cash"
	"github.com/fiskus-online/fiskus/backend/internal/chart"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/fx"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/journal"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/purchase"
	"github.com/fiskus-online/fiskus/backend/internal/store"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

const ownIBAN = "SK3112000000198742637541"

// Prijatá faktúra 500 + 115 = 615 € uhradená dvoma platbami 200 € a 415 €.
func TestPurchasePartialPayments(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	in := validPurchase()
	in.Rates = []purchase.RateRow{{Rate: vat.Rate23, Base: m.MustParse("500")}}
	p, _, err := a.SavePurchase(ctx, c.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Summary.Total.Equal(m.MustParse("615")) {
		t.Fatalf("suma %s", p.Summary.Total)
	}
	txs, _, err := a.Store.SaveBankTransactions(ctx, c.ID, []bank.Transaction{
		{Account: ownIBAN, Date: invoice.MustDate("2026-10-20"), Amount: m.MustParse("200"), Currency: "EUR", Direction: bank.Out, Fingerprint: "a", Status: bank.StatusOpen},
		{Account: ownIBAN, Date: invoice.MustDate("2026-11-05"), Amount: m.MustParse("415"), Currency: "EUR", Direction: bank.Out, Fingerprint: "b", Status: bank.StatusOpen},
		{Account: ownIBAN, Date: invoice.MustDate("2026-11-06"), Amount: m.MustParse("1"), Currency: "EUR", Direction: bank.Out, Fingerprint: "c", Status: bank.StatusOpen},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.MatchTransaction(ctx, c.ID, txs[0].ID, "purchase", p.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := a.Store.GetPurchase(ctx, c.ID, p.ID)
	if got.Status != purchase.StatusBooked || !got.Balance.Equal(m.MustParse("415")) || !got.PaidAmount.Equal(m.MustParse("200")) {
		t.Fatalf("po prvej platbe: %s uhradené %s zostáva %s", got.Status, got.PaidAmount, got.Balance)
	}
	// návrh párovania ponúka zostatok
	docs, _ := a.OpenDocuments(ctx, c.ID, txs[1].ID)
	offered := false
	for _, d := range docs {
		offered = offered || (d.ID == p.ID && d.Kind == "purchase" && d.Amount.Equal(m.MustParse("415")))
	}
	if !offered {
		t.Fatalf("doklady: %+v", docs)
	}
	// rozpracovanú faktúru nemožno zmazať ani znížiť pod úhrady
	if err := a.DeletePurchase(ctx, c.ID, p.ID); !errors.Is(err, purchase.ErrPaid) {
		t.Fatalf("zmazanie: %v", err)
	}
	low := got
	low.Rates = []purchase.RateRow{{Rate: vat.Rate23, Base: m.MustParse("100")}}
	if _, _, err := a.SavePurchase(ctx, c.ID, low); !fieldErr(err, "rates") {
		t.Fatalf("suma pod úhradami: %v", err)
	}
	if _, err := a.MatchTransaction(ctx, c.ID, txs[1].ID, "purchase", p.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ = a.Store.GetPurchase(ctx, c.ID, p.ID); got.Status != purchase.StatusPaid || got.PaidDate != invoice.MustDate("2026-11-05") {
		t.Fatalf("uhradená: %s %s", got.Status, got.PaidDate)
	}
	if _, err := a.MatchTransaction(ctx, c.ID, txs[2].ID, "purchase", p.ID); !errors.Is(err, ErrDocumentPaid) {
		t.Fatalf("platba navyše: %v", err)
	}

	// kniha záväzkov k dňu
	list, _ := a.Store.ListPurchases(ctx, c.ID, store.PurchaseFilter{})
	for _, tt := range []struct {
		at   string
		want string // "" = bez záväzku
		paid string
	}{
		{"2026-10-10", "615", "0"},
		{"2026-10-31", "415", "200"},
		{"2026-11-05", "", ""},
	} {
		items := journal.Payables(list, invoice.MustDate(tt.at))
		switch {
		case tt.want == "" && len(items) != 0:
			t.Errorf("%s: %+v", tt.at, items)
		case tt.want != "" && (len(items) != 1 || !items[0].Remaining.Equal(m.MustParse(tt.want)) || !items[0].Paid.Equal(m.MustParse(tt.paid)) || !items[0].RemainEUR.Equal(m.MustParse(tt.want))):
			t.Errorf("%s: %+v", tt.at, items)
		}
	}

	// zrušené spárovanie druhej platby vráti zostatok 415 €, prvá ostane
	if _, err := a.UnmatchTransaction(ctx, c.ID, txs[1].ID); err != nil {
		t.Fatal(err)
	}
	if got, _ = a.Store.GetPurchase(ctx, c.ID, p.ID); got.Status != purchase.StatusBooked || !got.Balance.Equal(m.MustParse("415")) || len(got.Payments) != 1 {
		t.Fatalf("po zrušení: %s %s %+v", got.Status, got.Balance, got.Payments)
	}
}

// Prijatá faktúra 492 CZK = 19,68 € uhradená z eurového účtu dvoma
// platbami po 246 CZK (10 € a 9,50 €): každá odpíše 9,84 € z 321.
func TestPurchaseForeignPartialPayments(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	var rates []fx.Rate
	for _, d := range []string{"2026-10-02", "2026-10-03", "2026-10-04", "2026-10-05", "2026-10-06"} {
		rates = append(rates, rate("CZK", d, "25"))
	}
	if err := a.Store.SaveRates(ctx, rates); err != nil {
		t.Fatal(err)
	}
	in := validPurchase()
	in.Currency = "CZK"
	in.Rates = []purchase.RateRow{{Rate: vat.Rate23, Base: m.MustParse("400")}}
	p, _, err := a.SavePurchase(ctx, c.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	tx, ok := p.Tax()
	if !p.Summary.Total.Equal(m.MustParse("492")) || !ok || !tx.Summary.Total.Equal(m.MustParse("19.68")) {
		t.Fatalf("suma %s, v eurách %+v %v", p.Summary.Total, tx.Summary, ok)
	}
	txs, _, err := a.Store.SaveBankTransactions(ctx, c.ID, []bank.Transaction{
		{Account: ownIBAN, Date: invoice.MustDate("2026-11-10"), Amount: m.MustParse("10"), Currency: "EUR", Direction: bank.Out, Fingerprint: "a", Status: bank.StatusOpen},
		{Account: ownIBAN, Date: invoice.MustDate("2026-11-20"), Amount: m.MustParse("9.50"), Currency: "EUR", Direction: bank.Out, Fingerprint: "b", Status: bank.StatusOpen},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.MatchForeign(ctx, c.ID, txs[0].ID, "purchase", p.ID, m.MustParse("500")); !fieldErr(err, "foreignAmount") {
		t.Fatalf("viac ako zostatok: %v", err)
	}
	if _, err := a.MatchForeign(ctx, c.ID, txs[0].ID, "purchase", p.ID, m.MustParse("246")); err != nil {
		t.Fatal(err)
	}
	got, _ := a.Store.GetPurchase(ctx, c.ID, p.ID)
	if got.Status != purchase.StatusBooked || !got.Balance.Equal(m.MustParse("246")) || !got.Payments[0].EUR.Equal(m.MustParse("10")) {
		t.Fatalf("po prvej platbe: %s %s %+v", got.Status, got.Balance, got.Payments)
	}
	// zostatok v knihe záväzkov v eurách pomerne: 246 CZK = 9,84 €
	list, _ := a.Store.ListPurchases(ctx, c.ID, store.PurchaseFilter{})
	if items := journal.Payables(list, invoice.MustDate("2026-11-15")); len(items) != 1 || !items[0].RemainEUR.Equal(m.MustParse("9.84")) {
		t.Fatalf("záväzok: %+v", items)
	}
	if _, err := a.MatchForeign(ctx, c.ID, txs[1].ID, "purchase", p.ID, m.Zero); err != nil {
		t.Fatal(err)
	}
	if got, _ = a.Store.GetPurchase(ctx, c.ID, p.ID); got.Status != purchase.StatusPaid {
		t.Fatalf("uhradená: %s", got.Status)
	}
	book, err := a.Ledger(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	// platíme viac, ako je záväzok v účtovníctve: kurzová strata, menej: zisk
	want := map[string]string{
		"2026-11-10": "321 MD 9.84, 221 D 10.00, 563 MD 0.16",
		"2026-11-20": "321 MD 9.84, 221 D 9.50, 663 D 0.34",
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
}

// Pokladničný doklad uhradí časť prijatej faktúry; storno úhradu vráti.
func TestPurchaseCashPartialPayment(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	in := validPurchase() // 81,30 + 18,70 = 100 €
	p, _, err := a.SavePurchase(ctx, c.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateCash(ctx, c.ID, cash.Document{Kind: cash.In, Date: invoice.MustDate("2026-10-10"), Amount: m.MustParse("500"), Purpose: "Vklad"}); err != nil {
		t.Fatal(err)
	}
	over := cash.Document{Kind: cash.Out, Date: invoice.MustDate("2026-10-12"), Amount: m.MustParse("120"), Purpose: "Úhrada", LinkKind: "purchase", LinkID: p.ID}
	if _, err := a.CreateCash(ctx, c.ID, over); !fieldErr(err, "linkId") {
		t.Fatalf("viac ako zostatok: %v", err)
	}
	part := over
	part.Amount = m.MustParse("40")
	d, err := a.CreateCash(ctx, c.ID, part)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := a.Store.GetPurchase(ctx, c.ID, p.ID)
	if got.Status != purchase.StatusBooked || !got.Balance.Equal(m.MustParse("60")) {
		t.Fatalf("po úhrade pokladňou: %s %s", got.Status, got.Balance)
	}
	if _, err := a.CancelCash(ctx, c.ID, d.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ = a.Store.GetPurchase(ctx, c.ID, p.ID); !got.Balance.Equal(m.MustParse("100")) || len(got.Payments) != 0 {
		t.Fatalf("po storne: %s %+v", got.Balance, got.Payments)
	}
}
