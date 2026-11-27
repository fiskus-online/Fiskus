package app

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/dpfo"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/payroll"
)

func TestSettlements(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	jana, err := a.SaveEmployee(ctx, c.ID, payroll.Employee{FirstName: "Jana", LastName: "Nová", Contract: payroll.TPP, Insurer: "25",
		Start: "2026-01-01", Wage: m.MustParse("1500"), Hours: m.MustParse("40"), NCZD: true})
	if err != nil {
		t.Fatal(err)
	}
	for mo := 1; mo <= 12; mo++ {
		if _, err := a.ClosePayroll(ctx, c.ID, 2026, mo); err != nil {
			t.Fatal(err)
		}
	}
	v, err := a.Settlements(ctx, c.ID, 2026)
	if err != nil || len(v.List) != 1 || len(v.Open) != 0 || v.Deadline != "2027-03-31" {
		t.Fatalf("zúčtovanie: %+v %v", v, err)
	}
	r := v.List[0]
	// 12 × 1 500; poistné 12 × 141 + 12 × 75; preddavky 12 × 149,48
	want := map[string]string{"01": "18000.00", "02": "2592.00", "03": "15408.00", "04a": "5966.73", "06": "1793.84",
		"18": "1793.76", "19": "0.08", "22": "0.08"}
	for k, w := range want {
		if r.Rows[k] != w {
			t.Errorf("r. %s = %s, chcem %s", k, r.Rows[k], w)
		}
	}
	if len(r.Months) != 12 {
		t.Errorf("mesiace %v", r.Months)
	}
	tests := []struct {
		name string
		req  SettlementRequest
		ok   bool
	}{
		{"záporné DDS", SettlementRequest{DDS: m.MustParse("-1")}, false},
		{"13 mesiacov manželky", SettlementRequest{Spouse: &dpfo.Spouse{Months: 13}}, false},
		{"manželka a iný zamestnávateľ", SettlementRequest{Requested: true, Spouse: &dpfo.Spouse{Months: 12},
			OtherIncome: m.MustParse("1000"), OtherSocial: m.MustParse("94"), OtherHealth: m.MustParse("50"),
			OtherPrepaid: m.MustParse("100")}, true},
	}
	for _, tt := range tests {
		err := a.SaveSettlementRequest(ctx, c.ID, 2026, jana.ID, tt.req)
		if (err == nil) != tt.ok {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		if !tt.ok {
			continue
		}
		v, _ = a.Settlements(ctx, c.ID, 2026)
		r = v.List[0]
		// základ 15 408 + 856 = 16 264; NČZD 5 966,73 + 5 455,30; zdaniteľná 4 841,97; daň 919,97; preddavky 1 893,76
		if !r.Request.Requested || r.Rows["03"] != "16264.00" || r.Rows["06"] != "919.97" || r.Result.String() != "-973.79" {
			t.Errorf("%s: %+v %v", tt.name, r.Rows, r.Result)
		}
	}
	if err := a.SaveSettlementRequest(ctx, c.ID, 2026, 999, SettlementRequest{}); err == nil {
		t.Error("neexistujúci zamestnanec")
	}
	// neuzavretý rok: predbežné
	v, _ = a.Settlements(ctx, c.ID, 2025)
	if len(v.List) != 0 || len(v.Open) != 12 {
		t.Errorf("2025: %+v", v)
	}
}
