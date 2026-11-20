package asset

import (
	"strings"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
)

func tz(date, amount string) Improvement {
	return Improvement{Date: invoice.MustDate(date), Amount: money.MustParse(amount)}
}

// Príklady finančnej správy (podpora.financnasprava.sk: vplyv technického
// zhodnotenia na rovnomerné a zrýchlené odpisy, prerušenie odpisovania).
func TestImprovementsAndPause(t *testing.T) {
	cases := []struct {
		name  string
		a     Asset
		want  map[int]string // uplatnený odpis po rokoch (chýbajúci rok = žiadny riadok)
		years int            // počet riadkov plánu
	}{
		{"rovnomerne, TZ v 3. roku",
			Asset{Group: 1, Acquired: invoice.MustDate("2024-06-01"), Cost: money.MustParse("120000"), Improvements: []Improvement{tz("2026-05-10", "10000")}},
			map[int]string{2024: "17500.00", 2025: "30000.00", 2026: "32500.00", 2027: "32500.00", 2028: "17500.00"}, 5},
		{"rovnomerne, TZ v 4. roku predĺži odpisovanie",
			Asset{Group: 1, Acquired: invoice.MustDate("2023-06-01"), Cost: money.MustParse("120000"), Improvements: []Improvement{tz("2026-08-01", "60000")}},
			map[int]string{2023: "17500.00", 2024: "30000.00", 2025: "30000.00", 2026: "45000.00", 2027: "45000.00", 2028: "12500.00"}, 6},
		{"rovnomerne, TZ v prvom roku zvyšuje VC od začiatku",
			Asset{Group: 2, Acquired: invoice.MustDate("2025-04-01"), Cost: money.MustParse("120000"), Improvements: []Improvement{tz("2025-12-01", "5000")}},
			map[int]string{2025: "15625.00", 2026: "20833.33"}, 7},
		{"zrýchlene, TZ v 3. roku zvýši ZC aj o neuplatnenú časť",
			Asset{Group: 2, Method: Accelerated, Acquired: invoice.MustDate("2024-06-01"), Cost: money.MustParse("20000"), Improvements: []Improvement{tz("2026-05-01", "2000")}},
			map[int]string{2024: "1944.44", 2025: "5555.56", 2026: "4833.33", 2027: "3866.67", 2028: "2900.00", 2029: "1933.33", 2030: "966.67"}, 7},
		{"zrýchlene, TZ v prvom roku",
			Asset{Group: 2, Method: Accelerated, Acquired: invoice.MustDate("2026-06-01"), Cost: money.MustParse("150000"), Improvements: []Improvement{tz("2026-12-01", "5000")}},
			map[int]string{2026: "15069.44", 2027: "43055.56", 2032: "10763.89"}, 7},
		{"prerušenie v roku zaradenia a ďalšom: potom celé ročné odpisy",
			Asset{Group: 1, Acquired: invoice.MustDate("2025-05-01"), Cost: money.MustParse("48000"), Paused: []int{2025, 2026}},
			map[int]string{2025: "0", 2026: "0", 2027: "12000.00", 2028: "12000.00", 2029: "12000.00", 2030: "12000.00"}, 6},
		{"prerušenie v druhom roku posunie zvyšok",
			Asset{Group: 1, Acquired: invoice.MustDate("2025-05-01"), Cost: money.MustParse("48000"), Paused: []int{2026}},
			map[int]string{2025: "8000.00", 2026: "0", 2027: "12000.00", 2028: "12000.00", 2029: "12000.00", 2030: "4000.00"}, 6},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.a.Name = "Majetok"
			p := plan(t, c.a)
			if len(p) != c.years {
				t.Fatalf("riadkov %d, chcem %d: %+v", len(p), c.years, p)
			}
			for y, w := range c.want {
				got, ok := p[y]
				if !ok || !got.Claimed.Equal(money.MustParse(w)) {
					t.Errorf("%d: uplatnené %s, chcem %s (%s)", y, got.Claimed, w, got.Note)
				}
			}
		})
	}
}

func TestImprovementResidualAndCost(t *testing.T) {
	a := Asset{Name: "Budova", Group: 1, Acquired: invoice.MustDate("2024-06-01"), Cost: money.MustParse("120000"), Improvements: []Improvement{tz("2026-05-10", "10000")}}
	a.Normalize()
	if !a.CostAt(2025).Equal(money.MustParse("120000")) || !a.CostAt(2026).Equal(money.MustParse("130000")) {
		t.Fatalf("VC: %s %s", a.CostAt(2025), a.CostAt(2026))
	}
	// ZC vrátane neuplatnenej časti 2024 (12 500)
	if got := a.ResidualAt(2026); !got.Equal(money.MustParse("50000")) {
		t.Fatalf("ZC 2026: %s", got)
	}
	// predaj v 2027: ZC do výdavkov
	a.Disposed, a.DisposalKind = invoice.MustDate("2027-03-01"), DisposalSale
	p := plan(t, a)
	if !p[2027].Disposal.Equal(money.MustParse("50000")) {
		t.Fatalf("vyradenie: %+v", p[2027])
	}
}

func TestImprovementValidate(t *testing.T) {
	base := Asset{Name: "Stroj", Group: 2, Acquired: invoice.MustDate("2025-03-01"), Cost: money.MustParse("5000")}
	cases := []struct {
		name  string
		edit  func(*Asset)
		field string
	}{
		{"TZ pred zaradením", func(a *Asset) { a.Improvements = []Improvement{tz("2025-02-01", "100")} }, "improvements.0"},
		{"TZ bez sumy", func(a *Asset) { a.Improvements = []Improvement{tz("2025-04-01", "0")} }, "improvements.0"},
		{"TZ po vyradení", func(a *Asset) {
			a.Disposed, a.DisposalKind = invoice.MustDate("2026-01-01"), DisposalSale
			a.Improvements = []Improvement{tz("2026-02-01", "100")}
		}, "improvements.0"},
		{"prerušenie pred zaradením", func(a *Asset) { a.Paused = []int{2024} }, "paused.0"},
		{"prerušenie dvakrát", func(a *Asset) { a.Paused = []int{2026, 2026} }, "paused.1"},
		{"prerušenie v roku vyradenia", func(a *Asset) {
			a.Disposed, a.DisposalKind = invoice.MustDate("2026-01-01"), DisposalSale
			a.Paused = []int{2026}
		}, "paused.0"},
		{"v poriadku", func(a *Asset) { a.Improvements = []Improvement{tz("2026-02-01", "1200.50")}; a.Paused = []int{2027} }, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := base
			c.edit(&a)
			a.Normalize()
			err := a.Validate()
			if c.field == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			ve, ok := err.(*invoice.ValidationError)
			if !ok || !strings.Contains(fieldsOf(ve), c.field) {
				t.Fatalf("chcem %s: %v", c.field, err)
			}
		})
	}
	// nehmotný majetok sa neprerušuje — prerušenie sa zahodí
	a := Asset{Name: "Softvér", Kind: Intangible, Years: 3, Acquired: invoice.MustDate("2025-01-01"), Cost: money.MustParse("3000"), Paused: []int{2025}}
	a.Normalize()
	if a.Paused != nil {
		t.Fatal("nehmotný majetok s prerušením")
	}
}

func fieldsOf(ve *invoice.ValidationError) string {
	var s []string
	for _, f := range ve.Fields {
		s = append(s, f.Field)
	}
	return strings.Join(s, ",")
}
