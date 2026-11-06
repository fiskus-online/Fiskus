package asset

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

func TestAccSchedule(t *testing.T) {
	d := invoice.MustDate
	m := money.MustParse
	base := Asset{Name: "Stroj", Kind: Tangible, Group: 1, Method: Straight, Acquired: d("2025-03-15"), Cost: m("4800"), Share: 100}
	type row struct {
		year                             int
		amount, residual, disposal, diff string
	}
	tests := []struct {
		name string
		edit func(a *Asset)
		want []row
	}{
		{"48 mesiacov od apríla", func(a *Asset) { a.AccMonths = 48 }, []row{
			{2025, "900", "3900", "0", "-100"}, // daňový 1 200 × 10/12 = 1 000
			{2026, "1200", "2700", "0", "0"},
			{2027, "1200", "1500", "0", "0"},
			{2028, "1200", "300", "0", "0"},
			{2029, "300", "0", "0", "100"}, // neuplatnená časť daňového odpisu 200
		}},
		{"vyradenie v júni 2027", func(a *Asset) { a.AccMonths = 48; a.Disposed = d("2027-06-10"); a.DisposalKind = DisposalSale }, []row{
			{2025, "900", "3900", "0", "-100"},
			{2026, "1200", "2700", "0", "0"},
			{2027, "500", "0", "2200", "500"},
		}},
		{"ako daňové, aj s podielom 50 %", func(a *Asset) { a.Share = 50 }, []row{
			{2025, "1000", "3800", "0", "0"},
			{2026, "1200", "2600", "0", "0"},
			{2027, "1200", "1400", "0", "0"},
			{2028, "1200", "200", "0", "0"},
			{2029, "200", "0", "0", "0"},
		}},
		{"zaokrúhlenie v poslednom mesiaci", func(a *Asset) { a.Cost = m("2500"); a.AccMonths = 36; a.Acquired = d("2025-12-01") }, []row{
			{2025, "0", "2500", "0", "-52.08"},
			{2026, "833.28", "1666.72", "0", "208.28"},
			{2027, "833.28", "833.44", "0", "208.28"},
			{2028, "833.44", "0", "0", "208.44"},
		}},
	}
	for _, tt := range tests {
		a := base
		tt.edit(&a)
		plan, err := a.AccSchedule()
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if len(plan) != len(tt.want) {
			t.Fatalf("%s: %d rokov %+v", tt.name, len(plan), plan)
		}
		for i, w := range tt.want {
			y := plan[i]
			if y.Year != w.year || !y.Amount.Equal(m(w.amount)) || !y.Residual.Equal(m(w.residual)) || !y.Disposal.Equal(m(w.disposal)) || !y.Diff.Equal(m(w.diff)) {
				t.Errorf("%s %d: %+v, chcem %+v", tt.name, w.year, y, w)
			}
		}
	}
	if DepAccount("022") != "082" || DepAccount("013") != "073" || DepAccount("021") != "081" || DepAccount("511") != "" {
		t.Error("účty oprávok")
	}
	if (&Asset{Kind: Intangible}).DefaultAccount() != "013" || (&Asset{Kind: Tangible, Group: 5}).DefaultAccount() != "021" {
		t.Error("predvolený účet")
	}
	bad := base
	bad.Account, bad.AccMonths = "511", -1
	if err := bad.Validate(); err == nil {
		t.Error("zlý účet a doba")
	}
	a := base
	a.AccMonths = 48
	if !a.AccResidualAt(2026).Equal(m("2700")) || !a.AccResidualAt(2024).IsZero() || !a.AccInYear(2027).Amount.Equal(m("1200")) {
		t.Error("ZC k roku")
	}
}
