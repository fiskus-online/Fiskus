package dpfo

import (
	"errors"
	"slices"
	"testing"
)

// Hodnoty 2027 overené proti zverejneným sumám (ŽM 295,22 € od 1. 7. 2026).
func TestParams2027(t *testing.T) {
	p, err := ParamsFor(2027)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ name, got, want string }{
		{"NČZD do hranice", nczd(m("27101.20"), p).String(), "6199.62"},
		{"NČZD nad hranicou: 15 233,35 − 30 000 / 3", nczd(m("30000"), p).String(), "5233.35"},
		{"NČZD nulová od 45 700,05", nczd(m("45700.05"), p).String(), "0.00"},
		{"mesačná NČZD", MonthlyNCZD(m("1000"), p).String(), "516.64"},
		{"daň na hranici 25 %", progressive(m("45700.06"), p.Brackets).String(), "8683.01"},
		{"daň vo všetkých pásmach", progressive(m("100000"), p.Brackets).String(), "25225.86"},
		{"manžel/ka bez príjmu", spouseNCZD(&Spouse{Months: 12}, m("40000"), p).String(), "5668.22"},
		{"manžel/ka nad hranicou: 20 901,58 − 50 000 / 3", spouseNCZD(&Spouse{Months: 12}, m("50000"), p).String(), "4234.91"},
		{"hranica bonusu", p.BonusLimit.String(), "29160.00"},
	} {
		if tt.got != tt.want {
			t.Errorf("%s: %s, chcem %s", tt.name, tt.got, tt.want)
		}
	}
	// priznanie za 2027 ešte nezostavíme (tlačivo nie je vydané)
	if slices.Contains(Years(), 2027) {
		t.Errorf("Years() obsahuje 2027: %v", Years())
	}
	if _, err := Compute(Input{Year: 2027, Income: m("1000")}); !errors.Is(err, ErrYear) {
		t.Errorf("Compute 2027: %v", err)
	}
}
