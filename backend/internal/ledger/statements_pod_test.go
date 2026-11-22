package ledger

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/chart"
)

// Každý účet rámcovej osnovy (triedy 0 – 6) patrí pri zostatku MD aj D
// najviac do jedného riadku súvahy alebo výkazu Úč POD (žiadne dvojité
// započítanie); nepriradené smú byť iba účty, ktoré vzor nemá.
func TestPodMappingUnique(t *testing.T) {
	allowed := map[string]bool{ // vnútorné zúčtovanie, zúčtovacie a závierkové účty bez riadku vo vzore
		"395": true, "396": true, "431A": true,
	}
	for _, acc := range chart.Default().Accounts {
		if acc.Number == "" || acc.Number[0] > '6' {
			continue
		}
		for _, sign := range []string{"100", "-100"} {
			hits := 0
			for ti, rules := range [][]rule{podAssets, podLiabilities, podIncome} {
				for _, r := range rules {
					if ti == 2 && r.code == "01" {
						continue // čistý obrat je informatívny riadok mimo súčtov
					}
					for _, it := range r.items {
						it = trimCorr(it)
						if matches(it, acc.Number, m(sign)) {
							hits++
						}
					}
				}
			}
			if hits > 1 {
				t.Errorf("účet %s (%s) je v %d riadkoch", acc.Number, sign, hits)
			}
			if hits == 0 && !allowed[acc.Number] {
				t.Errorf("účet %s (%s) %s nemá riadok", acc.Number, sign, acc.Name)
			}
		}
	}
}

func trimCorr(s string) string {
	if len(s) > 0 && s[0] == '-' {
		return s[1:]
	}
	return s
}

func TestStatementsSmall(t *testing.T) {
	// s.r.o.: základné imanie 5 000, zisk minulého roka 2 000 (431), tovar,
	// odberateľ, dodávateľ, DPH, auto s oprávkami, úver, mzdy, daň
	cur := turn(
		"221001", "3700", "211", "200",
		"022", "12000", "082", "-3000",
		"132", "1500", "311", "2460", "-391", "0",
		"321", "-1230", "343", "-460", "331", "-900", "336", "-450", "341", "-120",
		"461", "-4000", "411", "-5000", "431", "-2000",
		"604", "-9000", "602", "-6000", "504", "4000", "518", "1500",
		"521", "2400", "524", "840", "551", "3000", "562", "150", "568", "60", "591", "350",
	)
	st := BuildStatementsForm(FormSmall, cur, nil)
	a, l, i := rowsBy(st.Assets), rowsBy(st.Liabilities), rowsBy(st.Income)
	want := []struct {
		rows map[string]StatementRow
		code string
		v    string
	}{
		{a, "14", "9000"}, {a, "39", "1500"}, {a, "57", "2460"}, {a, "72", "200"}, {a, "73", "3700"},
		{a, "33", "7860"}, {a, "01", "16860"},
		{l, "82", "5000"}, {l, "98", "2000"}, {l, "100", "2700"}, {l, "80", "9700"},
		{l, "121", "4000"}, {l, "126", "1230"}, {l, "131", "900"}, {l, "132", "450"}, {l, "133", "580"},
		{l, "122", "3160"}, {l, "79", "16860"},
		{i, "01", "15000"}, {i, "03", "9000"}, {i, "05", "6000"}, {i, "02", "15000"},
		{i, "11", "4000"}, {i, "14", "1500"}, {i, "15", "3240"}, {i, "22", "3000"}, {i, "10", "11740"},
		{i, "27", "3260"}, {i, "28", "9500"}, {i, "45", "210"}, {i, "55", "-210"}, {i, "56", "3050"},
		{i, "57", "350"}, {i, "61", "2700"},
	}
	for _, w := range want {
		if got := w.rows[w.code].Netto; !got.Equal(m(w.v)) {
			t.Errorf("r. %s: %s, chcem %s", w.code, got, w.v)
		}
	}
	if st.Form != FormSmall || !st.Difference.IsZero() || len(st.Unmapped) != 0 || len(st.Assets) != 78 || len(st.Liabilities) != 67 || len(st.Income) != 61 {
		t.Fatalf("vzor %s, rozdiel %s, nepriradené %+v, riadky %d/%d/%d", st.Form, st.Difference, st.Unmapped, len(st.Assets), len(st.Liabilities), len(st.Income))
	}
}
