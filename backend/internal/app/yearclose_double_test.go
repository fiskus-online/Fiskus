package app

import (
	"strings"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/ledger"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func TestYearCloseDouble(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	c.Bookkeeping = store.BookkeepingDouble
	if _, err := a.UpdateCompany(ctx, c); err != nil {
		t.Fatal(err)
	}
	d, _ := a.SaveDraft(ctx, c.ID, validDraft())
	if _, err := a.Issue(ctx, c.ID, d.ID); err != nil {
		t.Fatal(err)
	}
	check := func(info YearCloseInfo, id string) YearCheck {
		for _, x := range info.Checks {
			if x.ID == id {
				return x
			}
		}
		t.Fatalf("kontrola %s chýba: %+v", id, info.Checks)
		return YearCheck{}
	}
	// rok ešte neskončil
	info, err := a.YearCloseFor(ctx, c.ID, 2026)
	if err != nil || info.CanClose || check(info, "ended").OK || !check(info, "balanced").OK || !check(info, "unposted").OK {
		t.Fatalf("2026 v októbri: %+v %v", info, err)
	}
	a.Today = func() invoice.Date { return invoice.MustDate("2027-02-10") }
	info, _ = a.YearCloseFor(ctx, c.ID, 2026)
	if !info.CanClose {
		t.Fatalf("pripravený: %+v", info.Checks)
	}
	info, err = a.CloseYear(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	cl := info.Close
	if !cl.Closed || !cl.Double || !cl.Opening || !cl.Income.Equal(m.MustParse("400")) || !cl.Expenses.IsZero() || !cl.Result.Equal(m.MustParse("400")) {
		t.Fatalf("uzávierka: %+v", cl)
	}
	// počiatočné stavy 2027 prenesené
	book, _ := a.Ledger(ctx, c.ID, 2027)
	if !book.Opening["311"].Equal(m.MustParse("492")) || !book.Opening["431"].Equal(m.MustParse("-400")) {
		t.Fatalf("počiatočné stavy 2027: %+v", book.Opening)
	}
	// uzávierkové zápisy (710, 702) a otváracie zápisy 2027 (701)
	lc, err := a.LedgerClosingEntries(ctx, c.ID, 2026)
	if err != nil || !lc.Closed || !lc.Result.Equal(m.MustParse("400")) || len(lc.Closing) != 3 || len(lc.Opening) != 1 {
		t.Fatalf("uzávierkové zápisy: %+v %v", lc, err)
	}
	want := []string{
		"602 MD 400.00, 710 D 400.00",
		"710 MD 400.00, 702 D 400.00",
		"311 D 492.00, 343 MD 92.00, 702 MD 400.00",
	}
	for i, e := range lc.Closing {
		if got := lineStr(e.Lines); got != want[i] || e.Lines[0].Name == "" {
			t.Errorf("zápis %d %q: %s", i, e.Text, got)
		}
	}
	if got := lineStr(lc.Opening[0].Lines); got != "311 MD 492.00, 343 D 92.00, 431 D 400.00, 701 D 492.00, 701 MD 492.00" {
		t.Errorf("otvorenie: %s", got)
	}
	// uzamknutý rok: zápisy hlavnej knihy aj doklady
	if _, err := a.SaveInternal(ctx, c.ID, 2026, InternalDoc{Date: invoice.MustDate("2026-12-31"), Text: "Oprava",
		Lines: []ledger.Line{{Account: "518", Debit: m.MustParse("10")}, {Account: "321", Credit: m.MustParse("10")}}}); !fieldErr(err, "date") {
		t.Fatalf("interný doklad v uzavretom roku: %v", err)
	}
	if err := a.SetLedgerOpening(ctx, c.ID, 2026, nil); !fieldErr(err, "date") {
		t.Fatalf("počiatočné stavy uzavretého roka: %v", err)
	}
	dr := validDraft()
	dr.IssueDate, dr.DeliveryDate = invoice.MustDate("2026-12-20"), invoice.MustDate("2026-12-20")
	if d2, err := a.SaveDraft(ctx, c.ID, dr); err == nil {
		if _, err := a.Issue(ctx, c.ID, d2.ID); err == nil {
			t.Fatal("vystavenie faktúry do uzavretého roka")
		}
	}
	// otvorenie, oprava a opätovná uzávierka prenesú počiatočné stavy 2027 znova
	if _, err := a.ReopenYear(ctx, c.ID, 2026, "chýbajúci náklad"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveInternal(ctx, c.ID, 2026, InternalDoc{Date: invoice.MustDate("2026-12-31"), Text: "Chýbajúci náklad",
		Lines: []ledger.Line{{Account: "518", Debit: m.MustParse("100")}, {Account: "321", Credit: m.MustParse("100")}}}); err != nil {
		t.Fatal(err)
	}
	if info, err = a.CloseYear(ctx, c.ID, 2026); err != nil || !info.Close.Opening || !info.Close.Result.Equal(m.MustParse("300")) {
		t.Fatalf("druhá uzávierka: %+v %v", info.Close, err)
	}
	next, err := a.Ledger(ctx, c.ID, 2027)
	if err != nil || !next.Opening["431"].Equal(m.MustParse("-300")) || !next.Opening["321"].Equal(m.MustParse("-100")) {
		t.Fatalf("počiatočné stavy po opätovnej uzávierke: %v %v", next.Opening, err)
	}
	if ck := checkByID(t, a, c.ID, 2027, "opening"); !ck.OK {
		t.Errorf("nadväznosť: %+v", ck)
	}
	// ručne zadané počiatočné stavy ostanú, kontrola nadväznosti upozorní
	if err := a.SetLedgerOpening(ctx, c.ID, 2027, []ledger.Line{{Account: "311", Debit: m.MustParse("492")},
		{Account: "343", Credit: m.MustParse("92")}, {Account: "431", Credit: m.MustParse("400")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ReopenYear(ctx, c.ID, 2026, "kontrola stavov"); err != nil {
		t.Fatal(err)
	}
	if info, err = a.CloseYear(ctx, c.ID, 2026); err != nil || info.Close.Opening {
		t.Fatalf("tretia uzávierka: %+v %v", info.Close, err)
	}
	if ck := checkByID(t, a, c.ID, 2027, "opening"); ck.OK || ck.Blocking || !strings.Contains(ck.Text, "321") {
		t.Errorf("nadväznosť ručných stavov: %+v", ck)
	}
}

func checkByID(t *testing.T, a *App, cid int64, year int, id string) YearCheck {
	t.Helper()
	info, err := a.YearCloseFor(ctx, cid, year)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range info.Checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("kontrola %s chýba: %+v", id, info.Checks)
	return YearCheck{}
}
