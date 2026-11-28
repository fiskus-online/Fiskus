package app

import (
	"fmt"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/dpfo"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/payroll"
)

// Potvrdenie o zdaniteľných príjmoch zo súčtu uzavretých mesiacov.
func TestCertificate(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	// Peter žil v domácnosti iba január až jún
	half := [12]bool{true, true, true, true, true, true}
	jana, err := a.SaveEmployee(ctx, c.ID, payroll.Employee{FirstName: "Jana", LastName: "Nová", Contract: payroll.TPP, Insurer: "25",
		Start: "2026-01-01", Wage: m.MustParse("1500"), Hours: m.MustParse("40"), NCZD: true, Bonus: true,
		Children: []dpfo.Child{{Name: "Eva Nová", RC: "1556150002"}, {Name: "Peter Nový", RC: "1803100002", Months: half}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveEmployee(ctx, c.ID, payroll.Employee{FirstName: "Ivan", LastName: "Dohodár", Contract: payroll.DoPC, Insurer: "25",
		Start: "2026-01-01", Wage: m.MustParse("300"), Hours: m.MustParse("10")}); err != nil {
		t.Fatal(err)
	}
	for mo := 1; mo <= 8; mo++ {
		if _, err := a.ClosePayroll(ctx, c.ID, 2026, mo); err != nil {
			t.Fatal(err)
		}
	}
	// príjmy od iného zamestnávateľa sa do potvrdenia nedostanú
	if err := a.SaveSettlementRequest(ctx, c.ID, 2026, jana.ID, SettlementRequest{Requested: true,
		OtherIncome: m.MustParse("1000"), OtherPrepaid: m.MustParse("100")}); err != nil {
		t.Fatal(err)
	}
	v, err := a.Settlements(ctx, c.ID, 2026)
	if err != nil || len(v.List) != 2 {
		t.Fatalf("%+v %v", v, err)
	}
	byName := map[string]Certificate{}
	for _, r := range v.List {
		byName[r.Employee.LastName] = r.Cert
	}
	// Jana: 8 × 1 500; poistné 8 × (141 + 75); preddavky 8 × 149,48
	tests := []struct {
		name, who, row, want string
	}{
		{"úhrn príjmov", "Nová", "01", "12000.00"},
		{"z dohôd", "Nová", "01a", "0.00"},
		{"poistné", "Nová", "02", "1728.00"},
		{"sociálne", "Nová", "02a", "1128.00"},
		{"zdravotné", "Nová", "02b", "600.00"},
		{"čiastkový základ", "Nová", "03", "10272.00"},
		{"preddavky bez bonusu", "Nová", "04", "1195.84"},
		{"príjmy z dohody", "Dohodár", "01a", "2400.00"},
		{"dohoda bez NČZD", "Dohodár", "11", "0.00"},
	}
	for _, tt := range tests {
		if got := byName[tt.who].Rows[tt.row]; got != tt.want {
			t.Errorf("%s: r. %s = %s, chcem %s", tt.name, tt.row, got, tt.want)
		}
	}
	cj := byName["Nová"]
	if fmt.Sprint(cj.Months) != "[1 2 3 4 5 6 7 8]" || cj.Rows["11"] == "0.00" {
		t.Errorf("mesiace a NČZD: %v %s", cj.Months, cj.Rows["11"])
	}
	if len(cj.Children) != 2 || fmt.Sprint(cj.Children[0].Months) != "[1 2 3 4 5 6 7 8]" || fmt.Sprint(cj.Children[1].Months) != "[1 2 3 4 5 6]" {
		t.Errorf("deti: %+v", cj.Children)
	}
	if len(byName["Dohodár"].Children) != 0 {
		t.Errorf("dohodár bez detí: %+v", byName["Dohodár"].Children)
	}
}
