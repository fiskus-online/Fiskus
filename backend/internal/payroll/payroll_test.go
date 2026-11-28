package payroll

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/dpfo"
	"github.com/fiskus-online/fiskus/backend/internal/money"
)

func fund(r Result, f string) Fund {
	for _, x := range r.Social {
		if x.Fund == f {
			return x
		}
	}
	return Fund{}
}

func TestCompute2026(t *testing.T) {
	p, err := ParamsFor(2026)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		e     Employee
		in    Input
		month int
		want  map[string]string // pole → suma
		funds []string          // fondy, ktoré sa platia
	}{
		{
			name: "pracovný pomer 1 500 € s NČZD",
			e:    Employee{Contract: TPP, Wage: m("1500"), NCZD: true},
			want: map[string]string{"gross": "1500", "socialEmployee": "141", "socialEmployer": "378", "healthEmployee": "75",
				"healthEmployer": "165", "taxBase": "1284", "nczd": "497.23", "tax": "149.48", "net": "1134.52", "cost": "2043"},
			funds: []string{FundSickness, FundOldAge, FundDisability, FundUnemployment, FundShortTime, FundGuarantee, FundAccident, FundReserve},
		},
		{
			name: "odmena, dovolenka, PN a zrážka",
			e:    Employee{Contract: TPP, Wage: m("1000"), NCZD: true},
			in:   Input{Extra: m("100"), Leave: m("200"), SickPay: m("50"), Deduction: m("20")},
			want: map[string]string{"gross": "1300", "income": "1350", "socialEmployee": "122.20", "healthEmployee": "65",
				"taxBase": "1162.80", "tax": "126.45", "net": "1036.35", "toPay": "1016.35"},
		},
		{
			name:  "dve deti, bonus nad preddavok",
			e:     Employee{Contract: TPP, Wage: m("915"), NCZD: true, Bonus: true, Children: []dpfo.Child{{Name: "A", RC: "130620/0005"}, {Name: "B", RC: "200304/0006"}}},
			month: 1,
			// 915 − 86,01 − 45,75 = 783,24; (783,24 − 497,23) × 19 % = 54,34; bonus 200, strop 36 % = 281,96
			want: map[string]string{"socialEmployee": "86.01", "healthEmployee": "45.75", "taxBase": "783.24", "tax": "54.34",
				"taxBonus": "200", "taxPaid": "0", "bonusPaid": "145.66", "net": "928.90"},
		},
		{
			name:  "dôchodca, dohoda s nepravidelným príjmom, výnimka 200 €",
			e:     Employee{Contract: DoVP, Wage: m("300"), Pension: PensionOld, OOP: true},
			want:  map[string]string{"socialBase": "100", "socialEmployee": "4", "socialEmployer": "21.90", "healthEmployee": "15", "healthEmployer": "33", "tax": "53.39", "net": "227.61"},
			funds: []string{FundOldAge, FundGuarantee, FundAccident, FundReserve},
		},
		{
			name:  "študent DoBPŠ 350 € s výnimkou",
			e:     Employee{Contract: DoBPS, Wage: m("350"), OOP: true, NCZD: true},
			want:  map[string]string{"socialBase": "150", "socialEmployee": "10.50", "socialEmployer": "36.29", "healthEmployee": "17.50", "tax": "0"},
			funds: []string{FundOldAge, FundDisability, FundGuarantee, FundAccident, FundReserve},
		},
		{
			name:  "dohoda s pravidelným príjmom = ako pracovný pomer",
			e:     Employee{Contract: DoPC, Regular: true, Wage: m("400")},
			want:  map[string]string{"socialEmployee": "37.60", "socialEmployer": "100.80"},
			funds: []string{FundSickness, FundOldAge, FundDisability, FundUnemployment, FundShortTime, FundGuarantee, FundAccident, FundReserve},
		},
		{
			name:  "invalidný dôchodca nad 70 %, ZŤP",
			e:     Employee{Contract: TPP, Wage: m("1000"), Pension: PensionDisabled, ZTP: true},
			want:  map[string]string{"socialEmployee": "84", "healthEmployee": "25", "healthEmployer": "55"},
			funds: []string{FundSickness, FundOldAge, FundDisability, FundGuarantee, FundAccident, FundReserve},
		},
		{
			name: "nad maximálny vymeriavací základ",
			e:    Employee{Contract: TPP, Wage: m("20000")},
			// 16 764 × 9,4 % = 1 575,81; úrazové z celej mzdy
			want: map[string]string{"socialBase": "16764", "socialEmployee": "1575.81", "healthEmployee": "1000"},
		},
		{
			name: "hodinová mzda",
			e:    Employee{Contract: DoVP, Hourly: true, Wage: m("6.5")},
			in:   Input{Hours: m("12.5")},
			want: map[string]string{"baseWage": "81.25", "gross": "81.25"},
		},
	}
	for _, tt := range tests {
		month := tt.month
		if month == 0 {
			month = 3
		}
		r, err := Compute(tt.e, tt.in, p, month)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		got := map[string]money.Dec{"gross": r.Gross, "income": r.Income, "baseWage": r.BaseWage, "socialBase": r.SocialBase,
			"socialEmployee": r.SocialEmployee, "socialEmployer": r.SocialEmployer, "healthEmployee": r.HealthEmployee,
			"healthEmployer": r.HealthEmployer, "taxBase": r.TaxBase, "nczd": r.NCZD, "tax": r.Tax, "taxBonus": r.TaxBonus,
			"taxPaid": r.TaxPaid, "bonusPaid": r.BonusPaid, "net": r.Net, "toPay": r.ToPay, "cost": r.Cost}
		for k, v := range tt.want {
			if !got[k].Equal(m(v)) {
				t.Errorf("%s: %s = %s, chcem %s", tt.name, k, got[k], v)
			}
		}
		if tt.funds != nil {
			if len(r.Social) != len(tt.funds) {
				t.Errorf("%s: fondy %v", tt.name, r.Social)
			}
			for _, f := range tt.funds {
				if fund(r, f).Fund == "" {
					t.Errorf("%s: chýba fond %s", tt.name, f)
				}
			}
		}
	}
}

