package dpfo

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

func TestMonthly2026(t *testing.T) {
	p, err := ParamsFor(2026)
	if err != nil {
		t.Fatal(err)
	}
	nczd := []struct{ base, want string }{
		{"0", "0"},
		{"1000", "497.23"},    // 5 966,73 / 12
		{"2173.59", "497.23"}, // do 1/12 hranice 26 083,13
		{"3000", "221.76"},    // (14 661,11 − 36 000 / 3) / 12
		{"3665.28", "0"},      // nad 43 983 ročne nulová
	}
	for _, tt := range nczd {
		if got := MonthlyNCZD(money.MustParse(tt.base), p); !got.Equal(money.MustParse(tt.want)) {
			t.Errorf("NČZD z %s = %s, chcem %s", tt.base, got, tt.want)
		}
	}
	tax := []struct{ base, want string }{
		{"-5", "0"},
		{"1000", "190"},
		{"3665.28", "696.40"},
		{"4000", "780.08"},  // 696,4032 + 334,72 × 25 %
		{"7000", "1666.08"}, // všetky štyri pásma
	}
	for _, tt := range tax {
		if got := MonthlyTax(money.MustParse(tt.base), p); !got.Equal(money.MustParse(tt.want)) {
			t.Errorf("preddavok z %s = %s, chcem %s", tt.base, got, tt.want)
		}
	}
	one := []Child{{Name: "Ema", RC: rc2013}}
	two := []Child{{Name: "Ema", RC: rc2013}, {Name: "Jano", RC: rc2010}}
	adult := []Child{{Name: "Peter", RC: rc2007}}
	part := []Child{{Name: "Ema", RC: rc2013, Months: [12]bool{2: true}}} // iba v marci
	bonus := []struct {
		name     string
		children []Child
		base     string
		month    int
		want     string
	}{
		{"jedno dieťa", one, "1500", 1, "100"},
		{"strop 29 % základu", one, "300", 1, "87"},
		{"zníženie nad 2 286", one, "2786", 1, "50"}, // 100 − 500 / 10
		{"dve deti, staršie 50 €", two, "1500", 3, "150"},
		{"dospelé", adult, "1500", 1, "0"},
		{"mimo mesiacov v domácnosti", part, "1500", 1, "0"},
		{"v marci", part, "1500", 3, "100"},
		{"bez príjmu", one, "0", 1, "0"},
	}
	for _, tt := range bonus {
		got, err := MonthlyBonus(tt.children, money.MustParse(tt.base), tt.month, p)
		if err != nil || !got.Equal(money.MustParse(tt.want)) {
			t.Errorf("%s: bonus %s (%v), chcem %s", tt.name, got, err, tt.want)
		}
	}
	if _, err := MonthlyBonus([]Child{{Name: "X", RC: "123"}}, money.FromInt(1000), 1, p); err == nil {
		t.Error("zlé rodné číslo")
	}
}
