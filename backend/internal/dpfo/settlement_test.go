package dpfo

import "testing"

func TestSettle(t *testing.T) {
	p, _ := ParamsFor(2026)
	base := SettlementInput{Income: m("18000"), Social: m("1692"), Health: m("900"), Prepaid: m("1793.76")}
	child := []Child{{Name: "A", RC: "130620/0005"}}
	bonus, _ := Bonus(child, m("15408"), p)
	tests := []struct {
		name string
		in   func(SettlementInput) SettlementInput
		want map[string]string
	}{
		{"len daňovník", func(i SettlementInput) SettlementInput { return i },
			map[string]string{"03": "15408", "04a": "5966.73", "05": "9441.27", "06": "1793.84", "19": "0.08", "20": "0", "22": "0.08"}},
		{"manželka bez príjmu", func(i SettlementInput) SettlementInput { i.Spouse = &Spouse{Months: 12}; return i },
			map[string]string{"04b": "5455.30", "04": "11422.03", "05": "3985.97", "06": "757.33", "20": "-1036.43", "22": "-1036.43"}},
		{"DDS nad strop", func(i SettlementInput) SettlementInput { i.DDS = m("300"); return i },
			map[string]string{"04c": "180", "05": "9261.27", "06": "1759.64"}},
		{"bonus vyplatený celý", func(i SettlementInput) SettlementInput { i.Children, i.BonusPaid = child, bonus; return i },
			map[string]string{"09": bonus.String(), "11": "0", "13": "0", "22": "0.08"}},
		{"bonus vyplatený viac", func(i SettlementInput) SettlementInput {
			i.Children, i.BonusPaid = child, bonus.Add(m("50"))
			return i
		}, map[string]string{"13": "-50", "22": "50.08"}},
		{"bonus nevyplatený", func(i SettlementInput) SettlementInput { i.Children = child; return i },
			map[string]string{"11": bonus.String(), "22": m("0.08").Sub(bonus).String()}},
		{"nízky príjem", func(i SettlementInput) SettlementInput {
			return SettlementInput{Income: m("4000"), Social: m("376"), Health: m("200"), Prepaid: m("0")}
		}, map[string]string{"04": "3424", "05": "0", "06": "0", "22": "0"}},
	}
	for _, tt := range tests {
		s := Settle(tt.in(base), p)
		got := map[string]string{"03": s.R03.String(), "04a": s.R04a.String(), "04b": s.R04b.String(), "04c": s.R04c.String(),
			"04": s.R04.String(), "05": s.R05.String(), "06": s.R06.String(), "09": s.R09.String(), "11": s.R11.String(),
			"13": s.R13.String(), "19": s.R19.String(), "20": s.R20.String(), "22": s.R22.String()}
		for k, v := range tt.want {
			if !m(got[k]).Equal(m(v)) {
				t.Errorf("%s: r. %s = %s, chcem %s", tt.name, k, got[k], v)
			}
		}
	}
}
