package app

import (
	"strings"
	"testing"
	"time"

	"github.com/fiskus-online/fiskus/backend/internal/bank"
	"github.com/fiskus-online/fiskus/backend/internal/calendar"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/payroll"
)

func TestPayrollDeadlines(t *testing.T) {
	days := calendar.NewDays(nil)
	tests := []struct {
		name   string
		year   int
		month  time.Month
		payDay int
		want   []string // id dátum (pôvodný)
	}{
		{"výplata v sobotu", 2026, time.September, 10, []string{
			"pay-wage-2026-09 2026-10-10 ", "pay-ins-2026-09 2026-10-12 2026-10-10", "pay-tax-2026-09 2026-10-15 "}},
		{"daň v nedeľu", 2026, time.October, 10, []string{
			"pay-wage-2026-10 2026-11-10 ", "pay-ins-2026-10 2026-11-10 ", "pay-tax-2026-10 2026-11-16 2026-11-15"}},
		{"deň 31 vo februári", 2026, time.January, 31, []string{
			"pay-wage-2026-01 2026-02-28 ", "pay-ins-2026-01 2026-03-02 2026-02-28", "pay-tax-2026-01 2026-03-05 "}},
		{"bez dňa výplaty", 2026, time.October, 0, []string{"pay-ins-2026-10 2026-11-30 "}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := payrollDeadlines(tt.year, tt.month, tt.payDay, days)
			if len(got) != len(tt.want) {
				t.Fatalf("termíny: %+v", got)
			}
			for i, d := range got {
				s := d.id + " " + d.date.String() + " "
				if !d.shifted.IsZero() {
					s += d.shifted.String()
				}
				if s != tt.want[i] {
					t.Errorf("%d: %q, chcem %q", i, s, tt.want[i])
				}
			}
		})
	}
}

func TestPayrollCalendar(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	a.Today = func() invoice.Date { return invoice.MustDate("2026-10-20") }
	// bez zamestnancov žiadne termíny miezd
	v, err := a.Calendar(ctx, c.ID, invoice.MustDate("2026-10-01"), invoice.MustDate("2026-10-31"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range v.Events {
		if strings.HasPrefix(e.ID, "pay-") {
			t.Fatalf("termín bez zamestnancov: %+v", e)
		}
	}
	if _, err := a.SavePayrollPaySettings(ctx, c.ID, PayrollPaySettings{PayDay: 40}); err == nil {
		t.Fatal("deň výplaty 40")
	}
	if _, err := a.SavePayrollPaySettings(ctx, c.ID, PayrollPaySettings{SocialIBAN: ibanSocial, SocialVS: "1234567890", TaxOUD: "1234567890",
		Health: map[string]HealthPay{"25": {IBAN: ibanHealth, VS: "33334444"}}, PayDay: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveEmployee(ctx, c.ID, payroll.Employee{FirstName: "Jana", LastName: "Nová", Contract: payroll.TPP, Insurer: "25",
		Start: "2026-01-01", Wage: m.MustParse("1500"), Hours: m.MustParse("40"), NCZD: true, IBAN: ibanWorker}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ClosePayroll(ctx, c.ID, 2026, 9); err != nil {
		t.Fatal(err)
	}
	pp, _ := a.PayrollPaymentsFor(ctx, c.ID, 2026, 9)
	var tax PayrollPayment
	for _, p := range pp.Payments {
		if p.Key == "tax" {
			tax = p
		}
	}
	// preddavok dane uhradený z banky
	if _, _, err := a.Store.SaveBankTransactions(ctx, c.ID, []bank.Transaction{{Account: ibanFirm, Date: invoice.MustDate("2026-10-14"),
		Amount: tax.Amount, Currency: "EUR", Direction: bank.Out, VS: "1100092026", CounterIBAN: TaxIBAN("1234567890"), Fingerprint: "t",
		Status: bank.StatusOpen}}); err != nil {
		t.Fatal(err)
	}
	v, err = a.Calendar(ctx, c.ID, invoice.MustDate("2026-10-01"), invoice.MustDate("2026-11-30"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]calendar.Event{}
	for _, e := range v.Events {
		if strings.HasPrefix(e.ID, "pay-") {
			got[e.ID] = e
		}
	}
	tests := []struct {
		id            string
		date          string
		done, overdue bool
		amount        bool
	}{
		{"pay-wage-2026-09", "2026-10-10", false, false, true},
		{"pay-ins-2026-09", "2026-10-12", false, true, true},
		{"pay-tax-2026-09", "2026-10-15", true, false, true},
		{"pay-ins-2026-10", "2026-11-10", false, false, false}, // mzdy za október ešte nie sú uzavreté
	}
	for _, tt := range tests {
		e, ok := got[tt.id]
		if !ok {
			t.Errorf("%s chýba: %v", tt.id, got)
			continue
		}
		if e.Date.String() != tt.date || e.Done != tt.done || e.Overdue != tt.overdue || (e.Amount != nil) != tt.amount {
			t.Errorf("%s: %+v", tt.id, e)
		}
	}
	if !got["pay-tax-2026-09"].Amount.Equal(tax.Amount) {
		t.Errorf("suma dane: %+v", got["pay-tax-2026-09"].Amount)
	}
}