func TestComputeMisc(t *testing.T) {
	p, _ := ParamsFor(2026)
	// ročné zúčtovanie vo mzde: preplatok dane a doplatok bonusu zvýšia čistú mzdu a znížia odvod dane
	{
		e := Employee{Contract: TPP, Wage: m("1500"), NCZD: true}
		base, _ := Compute(e, Input{}, p, 3)
		r, _ := Compute(e, Input{SettleTax: m("-120.50"), SettleBonus: m("30")}, p, 3)
		if !r.Net.Equal(base.Net.Add(m("150.50"))) || !r.ToPay.Equal(base.ToPay.Add(m("150.50"))) {
			t.Errorf("ročné zúčtovanie: net %s, pred %s", r.Net, base.Net)
		}
		tb, tr := Sum([]Result{base}, nil), Sum([]Result{r}, nil)
		if !tr.Tax.Equal(tb.Tax.Sub(m("150.50"))) {
			t.Errorf("odvod dane %s, pred %s", tr.Tax, tb.Tax)
		}
	}
	// odvodová výnimka: úrazové a garančné z celej odmeny
	for _, e := range []Employee{{Contract: DoBPS, Wage: m("150"), OOP: true}, {Contract: DoPC, Regular: true, Wage: m("150"), Pension: PensionOld, OOP: true}} {
		r, _ := Compute(e, Input{}, p, 1)
		if f := fund(r, FundAccident); !f.Base.Equal(m("150")) || !f.Employer.Equal(m("1.20")) {
			t.Errorf("%s úrazové pri výnimke: %+v", e.Contract, f)
		}
		if f := fund(r, FundGuarantee); !f.Base.Equal(m("150")) || !f.Employer.Equal(m("0.37")) {
			t.Errorf("%s garančné pri výnimke: %+v", e.Contract, f)
		}
		if f := fund(r, FundOldAge); !f.Base.IsZero() {
			t.Errorf("%s starobné pri výnimke: %+v", e.Contract, f)
		}
	}
	r, _ := Compute(Employee{Contract: TPP, Wage: m("20000")}, Input{}, p, 1)
	if f := fund(r, FundAccident); !f.Base.Equal(m("20000")) || !f.Employer.Equal(m("160")) {
		t.Errorf("úrazové bez maxima: %+v", f)
	}
	r, _ = Compute(Employee{Contract: TPP, Wage: m("800"), Hours: m("40")}, Input{}, p, 1)
	if len(r.Notes) != 1 {
		t.Errorf("upozornenie na minimálnu mzdu: %v", r.Notes)
	}
	r, _ = Compute(Employee{Contract: TPP, Wage: m("800"), Hours: m("20")}, Input{}, p, 1)
	if len(r.Notes) != 0 {
		t.Errorf("polovičný úväzok: %v", r.Notes)
	}
	if _, err := ParamsFor(2024); err == nil {
		t.Error("rok 2024")
	}
	if _, err := Compute(Employee{}, Input{}, p, 13); err == nil {
		t.Error("mesiac 13")
	}
	e := Employee{Start: "2026-03-15", End: "2026-05-31"}
	for ym, want := range map[string]bool{"2026-02": false, "2026-03": true, "2026-05": true, "2026-06": false} {
		if e.ActiveIn(ym) != want {
			t.Errorf("aktívny v %s", ym)
		}
	}
	tot := Sum([]Result{{EmployeeID: 1, Gross: m("100"), SocialEmployee: m("9.4"), SocialEmployer: m("25.2"), HealthEmployee: m("5"), HealthEmployer: m("11"), TaxPaid: m("10")},
		{EmployeeID: 2, Gross: m("200"), HealthEmployee: m("10"), HealthEmployer: m("22"), BonusPaid: m("30")}}, map[int64]string{1: "25", 2: "24"})
	if !tot.Social.Equal(m("34.6")) || !tot.Health["25"].Equal(m("16")) || !tot.Health["24"].Equal(m("32")) || !tot.Tax.Equal(m("-20")) {
		t.Errorf("súčty: %+v", tot)
	}
}

