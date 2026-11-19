package app

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/household"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func TestBulkHouseholdEntries(t *testing.T) {
	a, firm := newApp(t, edition.Desktop)
	h, err := a.CreateCompany(ctx, store.Company{Name: "Domácnosť", Bookkeeping: store.BookkeepingHousehold})
	if err != nil {
		t.Fatal(err)
	}
	bank, _ := a.SaveHouseholdAccount(ctx, h.ID, household.Account{Name: "Účet", Kind: household.Bank})
	card, _ := a.SaveHouseholdAccount(ctx, h.ID, household.Account{Name: "Karta", Kind: household.Card})
	old, _ := a.SaveHouseholdAccount(ctx, h.ID, household.Account{Name: "Starý", Kind: household.Bank, Closed: true})
	day := invoice.MustDate("2026-10-01")
	add := func(e household.Entry) int64 {
		e.Date = day
		if e.AccountID == 0 {
			e.AccountID = bank.ID
		}
		x, err := a.SaveHouseholdEntry(ctx, h.ID, e)
		if err != nil {
			t.Fatal(err)
		}
		return x.ID
	}
	e1 := add(household.Entry{Amount: m.MustParse("-10"), Category: "Iné výdavky", Payee: "Lidl"})
	e2 := add(household.Entry{Amount: m.MustParse("-20"), Category: "Iné výdavky", Payee: "Lidl"})
	inc := add(household.Entry{Amount: m.MustParse("100"), Category: "Iné príjmy"})
	tr := add(household.Entry{ToAccountID: card.ID, Amount: m.MustParse("50")})

	cases := []struct {
		name    string
		in      BulkInput
		field   string
		changed int
		skipped int
	}{
		{"nič neoznačené", BulkInput{BulkChange: household.BulkChange{Category: "Potraviny"}}, "ids", 0, 0},
		{"bez zmeny", BulkInput{IDs: []int64{e1}}, "category", 0, 0},
		{"zrušený účet", BulkInput{IDs: []int64{e1}, BulkChange: household.BulkChange{AccountID: old.ID}}, "accountId", 0, 0},
		{"kategória: príjem a prevod sa preskočia", BulkInput{IDs: []int64{e1, e2, inc, tr, e1}, BulkChange: household.BulkChange{Category: "Potraviny"}}, "", 2, 2},
		{"znova to isté nič nemení", BulkInput{IDs: []int64{e1, e2}, BulkChange: household.BulkChange{Category: "Potraviny"}}, "", 0, 0},
		{"účet", BulkInput{IDs: []int64{e1, tr}, BulkChange: household.BulkChange{AccountID: card.ID}}, "", 1, 1},
		{"zmazanie", BulkInput{IDs: []int64{e2, inc}, Delete: true}, "", 2, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, err := a.BulkHouseholdEntries(ctx, h.ID, c.in)
			if c.field != "" {
				if !fieldErr(err, c.field) {
					t.Fatalf("chcem chybu %s: %v", c.field, err)
				}
				return
			}
			if err != nil || r.Changed != c.changed || len(r.Skipped) != c.skipped {
				t.Fatalf("%+v %v", r, err)
			}
		})
	}
	x, _ := a.Store.GetHHEntry(ctx, h.ID, e1)
	if x.Category != "Potraviny" || x.AccountID != card.ID {
		t.Fatalf("pohyb: %+v", x)
	}
	if es, _ := a.HouseholdEntries(ctx, h.ID, 0); len(es) != 2 {
		t.Fatalf("po zmazaní %d pohybov", len(es))
	}
	// pohyb inej firmy
	if _, err := a.BulkHouseholdEntries(ctx, firm.ID, BulkInput{IDs: []int64{e1}, Delete: true}); err == nil {
		t.Fatal("firma nie je domácnosť")
	}
}
