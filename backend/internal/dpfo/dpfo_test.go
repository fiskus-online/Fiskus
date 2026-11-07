package dpfo

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

func TestCompute(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want map[string]string // riadok → suma ("" = nevyplnený)
	}{
		{"2026 živnosť, skutočné výdavky", Input{Year: 2026, Income: m("30000"), Expenses: m("12000"), Prepaid: m("1000")},
			map[string]string{"39": "30000", "40": "12000", "41": "18000", "72": "18000", "73": "5966.73", "77": "5966.73", "78": "",
				"91": "5966.73", "92": "12033.27", "95": "30000", "96": "1804.99", "116": "1804.99", "133": "1000", "135": "804.99", "136": ""}},
		{"2026 paušálne výdavky so stropom a poistným", Input{Year: 2026, Income: m("40000"), Insurance: m("4000"), FlatExpenses: true},
			map[string]string{"40": "24000", "41": "16000", "92": "10033.27", "96": "1504.99", "135": "1504.99"}},
		{"2026 príjmy nad 100 000 — pásma 19/25/30/35, bez NČZD", Input{Year: 2026, Income: m("150000"), Expenses: m("50000")},
			map[string]string{"41": "100000", "73": "", "92": "100000", "96": "25593.02", "116": "25593.02"}},
		{"2025 živnosť (NČZD 5 753,79)", Input{Year: 2025, Income: m("30000"), Expenses: m("12000")},
			map[string]string{"73": "5753.79", "92": "12246.21", "96": "1836.93"}},
		{"2026 príjmy do 50 % NČZD — daň sa nevyrubí", Input{Year: 2026, Income: m("2500")},
			map[string]string{"41": "2500", "116": "", "135": ""}},
		{"2026 strata", Input{Year: 2026, Income: m("1000"), Expenses: m("3000")},
			map[string]string{"41": "", "42": "-2000", "45": "", "46": "-2000", "116": ""}},
		{"2026 zamestnanie a živnosť", Input{Year: 2026, EmpIncome: m("20000"), EmpInsurance: m("2680"), EmpPrepaid: m("2000"), Income: m("15000"), Expenses: m("5000"), DDS: m("250")},
			map[string]string{"38": "17320", "72": "27320", "73": "5554.44", "75": "180", "77": "5734.44", "78": "11585.56",
				"81": "2201.26", "91": "", "92": "10000", "96": "1500", "116": "3701.26", "131": "2000", "135": "1701.26"}},
		{"2026 preplatok a podiel dane 2 %", Input{Year: 2026, Income: m("30000"), Expenses: m("12000"), Prepaid: m("2000"), NGOICO: "123"},
			map[string]string{"135": "", "136": "195.01", "152": "36.10"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := Compute(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			for row, want := range tt.want {
				got := res.Rows[row]
				if (want == "" && !got.IsZero()) || (want != "" && !got.Equal(m(want))) {
					t.Errorf("r. %s = %s, chcem %q", row, got, want)
				}
			}
		})
	}
	if _, err := Compute(Input{Year: 2019}); !errors.Is(err, ErrYear) {
		t.Errorf("neznámy rok: %v", err)
	}
}

func TestProgressive(t *testing.T) {
	p, _ := ParamsFor(2026)
	for _, tt := range []struct{ base, want string }{
		{"0", "0"}, {"10000", "1900"}, {"43983.32", "8356.83"}, {"50000", "9861"}, {"100000", "25593.02"},
	} {
		if got := progressive(m(tt.base), p.Brackets); !got.Equal(m(tt.want)) {
			t.Errorf("%s → %s, chcem %s", tt.base, got, tt.want)
		}
	}
}

func TestXML(t *testing.T) {
	in := Input{Year: 2026, Income: m("30000"), Expenses: m("12000"), Prepaid: m("2000"), NGOICO: "31 821 715", NGOName: "Občianske združenie Pomoc deťom a mládeži Banská Bystrica",
		RefundIBAN: "sk31 1200 0000 1987 4263 7541", Date: "15.03.2027",
		Taxpayer: Taxpayer{DIC: "1012345678", Surname: "Novák", Name: "Ján", Street: "Hlavná", Number: "1", Zip: "974 01", City: "Banská Bystrica", Country: "Slovenská republika", NACE: "43.21.0", Activity: "Elektrické inštalácie"}}
	in.Table1a[0] = [2]money.Dec{m("1000"), m("800")}
	res, _ := Compute(in)
	out, err := XML(in, res)
	if err != nil {
		t.Fatal(err)
	}
	// rovnaká štruktúra ako šablóna formulára (rovnaké prvky v rovnakom poradí)
	tpl, _ := parse(templateV25)
	got, err := parse(out)
	if err != nil {
		t.Fatalf("neplatné XML: %v", err)
	}
	if a, b := shape(tpl), shape(got); a != b {
		t.Fatal("štruktúra XML sa líši od šablóny formulára")
	}
	s := string(out)
	for _, want := range []string{
		"<dic>1012345678</dic>", "<rok>2026</rok>", "<priezvisko>Novák</priezvisko>", "<psc>97401</psc>",
		"<k1>43</k1>", "<k2>21</k2>", "<k3>0</k3>", "<cinnost>Elektrické inštalácie</cinnost>",
		"<t1r2>\n\t\t\t\t<s1>30000.00</s1>\n\t\t\t\t<s2>12000.00</s2>", "<r39>30000.00</r39>", "<r96>1804.99</r96>",
		"<r136>195.01</r136>", "<ico>31821715</ico>", "<neuplatnujemPar50>0</neuplatnujemPar50>", "<r152>36.10</r152>",
		"<vydavkyPar6ods11_ods1a2>1</vydavkyPar6ods11_ods1a2>", "<vratitDanPreplatok>1</vratitDanPreplatok>",
		"<IBAN>SK3112000000198742637541</IBAN>", "<riadok><![CDATA[Občianske združenie Pomoc deťom a]]></riadok>",
		"<datumVyhlasenia>15.03.2027</datumVyhlasenia>",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("chýba %q", want)
		}
	}
	if !bytes.HasPrefix(out, []byte(`<?xml version="1.0" encoding="utf-8"?>`)) {
		t.Error("hlavička XML")
	}
	// tabuľka 1a: riadok 1 vyplnený, ostatné prázdne
	if !strings.Contains(s, "<tabulka1a>\n\t\t\t<t1r1>\n\t\t\t\t<s1>1000.00</s1>\n\t\t\t\t<s2>800.00</s2>") {
		t.Error("tabuľka 1a")
	}
}

// shape je postupnosť ciest prvkov (bez hodnôt).
func shape(n *node) string {
	var b strings.Builder
	var walk func(n *node, path string)
	walk = func(n *node, path string) {
		p := path + "/" + n.name
		b.WriteString(p + "\n")
		for _, c := range n.children {
			walk(c, p)
		}
	}
	walk(n, "")
	return b.String()
}