func TestCompute2027(t *testing.T) {
	p, err := ParamsFor(2027)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		e    Employee
		want map[string]string
	}{
		{
			// minimálna mzda 972 €: odvody zamestnanca spolu 139,96 €, zamestnávateľa 351,85 €
			// (fondy zaokrúhlené nadol, rovnako ako v zverejnených prepočtoch)
			name: "minimálna mzda s NČZD",
			e:    Employee{Contract: TPP, Wage: m("972"), NCZD: true},
			want: map[string]string{"socialEmployee": "91.36", "healthEmployee": "48.60", "socialEmployer": "244.93",
				"healthEmployer": "106.92", "taxBase": "832.04", "nczd": "516.64", "tax": "59.92", "net": "772.12"},
		},
		{
			name: "nad maximálny vymeriavací základ 17 820 €",
			e:    Employee{Contract: TPP, Wage: m("20000")},
			want: map[string]string{"socialBase": "17820", "socialEmployee": "1675.08"},
		},
	}
	for _, tt := range tests {
		r, err := Compute(tt.e, Input{}, p, 3)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		got := map[string]money.Dec{"socialBase": r.SocialBase, "socialEmployee": r.SocialEmployee, "socialEmployer": r.SocialEmployer,
			"healthEmployee": r.HealthEmployee, "healthEmployer": r.HealthEmployer, "taxBase": r.TaxBase, "nczd": r.NCZD, "tax": r.Tax, "net": r.Net}
		for k, v := range tt.want {
			if !got[k].Equal(m(v)) {
				t.Errorf("%s: %s = %s, chcem %s", tt.name, k, got[k], v)
			}
		}
	}
	if !p.MinWage.Equal(m("972")) || !p.MinHourly.Equal(m("5.586")) || p.SickPayDays != 14 {
		t.Errorf("parametre 2027: %+v", p)
	}
}
