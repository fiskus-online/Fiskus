package app

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/household"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func TestHouseholdAssets(t *testing.T) {
	a, firm := newApp(t, edition.Desktop)
	h, err := a.CreateCompany(ctx, store.Company{Name: "Domácnosť", Bookkeeping: store.BookkeepingHousehold})
	if err != nil {
		t.Fatal(err)
	}
	today := a.Today()
	val := func(d invoice.Date, v string) household.AssetValue {
		return household.AssetValue{Date: d, Value: m.MustParse(v)}
	}
	cases := []struct {
		name  string
		x     household.Asset
		field string
	}{
		{"bez názvu", household.Asset{Kind: household.Vehicle, Values: []household.AssetValue{val(today, "1")}}, "name"},
		{"bez hodnoty", household.Asset{Name: "Auto", Kind: household.Vehicle}, "values"},
		{"auto", household.Asset{Name: "Auto", Kind: household.Vehicle, Values: []household.AssetValue{val(today.AddMonths(-24), "12000"), val(today, "9000")}}, ""},
		{"fond", household.Asset{Name: "Fond", Kind: household.Investment, Values: []household.AssetValue{val(today, "3000")}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := a.SaveHouseholdAsset(ctx, h.ID, c.x)
			if c.field == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if !fieldErr(err, c.field) {
				t.Fatalf("chcem %s: %v", c.field, err)
			}
		})
	}
	xs, err := a.HouseholdAssets(ctx, h.ID)
	if err != nil || len(xs) != 2 || xs[0].ID != 1 || xs[1].ID != 2 || !xs[0].Value.Equal(m.MustParse("9000")) {
		t.Fatalf("%+v %v", xs, err)
	}
	// úprava a neexistujúci
	x := xs[1].Asset
	x.Values = append(x.Values, val(today.AddDays(-1), "2900"))
	if x, err = a.SaveHouseholdAsset(ctx, h.ID, x); err != nil || x.Values[0].Date != today.AddDays(-1) {
		t.Fatalf("úprava %+v %v", x, err)
	}
	if _, err := a.SaveHouseholdAsset(ctx, h.ID, household.Asset{ID: 99, Name: "X", Kind: household.OtherAsset, Values: []household.AssetValue{val(today, "1")}}); err != store.ErrNotFound {
		t.Fatalf("neexistujúci: %v", err)
	}
	// čistý majetok: hotovosť 0 + účet 500 + majetok 12000 − úver
	acc, _ := a.SaveHouseholdAccount(ctx, h.ID, household.Account{Name: "Účet", Kind: household.Bank, Opening: m.MustParse("500")})
	if _, err := a.SaveHouseholdObligation(ctx, h.ID, household.Obligation{Kind: household.Loan, Name: "Úver", Principal: m.MustParse("4000"),
		Installment: m.MustParse("100"), Frequency: household.Monthly, FirstDue: today.AddMonths(-1), Category: "Splátky a dlhy", AccountID: acc.ID}); err != nil {
		t.Fatal(err)
	}
	r, err := a.HouseholdNetWorth(ctx, h.ID, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.History) != 7 || !r.Now.Total.Equal(m.MustParse("8500")) || len(r.Debts) != 1 || len(r.Assets) != 2 {
		t.Fatalf("%+v", r.Now)
	}
	if err := a.DeleteHouseholdAsset(ctx, h.ID, 1); err != nil {
		t.Fatal(err)
	}
	if xs, _ = a.HouseholdAssets(ctx, h.ID); len(xs) != 1 {
		t.Fatal("nezmazané")
	}
	if _, err := a.HouseholdAssets(ctx, firm.ID); err == nil {
		t.Fatal("firma nie je domácnosť")
	}
}
