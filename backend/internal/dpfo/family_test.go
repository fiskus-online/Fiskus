package dpfo

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/money"
)

const (
	rc2013 = "130620/0005" // 20. 6. 2013
	rc2020 = "200304/0006" // 4. 3. 2020
	rcNov  = "256108/0005" // 8. 11. 2025, dievča
	rc2010 = "100415/0004" // 15 rokov v apríli 2025
	rc2007 = "070901/0005" // 18 rokov v septembri 2025
	rc1985 = "850712/0006"
)

func TestBirthDate(t *testing.T) {
	cases := []struct {
		rc, want string
		bad      bool
	}{
		{rc2013, "2013-06-20", false},
		{rcNov, "2025-11-08", false},
		{"8507120006", "1985-07-12", false},
		{"130620/0006", "", true}, // kontrolný súčet
		{"131320/0005", "", true}, // mesiac 13
		{"12345", "", true},
		{"13062A/0005", "", true},
	}
	for _, c := range cases {
		got, err := BirthDate(c.rc)
		if c.bad {
			if err == nil {
				t.Errorf("%s: chcem chybu", c.rc)
			}
			continue
		}
		if err != nil || got.Format("2006-01-02") != c.want {
			t.Errorf("%s: %v %v", c.rc, got, err)
		}
	}
}

func TestBonus2025(t *testing.T) {
	p, _ := ParamsFor(2025)
	half := func(rc string, from int) Child { // od mesiaca from do konca roka
		c := Child{RC: rc}
		for i := from - 1; i < 12; i++ {
			c.Months[i] = true
		}
		return c
	}
	cases := []struct {
		name     string
		base     string
		children []Child
		want     string
	}{
		// príklady podpora.financnasprava.sk 790897
		{"príklad 1: dve deti, bez krátenia", "23500", []Child{{RC: rc2020}, {RC: rc2013}}, "2400"},
		{"príklad 2: strop 36 % základu", "6250", []Child{{RC: rc2020}, {RC: rc2013}}, "2250"},
		{"príklad 3: druhé dieťa narodené v novembri", "3250", []Child{{RC: rc2013}, {RC: rcNov}}, "980.40"},
		{"príklad 4: zníženie o 1/10 nad hranicou", "30000", []Child{{RC: rc2020}, {RC: rc2013}}, "1548"},
		{"príklad 5: zníženie aj pre novorodenca", "30000", []Child{{RC: rc2020}, {RC: rcNov}}, "903"},
		{"15 rokov v apríli: 4 × 100 + 8 × 50", "20000", []Child{{RC: rc2010}}, "800"},
		{"18 rokov v septembri: 9 × 50", "20000", []Child{{RC: rc2007}}, "450"},
		{"dospelé dieťa", "20000", []Child{{RC: rc1985}}, "0"},
		{"iba niektoré mesiace", "20000", []Child{half(rc2013, 7)}, "600"},
		{"bez základu", "0", []Child{{RC: rc2013}}, "0"},
		{"vysoký základ: bonus zanikne", "40000", []Child{{RC: rc2013}}, "0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Bonus(c.children, m(c.base), p)
			if err != nil {
				t.Fatal(err)
			}
			if !got.Equal(m(c.want)) {
				t.Fatalf("bonus %s, chcem %s", got, c.want)
			}
		})
	}
	if _, err := Bonus([]Child{{Name: "Jano", RC: "1"}}, m("1000"), p); err == nil || !strings.Contains(err.Error(), "Jano") {
		t.Fatalf("zlé rodné číslo: %v", err)
	}
	res, err := Compute(Input{Year: 2025, Income: m("30000"), Children: []Child{{Name: "Jano", RC: "1"}}})
	if err != nil || !res.Rows["117"].IsZero() || len(res.Notes) == 0 {
		t.Fatalf("zlé rodné číslo vo výpočte: %v %v", err, res.Notes)
	}
}

func TestSpouseNCZD(t *testing.T) {
	p25, _ := ParamsFor(2025)
	p26, _ := ParamsFor(2026)
	cases := []struct {
		name string
		p    Params
		s    *Spouse
		base string
		want string
	}{
		{"bez príjmu, celý rok", p25, &Spouse{Months: 12}, "18000", "5260.61"},
		{"vlastný príjem 2 200, 7 mesiacov (príklad FS)", p25, &Spouse{OwnIncome: m("2200"), Months: 7}, "18000", "1785.36"},
		{"základ nad hranicou: 17 370,97 − 1/4", p25, &Spouse{Months: 12}, "60000", "2370.97"},
		{"vlastný príjem vyšší ako suma", p25, &Spouse{OwnIncome: m("6000"), Months: 12}, "18000", "0"},
		{"2026 nad hranicou: 20 116,40 − 1/3", p26, &Spouse{Months: 12}, "45000", "5116.40"},
		{"2026 bez príjmu", p26, &Spouse{Months: 12}, "20000", "5455.30"},
		{"neuplatňuje", p25, nil, "18000", "0"},
	}
	for _, c := range cases {
		if got := spouseNCZD(c.s, m(c.base), c.p); !got.Equal(m(c.want)) {
			t.Errorf("%s: %s, chcem %s", c.name, got, c.want)
		}
	}
}

