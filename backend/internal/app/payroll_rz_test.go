package app

import (
	"strings"
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

// Rozdiel z ročného zúčtovania sa vyrovná v zvolenej mzde nasledujúceho roka.
func TestSettlementInPayroll(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	c.Bookkeeping = "double"
	if _, err := a.UpdateCompany(ctx, c); err != nil {
		t.Fatal(err)
	}
	jana, err := a.SaveEmployee(ctx, c.ID, payroll.Employee{FirstName: "Jana", LastName: "Nová", Contract: payroll.TPP, Insurer: "25",
		Start: "2025-01-01", Wage: m.MustParse("1500"), Hours: m.MustParse("40"), NCZD: true})
	if err != nil {
		t.Fatal(err)
	}
	for mo := 1; mo <= 12; mo++ {
		if _, err := a.ClosePayroll(ctx, c.ID, 2025, mo); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := a.Payroll(ctx, c.ID, 2026, 3)
	tests := []struct {
		name string
		req  SettlementRequest
		ok   bool
	}{
		{"bez žiadosti", SettlementRequest{SettleMonth: "2026-03"}, false},
		{"máj je neskoro", SettlementRequest{Requested: true, SettleMonth: "2026-05"}, false},
		{"zlý rok", SettlementRequest{Requested: true, SettleMonth: "2025-03"}, false},
		{"marec s manželkou", SettlementRequest{Requested: true, SettleMonth: "2026-03", Spouse: &dpfo.Spouse{Months: 12}}, true},
	}
	for _, tt := range tests {
		if err := a.SaveSettlementRequest(ctx, c.ID, 2025, jana.ID, tt.req); (err == nil) != tt.ok {
			t.Errorf("%s: %v", tt.name, err)
		}
	}
	rz, _ := a.Settlements(ctx, c.ID, 2025)
	diff := rz.List[0].TaxDiff
	if diff.Sign() >= 0 {
		t.Fatalf("očakávaný preplatok: %+v", rz.List[0])
	}
	v, err := a.Payroll(ctx, c.ID, 2026, 3)
	if err != nil {
		t.Fatal(err)
	}
	r := v.Rows[0].Result
	if !r.SettleTax.Equal(diff) || !r.Net.Equal(before.Rows[0].Result.Net.Sub(diff)) {
		t.Fatalf("mzda 3/2026: settle %s net %s (pred %s), rozdiel %s", r.SettleTax, r.Net, before.Rows[0].Result.Net, diff)
	}
	if !v.Totals.Tax.Equal(before.Totals.Tax.Add(diff)) {
		t.Errorf("odvod dane %s, pred %s", v.Totals.Tax, before.Totals.Tax)
	}
	if len(r.Notes) == 0 || !strings.Contains(strings.Join(r.Notes, " "), "vráti sa preplatok") {
		t.Errorf("poznámka: %v", r.Notes)
	}
	// február bez vyrovnania
	if f, _ := a.Payroll(ctx, c.ID, 2026, 2); !f.Rows[0].Result.SettleTax.IsZero() {
		t.Error("vyrovnanie len v marci")
	}
	// uzavretie a zaúčtovanie: na 331 ostane suma na výplatu
	if _, err := a.ClosePayroll(ctx, c.ID, 2026, 3); err != nil {
		t.Fatal(err)
	}
	pp, err := a.PayrollPostingFor(ctx, c.ID, 2026, 3)
	if err != nil {
		t.Fatal(err)
	}
	var bal m.Dec
	for _, l := range pp.Lines {
		if l.Account == "331" {
			bal = bal.Add(l.Credit).Sub(l.Debit)
		}
	}
	if !bal.Equal(pp.ToPay) {
		t.Errorf("331 = %s, na výplatu %s", bal, pp.ToPay)
	}
	// uzavretý mesiac sa už nedá zvoliť
	if err := a.SaveSettlementRequest(ctx, c.ID, 2025, jana.ID, SettlementRequest{Requested: true, SettleMonth: "2026-03"}); err == nil {
		t.Error("uzavretý marec")
	}
}
