package app

import (
	"errors"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/payroll"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func TestPayrollLines(t *testing.T) {
	d := m.MustParse
	tests := []struct {
		name string
		res  []payroll.Result
		want [][3]string // účet, MD, D
	}{
		{"prázdne", nil, nil},
		{"mzda so zrážkou", []payroll.Result{{Gross: d("1500"), Income: d("1500"), Contrib: d("201"), ContribER: d("543"),
			TaxPaid: d("164.48"), Net: d("1134.52"), Deduction: d("34.52"), ToPay: d("1100")}},
			[][3]string{{"521", "1500", "0"}, {"331", "0", "1500"}, {"524", "543", "0"}, {"336", "0", "543"},
				{"331", "201", "0"}, {"336", "0", "201"}, {"331", "164.48", "0"}, {"342", "0", "164.48"},
				{"331", "34.52", "0"}, {"379", "0", "34.52"}}},
		{"PN a bonus nad preddavok", []payroll.Result{{Gross: d("500"), SickPay: d("120"), Income: d("620"), Contrib: d("67"),
			ContribER: d("181"), BonusPaid: d("40"), Net: d("593"), ToPay: d("593")}},
			[][3]string{{"521", "500", "0"}, {"331", "0", "500"}, {"527", "120", "0"}, {"331", "0", "120"},
				{"524", "181", "0"}, {"336", "0", "181"}, {"331", "67", "0"}, {"336", "0", "67"},
				{"342", "40", "0"}, {"331", "0", "40"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := payrollLines(tt.res)
			if len(got) != len(tt.want) {
				t.Fatalf("riadky: %+v", got)
			}
			var md, dal, z331, toPay m.Dec
			for i, w := range tt.want {
				l := got[i]
				if l.Account != w[0] || !l.Debit.Equal(d(w[1])) || !l.Credit.Equal(d(w[2])) {
					t.Errorf("riadok %d: %+v, chcem %v", i, l, w)
				}
				md, dal = md.Add(l.Debit), dal.Add(l.Credit)
				if l.Account == "331" {
					z331 = z331.Add(l.Credit).Sub(l.Debit)
				}
			}
			for _, r := range tt.res {
				toPay = toPay.Add(r.ToPay)
			}
			if !md.Equal(dal) || !z331.Equal(toPay) {
				t.Errorf("MD %s D %s, zostatok 331 %s ≠ na výplatu %s", md, dal, z331, toPay)
			}
		})
	}
}

func TestPostPayroll(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	var ve *invoice.ValidationError
	if _, err := a.PayrollPostingFor(ctx, c.ID, 2026, 3); !errors.As(err, &ve) {
		t.Fatalf("jednoduché účtovníctvo: %v", err)
	}
	c.Bookkeeping = store.BookkeepingDouble
	if _, err := a.UpdateCompany(ctx, c); err != nil {
		t.Fatal(err)
	}
	jana, err := a.SaveEmployee(ctx, c.ID, payroll.Employee{FirstName: "Jana", LastName: "Nová", Contract: payroll.TPP, Insurer: "25",
		Start: "2026-01-01", Wage: m.MustParse("1500"), Hours: m.MustParse("40"), NCZD: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SavePayroll(ctx, c.ID, 2026, 3, []payroll.Input{{EmployeeID: jana.ID, Deduction: m.MustParse("34.52")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.PostPayroll(ctx, c.ID, 2026, 3); !errors.As(err, &ve) {
		t.Fatalf("neuzavreté mzdy: %v", err)
	}
	if _, err := a.ClosePayroll(ctx, c.ID, 2026, 3); err != nil {
		t.Fatal(err)
	}
	doc, err := a.PostPayroll(ctx, c.ID, 2026, 3)
	if err != nil || doc.Payroll != "2026-03" || doc.Date.String() != "2026-03-31" {
		t.Fatalf("zaúčtovanie: %+v %v", doc, err)
	}
	pp, _ := a.PayrollPostingFor(ctx, c.ID, 2026, 3)
	if pp.Posted != doc.Number || !pp.Current || !pp.ToPay.Equal(m.MustParse("1100")) {
		t.Fatalf("stav: %+v", pp)
	}
	// po otvorení a zmene zrážky doklad nezodpovedá, nové zaúčtovanie ho nahradí
	if _, err := a.ReopenPayroll(ctx, c.ID, 2026, 3, "oprava zrážky"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SavePayroll(ctx, c.ID, 2026, 3, []payroll.Input{{EmployeeID: jana.ID, Deduction: m.MustParse("100")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ClosePayroll(ctx, c.ID, 2026, 3); err != nil {
		t.Fatal(err)
	}
	if pp, _ = a.PayrollPostingFor(ctx, c.ID, 2026, 3); pp.Current {
		t.Fatal("doklad po zmene miezd nemá byť aktuálny")
	}
	again, err := a.PostPayroll(ctx, c.ID, 2026, 3)
	if err != nil || again.Number != doc.Number {
		t.Fatalf("opakované zaúčtovanie: %+v %v", again, err)
	}
	// ručná úprava dokladu neodstráni väzbu na mzdy
	again.Text = "Mzdy marec"
	if _, err := a.SaveInternal(ctx, c.ID, 2026, again); err != nil {
		t.Fatal(err)
	}
	book, err := a.Ledger(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, x := range book.Internal {
		if x.Payroll == "2026-03" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("dokladov miezd za marec: %d", n)
	}
	var bal m.Dec
	for _, acc := range book.Accounts {
		if acc.Account == "331" {
			bal = acc.Closing.Neg()
		}
	}
	if !bal.Equal(m.MustParse("1034.52")) {
		t.Errorf("zostatok 331: %s", bal)
	}
}
