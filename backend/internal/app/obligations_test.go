package app

import (
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/household"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func TestHouseholdObligations(t *testing.T) {
	a, _ := newApp(t, edition.Desktop) // dnes 29. 10. 2026
	h, _ := a.CreateCompany(ctx, store.Company{Name: "Domácnosť", Bookkeeping: store.BookkeepingHousehold})
	acc, _ := a.SaveHouseholdAccount(ctx, h.ID, household.Account{Name: "Účet", Opening: m.MustParse("2000")})

	bad := []struct {
		name  string
		o     household.Obligation
		field string
	}{
		{"neznáma kategória", household.Obligation{Kind: household.Loan, Name: "X", Principal: m.MustParse("100"), Installment: m.MustParse("10"),
			FirstDue: invoice.MustDate("2026-10-01"), Category: "Neexistuje"}, "category"},
		{"príjmová kategória pri dlhu", household.Obligation{Kind: household.Debt, Name: "X", Principal: m.MustParse("100"), Installment: m.MustParse("10"),
			FirstDue: invoice.MustDate("2026-10-01"), Category: "Iné príjmy"}, "category"},
		{"neznámy účet", household.Obligation{Kind: household.Loan, Name: "X", Principal: m.MustParse("100"), Installment: m.MustParse("10"),
			FirstDue: invoice.MustDate("2026-10-01"), AccountID: 999}, "accountId"},
	}
	for _, c := range bad {
		if _, err := a.SaveHouseholdObligation(ctx, h.ID, c.o); !fieldErr(err, c.field) {
			t.Errorf("%s: %v", c.name, err)
		}
	}

	exe, err := a.SaveHouseholdObligation(ctx, h.ID, household.Obligation{Kind: household.Execution, Name: "Exekúcia EX 12/2025", Creditor: "JUDr. Novák",
		Principal: m.MustParse("900"), Installment: m.MustParse("150"), FirstDue: invoice.MustDate("2026-09-15"), AccountID: acc.ID, VS: "122025"})
	if err != nil || exe.Category != "Splátky a dlhy" || exe.Frequency != household.Monthly {
		t.Fatalf("exekúcia: %+v %v", exe, err)
	}
	// september zaplatený, október po splatnosti
	if _, err := a.PayObligation(ctx, h.ID, exe.ID, PayRequest{Date: invoice.MustDate("2026-09-14")}); err != nil {
		t.Fatal(err)
	}
	d, err := a.HouseholdObligation(ctx, h.ID, exe.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Payments) != 1 || !d.Payments[0].Amount.Equal(m.MustParse("-150")) || d.Payments[0].Payee != "JUDr. Novák" ||
		!d.State.Remaining.Equal(m.MustParse("750")) || d.State.Overdue != 1 || len(d.Schedule) != 6 {
		t.Fatalf("detail: %+v", d)
	}
	// prehľad: splátky po splatnosti a do mesiaca
	ov, _ := a.HouseholdSummary(ctx, h.ID, 2026, 10)
	if len(ov.Upcoming) != 2 || ov.Upcoming[0].Status != household.DueOverdue || ov.Upcoming[1].Date != invoice.MustDate("2026-11-15") {
		t.Fatalf("najbližšie splátky: %+v", ov.Upcoming)
	}
	// kalendár domácnosti: splátky, bez daňových termínov
	cal, err := a.Calendar(ctx, h.ID, invoice.MustDate("2026-10-01"), invoice.MustDate("2026-12-31"))
	if err != nil {
		t.Fatal(err)
	}
	titles := []string{}
	for _, e := range cal.Events {
		titles = append(titles, e.Title)
		if strings.Contains(e.Title, "Daňové priznanie") {
			t.Fatal("daňový termín v domácnosti")
		}
	}
	if len(cal.Events) != 3 || !cal.Events[0].Overdue || cal.Events[0].Link != "/domacnost/zavazky/"+itoa(int(exe.ID)) {
		t.Fatalf("kalendár: %v %+v", titles, cal.Events)
	}
	// so splátkou sa záväzok nedá zmazať
	if err := a.DeleteHouseholdObligation(ctx, h.ID, exe.ID); !fieldErr(err, "id") {
		t.Fatalf("zmazanie so splátkou: %v", err)
	}
	// pohyb s neexistujúcim záväzkom
	if _, err := a.SaveHouseholdEntry(ctx, h.ID, household.Entry{Date: invoice.MustDate("2026-10-01"), AccountID: acc.ID, Amount: m.MustParse("-1"),
		Category: "Iné výdavky", ObligationID: 999}); !fieldErr(err, "obligationId") {
		t.Fatalf("neexistujúci záväzok: %v", err)
	}
	// požičané: splátka je príjem
	lent, err := a.SaveHouseholdObligation(ctx, h.ID, household.Obligation{Kind: household.Lent, Name: "Požičané bratovi", Principal: m.MustParse("300"),
		Installment: m.MustParse("100"), FirstDue: invoice.MustDate("2026-11-01"), AccountID: acc.ID})
	if err != nil || lent.Category != "Iné príjmy" {
		t.Fatalf("požičané: %+v %v", lent, err)
	}
	e, err := a.PayObligation(ctx, h.ID, lent.ID, PayRequest{Amount: m.MustParse("100")})
	if err != nil || !e.Amount.Equal(m.MustParse("100")) || e.Date != invoice.MustDate("2026-10-29") {
		t.Fatalf("vrátené: %+v %v", e, err)
	}
	empty, _ := a.SaveHouseholdObligation(ctx, h.ID, household.Obligation{Kind: household.Regular, Name: "Nájom", Installment: m.MustParse("520"),
		FirstDue: invoice.MustDate("2026-11-05")})
	if err := a.DeleteHouseholdObligation(ctx, h.ID, empty.ID); err != nil {
		t.Fatalf("zmazanie bez splátok: %v", err)
	}
	list, _ := a.HouseholdObligations(ctx, h.ID)
	if len(list) != 2 {
		t.Fatalf("záväzky: %d", len(list))
	}
}
