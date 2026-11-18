package app

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/household"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func TestReconcileHouseholdAccount(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	h, err := a.CreateCompany(ctx, store.Company{Name: "Domácnosť", Bookkeeping: store.BookkeepingHousehold})
	if err != nil {
		t.Fatal(err)
	}
	acc, err := a.SaveHouseholdAccount(ctx, h.ID, household.Account{Name: "Účet", Kind: household.Bank, Opening: m.MustParse("1000")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveHouseholdEntry(ctx, h.ID, household.Entry{Date: invoice.MustDate("2026-10-01"), AccountID: acc.ID, Amount: m.MustParse("-100"), Category: "Potraviny"}); err != nil {
		t.Fatal(err)
	}
	day := invoice.MustDate("2026-10-31")
	in := func(bal string, adjust bool) ReconcileInput {
		return ReconcileInput{Date: day, Balance: m.MustParse(bal), Adjust: adjust}
	}
	cases := []struct {
		name     string
		in       ReconcileInput
		commit   bool
		field    string // očakávaná chyba poľa
		diff     string
		category string
		done     bool
		entry    bool
	}{
		{"bez dátumu", ReconcileInput{Balance: m.MustParse("900")}, false, "date", "", "", false, false},
		{"budúci dátum", ReconcileInput{Date: invoice.Today().AddDays(1)}, false, "date", "", "", false, false},
		{"náhľad s rozdielom", in("895.50", false), false, "", "-4.50", "Iné výdavky", false, false},
		{"rozdiel bez vyrovnania sa neuloží", in("895.50", false), true, "balance", "", "", false, false},
		{"vyrovnanie rozdielu", in("895.50", true), true, "", "-4.50", "Iné výdavky", true, true},
		{"po vyrovnaní sedí", in("895.50", false), true, "", "0", "", true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, err := a.ReconcileHouseholdAccount(ctx, h.ID, acc.ID, c.in, c.commit)
			if c.field != "" {
				if !fieldErr(err, c.field) {
					t.Fatalf("chcem chybu %s: %v", c.field, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !r.Difference.Equal(m.MustParse(c.diff)) || r.Done != c.done || (r.Entry != nil) != c.entry {
				t.Fatalf("%+v", r)
			}
			if c.category != "" && r.Category != c.category {
				t.Fatalf("kategória %q", r.Category)
			}
		})
	}
	get := func() HouseholdAccount {
		accs, _ := a.HouseholdAccounts(ctx, h.ID)
		for _, x := range accs {
			if x.ID == acc.ID {
				return x
			}
		}
		t.Fatal("účet chýba")
		return HouseholdAccount{}
	}
	got := get()
	if got.Reconciled == nil || got.Reconciled.Date != day || !got.Stale.IsZero() {
		t.Fatalf("odsúhlasenie: %+v", got)
	}
	// úprava účtu odsúhlasenie nezmaže ani nepodvrhne
	x := got.Account
	x.Reconciled = &household.Reconciled{Date: day, Balance: m.MustParse("1")}
	if x, err = a.SaveHouseholdAccount(ctx, h.ID, x); err != nil || !x.Reconciled.Balance.Equal(m.MustParse("895.50")) {
		t.Fatalf("úprava účtu: %+v %v", x.Reconciled, err)
	}
	// starší pohyb pridaný po odsúhlasení
	if _, err := a.SaveHouseholdEntry(ctx, h.ID, household.Entry{Date: invoice.MustDate("2026-10-15"), AccountID: acc.ID, Amount: m.MustParse("-10"), Category: "Potraviny"}); err != nil {
		t.Fatal(err)
	}
	if got = get(); !got.Stale.Equal(m.MustParse("-10")) {
		t.Fatalf("zmena po odsúhlasení: %s", got.Stale)
	}
}
