package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/payroll"
)

// fiktívne účty s platným kontrolným číslom
const (
	ibanSocial = "SK2381800000007000000011"
	ibanHealth = "SK2281800000007000000029"
	ibanWorker = "SK4609000000005000123456"
	ibanFirm   = "SK7211000000002900111222"
)

func TestTaxIBAN(t *testing.T) {
	tests := []struct{ oud, want string }{
		{"1234567890", "SK7981805002591234567890"},
		{"", ""},
		{"12345678901", ""},
		{"12a", ""},
	}
	for _, tt := range tests {
		if got := TaxIBAN(tt.oud); got != tt.want {
			t.Errorf("%q: %q, chcem %q", tt.oud, got, tt.want)
		}
	}
}

func TestPayrollPayList(t *testing.T) {
	d := m.MustParse
	rows := []PayrollRow{
		{Employee: payroll.Employee{ID: 1, FirstName: "Jana", LastName: "Nová", Insurer: "25", IBAN: ibanWorker},
			Result: payroll.Result{EmployeeID: 1, SocialEmployee: d("141"), SocialEmployer: d("379.65"), HealthEmployee: d("75"), HealthEmployer: d("165"),
				TaxPaid: d("90"), ToPay: d("1100")}},
		{Employee: payroll.Employee{ID: 2, FirstName: "Peter", LastName: "Malý", Insurer: "24"},
			Result: payroll.Result{EmployeeID: 2, SocialEmployee: d("10"), SocialEmployer: d("20"), HealthEmployee: d("5"), HealthEmployer: d("10"),
				BonusPaid: d("100"), ToPay: d("300")}},
	}
	full := PayrollPaySettings{SocialIBAN: ibanSocial, SocialVS: "1234567890", TaxOUD: "1234567890",
		Health: map[string]HealthPay{"24": {IBAN: ibanHealth, VS: "11112222"}, "25": {IBAN: ibanHealth, VS: "33334444"}}}
	type want struct {
		key, amount, vs, ss, problem string
	}
	tests := []struct {
		name  string
		ibans map[int64]string
		s     PayrollPaySettings
		want  []want
	}{
		{"všetko vyplnené", map[int64]string{2: ibanWorker}, full, []want{
			{"wage:1", "1100", "", "", ""}, {"wage:2", "300", "", "", ""},
			{"social", "550.65", "1234567890", "10992026", ""},
			{"health:24", "15", "11112222", "", ""}, {"health:25", "240", "33334444", "", ""},
			{"tax", "-10", "1100102026", "", "nie je čo odviesť"},
		}},
		{"chýbajú údaje", nil, PayrollPaySettings{SocialIBAN: ibanSocial}, []want{
			{"wage:1", "1100", "", "", ""}, {"wage:2", "300", "", "", "Chýba IBAN"},
			{"social", "550.65", "", "10992026", "variabilný symbol"},
			{"health:24", "15", "", "", "Doplňte účet"}, {"health:25", "240", "", "", "Doplňte účet"},
			{"tax", "-10", "1100102026", "", "nie je čo odviesť"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := payrollPayList(2026, 10, rows, tt.ibans, tt.s)
			if len(got) != len(tt.want) {
				t.Fatalf("úhrady: %+v", got)
			}
			for i, w := range tt.want {
				p := got[i]
				if p.Key != w.key || !p.Amount.Equal(d(w.amount)) || p.VS != w.vs || p.SS != w.ss ||
					(w.problem == "") != (p.Problem == "") || !strings.Contains(p.Problem, w.problem) {
					t.Errorf("%d: %+v, chcem %+v", i, p, w)
				}
			}
		})
	}
}

func TestPayrollPaymentOrder(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	c.IBAN = ibanFirm
	if _, err := a.UpdateCompany(ctx, c); err != nil {
		t.Fatal(err)
	}
	var ve *invoice.ValidationError
	if _, err := a.SavePayrollPaySettings(ctx, c.ID, PayrollPaySettings{SocialIBAN: "SK00", SocialVS: "123", TaxOUD: "x"}); !errors.As(err, &ve) || len(ve.Fields) != 3 {
		t.Fatalf("kontrola údajov: %v", err)
	}
	if _, err := a.SavePayrollPaySettings(ctx, c.ID, PayrollPaySettings{SocialIBAN: ibanSocial, SocialVS: "1234567890", TaxOUD: "1234567890",
		Health: map[string]HealthPay{"25": {IBAN: ibanHealth, VS: "33334444"}, "99": {IBAN: ibanHealth}}}); err != nil {
		t.Fatal(err)
	}
	jana, err := a.SaveEmployee(ctx, c.ID, payroll.Employee{FirstName: "Jana", LastName: "Nová", Contract: payroll.TPP, Insurer: "25",
		Start: "2026-01-01", Wage: m.MustParse("1500"), Hours: m.MustParse("40"), NCZD: true})
	if err != nil {
		t.Fatal(err)
	}
	a.Today = func() invoice.Date { return invoice.MustDate("2026-11-05") }
	on := invoice.MustDate("2026-11-06")
	if _, _, err := a.PayrollPaymentOrder(ctx, c.ID, 2026, 10, []string{"social"}, on); err == nil || !strings.Contains(err.Error(), "uzavrite") {
		t.Fatalf("neuzavreté mzdy: %v", err)
	}
	if _, err := a.ClosePayroll(ctx, c.ID, 2026, 10); err != nil {
		t.Fatal(err)
	}
	// IBAN doplnený po uzavretí sa použije
	if _, _, err := a.PayrollPaymentOrder(ctx, c.ID, 2026, 10, []string{"wage:1"}, on); err == nil || !strings.Contains(err.Error(), "IBAN") {
		t.Fatalf("bez IBAN zamestnanca: %v", err)
	}
	jana.IBAN = ibanWorker
	if _, err := a.SaveEmployee(ctx, c.ID, jana); err != nil {
		t.Fatal(err)
	}
	pp, err := a.PayrollPaymentsFor(ctx, c.ID, 2026, 10)
	if err != nil || len(pp.Payments) != 4 {
		t.Fatalf("úhrady: %+v %v", pp, err)
	}
	if _, ok := pp.Settings.Health["99"]; ok {
		t.Error("neznáma poisťovňa sa nemá uložiť")
	}
	if _, _, err := a.PayrollPaymentOrder(ctx, c.ID, 2026, 10, []string{"social"}, invoice.MustDate("2026-11-01")); err == nil {
		t.Fatal("dátum v minulosti")
	}
	name, xml, err := a.PayrollPaymentOrder(ctx, c.ID, 2026, 10, []string{"wage:1", "social", "health:25", "tax"}, on)
	if err != nil {
		t.Fatal(err)
	}
	s := string(xml)
	for _, w := range []string{"<NbOfTxs>4</NbOfTxs>", ibanWorker, ibanSocial, "SK7981805002591234567890",
		"/VS1234567890/SS10992026/KS3118", "/VS1100102026/SS/KS", "Mzda 10/2026"} {
		if !strings.Contains(s, w) {
			t.Errorf("v príkaze chýba %q", w)
		}
	}
	if name != "mzdy-2026-10-prikaz.xml" {
		t.Errorf("názov: %s", name)
	}
}