func TestLosses(t *testing.T) {
	cases := []struct {
		name   string
		in     Input
		r51    string // 2020
		r54    string // 2023
		r55    string
		left20 string
	}{
		{"mikrodaňovník: 2020 do 50 %, ostatné do základu", Input{Year: 2025, Income: m("30000"), Expenses: m("12000"),
			Losses: []Loss{{Year: 2020, Amount: m("20000"), Used: m("5000")}, {Year: 2023, Amount: m("3000")}}}, "9000", "3000", "6000", "15000"},
		{"strata menšia ako základ", Input{Year: 2025, Income: m("30000"), Expenses: m("12000"),
			Losses: []Loss{{Year: 2023, Amount: m("1000")}}}, "", "1000", "17000", ""},
		{"nad 100 000 € príjmov: spolu najviac 50 %", Input{Year: 2025, Income: m("150000"), Expenses: m("50000"),
			Losses: []Loss{{Year: 2022, Amount: m("80000")}, {Year: 2023, Amount: m("30000")}}}, "", "", "50000", ""},
		{"strata staršia ako 5 rokov sa nepočíta", Input{Year: 2025, Income: m("30000"), Expenses: m("12000"),
			Losses: []Loss{{Year: 2019, Amount: m("5000")}}}, "", "", "18000", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := Compute(c.in)
			if err != nil {
				t.Fatal(err)
			}
			r := res.Rows
			eq := func(d money.Dec, want string) bool {
				return (want == "" && d.IsZero()) || (want != "" && d.Equal(m(want)))
			}
			if !eq(r["51"], c.r51) || !eq(r["54"], c.r54) || !eq(r["55"], c.r55) || !eq(r["l47.04"], c.left20) || !r["57"].Equal(r["55"]) {
				t.Fatalf("r51 %s r54 %s r55 %s zostatok 2020 %s", r["51"], r["54"], r["55"], r["l47.04"])
			}
		})
	}
}

func TestComputeFamily(t *testing.T) {
	cases := []struct {
		name string
		in   Input
		want map[string]string
	}{
		{"jedno dieťa, bonus menší ako daň", Input{Year: 2025, Income: m("30000"), Expenses: m("12000"), Children: []Child{{RC: rc2013}}},
			map[string]string{"116": "1836.93", "116a": "18000", "117": "1200", "118": "636.93", "121": "", "135": "636.93"}},
		{"dve deti, bonus vyšší ako daň — vyplatí sa", Input{Year: 2025, Income: m("10000"), Expenses: m("4000"), Children: []Child{{RC: rc2013}, {RC: rc2020}}},
			map[string]string{"92": "246.21", "116": "36.93", "117": "2160", "118": "-2123.07", "120": "2160", "121": "2123.07", "135": "", "136": ""}},
		{"bonus od zamestnávateľa", Input{Year: 2025, Income: m("30000"), Expenses: m("12000"), Children: []Child{{RC: rc2013}}, EmpBonus: m("1200")},
			map[string]string{"117": "1200", "119": "1200", "120": "", "122": "", "135": "1836.93"}},
		{"manželka bez príjmu", Input{Year: 2025, Income: m("30000"), Expenses: m("12000"), Spouse: &Spouse{Name: "Nováková Jana", Months: 12}},
			map[string]string{"74": "5260.61", "77": "11014.40", "92": "6985.60", "96": "1047.84"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := Compute(c.in)
			if err != nil {
				t.Fatal(err)
			}
			for row, want := range c.want {
				got := res.Rows[row]
				if (want == "" && !got.IsZero()) || (want != "" && !got.Equal(m(want))) {
					t.Errorf("r. %s = %s, chcem %q", row, got, want)
				}
			}
		})
	}
}

func TestXMLFamily(t *testing.T) {
	half := Child{Name: "Novák Peter", RC: rc2020}
	half.Months[10], half.Months[11] = true, true
	in := Input{Year: 2025, Income: m("10000"), Expenses: m("4000"), Date: "10.03.2026", RefundIBAN: "SK31 1200 0000 1987 4263 7541", Attachments: 2,
		Children: []Child{{Name: "Nováková Eva", RC: rc2013}, half},
		Spouse:   &Spouse{Name: "Nováková Jana", RC: "855712/0000", OwnIncome: m("1200"), Months: 12},
		Losses:   []Loss{{Year: 2023, Amount: m("500")}}}
	res, err := Compute(in)
	if err != nil {
		t.Fatal(err)
	}
	x, err := XML(in, res)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<priezviskoMeno>Nováková Eva</priezviskoMeno>", "<rodneCislo>1306200005</rodneCislo>", "<m00>1</m00>", "<m11>1</m11>",
		"<uplatnujemNCZDNaManzela>1</uplatnujemNCZDNaManzela>", "<vlastnePrijmy>1200.00</vlastnePrijmy>", "<pocetMesiacov>12</pocetMesiacov>",
		"<rodneCislo>8557120000</rodneCislo>", "<r54>500.00</r54>", "<rok>2023</rok>", "<r154>2</r154>",
		"<vyplatitDanovyBonus>1</vyplatitDanovyBonus>", "<IBAN>SK3112000000198742637541</IBAN>",
		"<r117>" + res.Rows["117"].String() + "</r117>", "<r116a>6000.00</r116a>",
	} {
		if !bytes.Contains(x, []byte(want)) {
			t.Errorf("v XML chýba %s", want)
		}
	}
}
