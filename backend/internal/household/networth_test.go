package household

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
)

func TestAssetValidate(t *testing.T) {
	v := func(date, val string) AssetValue { return AssetValue{Date: invoice.MustDate(date), Value: m(val)} }
	cases := []struct {
		name  string
		a     Asset
		field string
	}{
		{"v poriadku", Asset{Name: "Auto", Kind: Vehicle, Values: []AssetValue{v("2026-01-01", "9000"), v("2025-01-01", "11000")}}, ""},
		{"bez názvu", Asset{Name: " ", Kind: Vehicle, Values: []AssetValue{v("2026-01-01", "1")}}, "name"},
		{"zlý druh", Asset{Name: "X", Kind: "car", Values: []AssetValue{v("2026-01-01", "1")}}, "kind"},
		{"bez hodnoty", Asset{Name: "X", Kind: Realty}, "values"},
		{"záporná", Asset{Name: "X", Kind: Realty, Values: []AssetValue{v("2026-01-01", "-1")}}, "values.0"},
		{"dvakrát dátum", Asset{Name: "X", Kind: Realty, Values: []AssetValue{v("2026-01-01", "1"), v("2026-01-01", "2")}}, "values.1"},
		{"predaj = nula", Asset{Name: "X", Kind: Vehicle, Values: []AssetValue{v("2025-01-01", "5000"), v("2026-01-01", "0")}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.a.Normalize()
			err := c.a.Validate()
			if c.field == "" {
				if err != nil {
					t.Fatal(err)
				}
				if c.a.Values[0].Date.After(c.a.Values[len(c.a.Values)-1].Date) {
					t.Fatal("hodnoty nie sú zoradené")
				}
				return
			}
			ve, ok := err.(*invoice.ValidationError)
			if !ok || ve.Fields[0].Field != c.field {
				t.Fatalf("chcem %s: %v", c.field, err)
			}
		})
	}
}

func TestNetWorthAt(t *testing.T) {
	accs := []Account{{ID: 1, Opening: m("1000")}, {ID: 2, Kind: Card, Opening: m("0")}}
	es := []Entry{
		{Date: d(2026, 2, 10), AccountID: 2, Amount: m("-200"), Category: "x"},    // karta v mínuse
		{Date: d(2026, 2, 15), AccountID: 1, Amount: m("-300"), ObligationID: 10}, // splátka úveru
		{Date: d(2026, 3, 15), AccountID: 1, Amount: m("-300"), ObligationID: 10},
		{Date: d(2026, 3, 20), AccountID: 1, Amount: m("100"), ObligationID: 20}, // vrátená pôžička
	}
	assets := []Asset{{Name: "Auto", Kind: Vehicle, Values: []AssetValue{
		{Date: d(2026, 1, 1), Value: m("10000")}, {Date: d(2026, 3, 1), Value: m("9500")},
	}}}
	obls := []Obligation{
		{ID: 10, Kind: Loan, Principal: m("6000"), FirstDue: d(2026, 2, 15)},
		{ID: 20, Kind: Lent, Principal: m("500"), FirstDue: d(2026, 3, 20)},
		{ID: 30, Kind: Regular, Installment: m("50"), FirstDue: d(2026, 1, 1)},
		{ID: 40, Kind: Debt, Principal: m("1000"), FirstDue: d(2026, 1, 1), Closed: true},
	}
	cases := []struct {
		date                                        string
		accounts, assets, receivables, debts, total string
	}{
		{"2025-12-31", "1000", "0", "0", "0", "1000"},
		{"2026-01-31", "1000", "10000", "0", "6000", "5000"},  // úver od mesiaca pred prvou splátkou
		{"2026-02-28", "500", "10000", "500", "5700", "5300"}, // požičané od 20. 2.
		{"2026-03-31", "300", "9500", "400", "5400", "4800"},
	}
	for _, c := range cases {
		n := NetWorthAt(accs, es, assets, obls, invoice.MustDate(c.date))
		got := []string{n.Accounts.String(), n.Assets.String(), n.Receivables.String(), n.Debts.String(), n.Total.String()}
		want := []string{c.accounts, c.assets, c.receivables, c.debts, c.total}
		for i := range got {
			if !m(got[i]).Equal(m(want[i])) {
				t.Errorf("%s: got %v, want %v", c.date, got, want)
				break
			}
		}
	}
}

func TestMonthEnds(t *testing.T) {
	got := MonthEnds(invoice.MustDate("2026-03-19"), 3)
	want := []string{"2025-12-31", "2026-01-31", "2026-02-28", "2026-03-19"}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for i := range want {
		if got[i].String() != want[i] {
			t.Fatalf("%v", got)
		}
	}
}
