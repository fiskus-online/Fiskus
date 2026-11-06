package app

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/bank"
	"github.com/palo-krnac/fiscus/backend/internal/cash"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/journal"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
)

func TestYearClose(t *testing.T) {
	a, c := newApp(t, edition.Desktop) // dnes 29. 10. 2026
	acc := "SK3112000000198742637541"
	other := "SK6807200002891987426353" // účet bez pohybov v 2025 so zadaným stavom 2026 — neprepíše sa
	_ = a.SetJournalOpening(ctx, c.ID, 2025, map[string]m.Dec{acc: m.MustParse("1000")})
	_ = a.SetJournalOpening(ctx, c.ID, 2026, map[string]m.Dec{other: m.MustParse("55")})
	txs, _, _ := a.Store.SaveBankTransactions(ctx, c.ID, []bank.Transaction{
		{Account: acc, Date: invoice.MustDate("2025-06-10"), Amount: m.MustParse("300"), Currency: "EUR", Direction: bank.Out, Fingerprint: "a", Status: bank.StatusOpen},
	})
	key := "bank:" + itoa(int(txs[0].ID))

	check := func(info YearCloseInfo, id string) YearCheck {
		for _, c := range info.Checks {
			if c.ID == id {
				return c
			}
		}
		t.Fatalf("chýba kontrola %s", id)
		return YearCheck{}
	}
	// bežný rok sa nedá uzavrieť, nerozpísaný pohyb tiež blokuje
	info, err := a.YearCloseFor(ctx, c.ID, 2026)
	if err != nil || check(info, "ended").OK || info.CanClose {
		t.Fatalf("bežný rok: %+v %v", info, err)
	}
	info, _ = a.YearCloseFor(ctx, c.ID, 2025)
	if check(info, "journal").OK || info.CanClose {
		t.Fatalf("nerozpísaný pohyb: %+v", info.Checks)
	}
	if _, err := a.CloseYear(ctx, c.ID, 2025); !fieldErr(err, "year") {
		t.Fatalf("uzávierka s nerozpísaným pohybom: %v", err)
	}
	if err := a.ClassifyJournal(ctx, c.ID, key, []journal.Line{{Column: journal.ExpOther, Amount: m.MustParse("300")}}, ""); err != nil {
		t.Fatal(err)
	}
	vklad, err := a.CreateCash(ctx, c.ID, cash.Document{Kind: cash.In, Date: invoice.MustDate("2025-12-30"), Amount: m.MustParse("50"), Purpose: "Vklad"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ClassifyJournal(ctx, c.ID, "cash:"+itoa(int(vklad.ID)), []journal.Line{{Column: journal.IncNonTax, Amount: m.MustParse("50")}}, ""); err != nil {
		t.Fatal(err)
	}
	info, _ = a.YearCloseFor(ctx, c.ID, 2025)
	if !info.CanClose || check(info, "inputs").OK {
		t.Fatalf("pred uzávierkou: %+v", info.Checks) // nezadané riadky výkazu iba upozornia
	}
	// uzávierka
	info, err = a.CloseYear(ctx, c.ID, 2025)
	if err != nil || info.Close == nil || !info.Close.Closed || !info.Close.Bank[acc].Equal(m.MustParse("700")) || len(info.Years) != 1 {
		t.Fatalf("uzávierka: %+v %v", info.Close, err)
	}
	// stav účtu prenesený do 2026
	book, _ := a.Journal(ctx, c.ID, 2026)
	found, kept := false, false
	for _, b := range book.Balances {
		if b.Account == acc {
			found = b.Opening.Equal(m.MustParse("700"))
		}
		if b.Account == other {
			kept = b.Opening.Equal(m.MustParse("55"))
		}
	}
	if !found || !kept || len(info.Close.Bank) != 1 {
		t.Fatalf("počiatočný stav 2026: %+v", book.Balances)
	}
	if _, err := a.CloseYear(ctx, c.ID, 2025); !fieldErr(err, "year") {
		t.Fatalf("druhá uzávierka: %v", err)
	}

	// uzavretý rok sa nedá meniť
	locked := []struct {
		name string
		err  error
	}{
		{"rozpis denníka", a.ClassifyJournal(ctx, c.ID, key, []journal.Line{{Column: journal.ExpOther, Amount: m.MustParse("300")}}, "")},
		{"pokladničný doklad", func() error {
			_, err := a.CreateCash(ctx, c.ID, cash.Document{Kind: cash.In, Date: invoice.MustDate("2025-05-01"), Amount: m.MustParse("1"), Purpose: "x"})
			return err
		}()},
		{"počiatočné stavy", a.SetJournalOpening(ctx, c.ID, 2025, map[string]m.Dec{acc: m.MustParse("1")})},
		{"riadky výkazu", a.SetStatementInputs(ctx, c.ID, 2025, journal.AssetInputs{})},
		{"uzávierková operácia", func() error { _, err := a.AddClosing(ctx, c.ID, 2025, "odpis", nil); return err }()},
		{"vystavenie faktúry", func() error {
			d := validDraft()
			d.IssueDate, d.DeliveryDate, d.DueDate = invoice.MustDate("2025-11-01"), invoice.MustDate("2025-11-01"), invoice.MustDate("2025-11-15")
			dr, err := a.SaveDraft(ctx, c.ID, d)
			if err != nil {
				return err
			}
			_, err = a.Issue(ctx, c.ID, dr.ID)
			return err
		}()},
		{"párovanie platby", func() error { _, err := a.IgnoreTransaction(ctx, c.ID, txs[0].ID, true, ""); return err }()},
	}
	for _, l := range locked {
		if !fieldErr(l.err, "date") {
			t.Errorf("%s v uzavretom roku: %v", l.name, l.err)
		}
	}
	// import výpisu: pohyb z uzavretého roka sa vynechá
	csv := "Dátum;Suma;Mena;Protistrana;Správa\n15.12.2025;-10,00;EUR;X;stary\n02.10.2026;-20,00;EUR;Y;novy\n"
	res, err := a.ImportStatement(ctx, c.ID, "vypis.csv", []byte(csv))
	if err != nil || res.Imported != 1 || len(res.Warnings) == 0 {
		t.Fatalf("import: %+v %v", res, err)
	}
	// nový rok ide ďalej
	if _, err := a.CreateCash(ctx, c.ID, cash.Document{Kind: cash.In, Date: invoice.MustDate("2026-01-02"), Amount: m.MustParse("1"), Purpose: "x"}); err != nil {
		t.Fatalf("doklad v otvorenom roku: %v", err)
	}

	// otvorenie roka: dôvod povinný, potom sa dá meniť
	if _, err := a.ReopenYear(ctx, c.ID, 2025, ""); !fieldErr(err, "reason") {
		t.Fatalf("otvorenie bez dôvodu: %v", err)
	}
	info, err = a.ReopenYear(ctx, c.ID, 2025, "chýbajúca faktúra za december")
	if err != nil || info.Close.Closed || len(info.Close.History) != 2 || info.Close.History[1].Reason == "" {
		t.Fatalf("otvorenie: %+v %v", info.Close, err)
	}
	if err := a.SetStatementInputs(ctx, c.ID, 2025, journal.AssetInputs{}); err != nil {
		t.Fatalf("zmena po otvorení: %v", err)
	}
	if _, err := a.ReopenYear(ctx, c.ID, 2025, "znova otvoriť"); !fieldErr(err, "year") {
		t.Fatalf("otvorenie neuzavretého roka: %v", err)
	}
}
