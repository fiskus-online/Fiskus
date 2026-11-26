package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/calendar"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/payroll"
)

func TestMVPPType(t *testing.T) {
	tests := []struct {
		name    string
		e       payroll.Employee
		vynimka bool
		want    string
		err     bool
	}{
		{"pracovný pomer", payroll.Employee{Contract: payroll.TPP}, false, "ZEC", false},
		{"DoVP pravidelný", payroll.Employee{Contract: payroll.DoVP, Regular: true}, false, "ZECD1", false},
		{"DoVP nepravidelný", payroll.Employee{Contract: payroll.DoVP}, false, "ZECD1N", false},
		{"DoPČ pravidelný", payroll.Employee{Contract: payroll.DoPC, Regular: true}, false, "ZECD2", false},
		{"DoPČ dôchodca s výnimkou", payroll.Employee{Contract: payroll.DoPC, Regular: true}, true, "ZECD2V", false},
		{"DoPČ nepravidelný s výnimkou", payroll.Employee{Contract: payroll.DoPC}, true, "", true},
		{"DoBPŠ pravidelný", payroll.Employee{Contract: payroll.DoBPS, Regular: true}, false, "ZECD3", false},
		{"DoBPŠ nepravidelný", payroll.Employee{Contract: payroll.DoBPS}, false, "ZECD3N", false},
		{"DoBPŠ výnimka", payroll.Employee{Contract: payroll.DoBPS, Regular: true}, true, "ZECD3V", false},
		{"DoBPŠ nepravidelný výnimka", payroll.Employee{Contract: payroll.DoBPS}, true, "ZECD3NV", false},
	}
	for _, tt := range tests {
		got, err := mvppType(tt.e, tt.vynimka)
		if got != tt.want || (err != nil) != tt.err {
			t.Errorf("%s: %q %v", tt.name, got, err)
		}
	}
}

func TestMVPPDaysHours(t *testing.T) {
	days := calendar.NewDays(nil)
	tests := []struct {
		name  string
		e     payroll.Employee
		in    payroll.Input
		days  int
		hours string
	}{
		// október 2026: 22 pracovných dní (bez sviatkov v teste)
		{"celý mesiac", payroll.Employee{Start: "2025-01-01", Hours: m.MustParse("40")}, payroll.Input{}, 31, "176.00"},
		{"nástup 15.", payroll.Employee{Start: "2026-10-15", Hours: m.MustParse("40")}, payroll.Input{}, 17, "96.00"},
		{"koniec 9.", payroll.Employee{Start: "2025-01-01", End: "2026-10-09", Hours: m.MustParse("20")}, payroll.Input{}, 9, "28.00"},
		{"hodinová mzda", payroll.Employee{Start: "2025-01-01", Hourly: true}, payroll.Input{Hours: m.MustParse("12.5")}, 31, "12.50"},
		{"bez pracovného času", payroll.Employee{Start: "2025-01-01"}, payroll.Input{}, 31, "999"},
	}
	for _, tt := range tests {
		if d := mvppDays(tt.e, 2026, 10); d != tt.days {
			t.Errorf("%s: dni %d", tt.name, d)
		}
		if h := mvppHours(tt.e, tt.in, 2026, 10, days); h != tt.hours {
			t.Errorf("%s: hodiny %s", tt.name, h)
		}
	}
}

func TestPayrollMVPP(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	c.ICO, c.IBAN = "36123455", ibanFirm
	if _, err := a.UpdateCompany(ctx, c); err != nil {
		t.Fatal(err)
	}
	a.Today = func() invoice.Date { return invoice.MustDate("2026-11-05") }
	jana, err := a.SaveEmployee(ctx, c.ID, payroll.Employee{FirstName: "Jana", LastName: "Nová", Contract: payroll.TPP, Insurer: "25",
		Start: "2026-01-01", Wage: m.MustParse("1500"), Hours: m.MustParse("40"), NCZD: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveEmployee(ctx, c.ID, payroll.Employee{FirstName: "Peter", LastName: "Mladý", Contract: payroll.DoBPS, Regular: true,
		OOP: true, Insurer: "24", Start: "2026-09-01", Wage: m.MustParse("400"), Hours: m.MustParse("10"), RC: "060101/1004"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ClosePayroll(ctx, c.ID, 2026, 10); err != nil {
		t.Fatal(err)
	}
	var me *MVPPError
	_, _, err = a.PayrollMVPP(ctx, c.ID, 2026, 10)
	if !errors.As(err, &me) || len(me.Problems) != 4 {
		t.Fatalf("chýbajúce údaje: %v", err)
	}
	jana.RC = "8551011007"
	if _, err := a.SaveEmployee(ctx, c.ID, jana); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SavePayrollPaySettings(ctx, c.ID, PayrollPaySettings{SocialVS: "1234567890", PayDay: 12,
		ContactFirst: "Eva", ContactLast: "Mzdová", ContactPhone: "0900 000 000", ContactEmail: "mzdy@example.sk"}); err != nil {
		t.Fatal(err)
	}
	// rodné číslo doplnené po uzavretí mesiaca sa použije
	name, data, err := a.PayrollMVPP(ctx, c.ID, 2026, 10)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	v, _ := a.Payroll(ctx, c.ID, 2026, 10)
	var total m.Dec
	for _, r := range v.Rows {
		total = total.Add(r.Result.SocialEmployee).Add(r.Result.SocialEmployer)
	}
	want := []string{
		`<mvpp xmlns="http://socpoist.sk/xsd/mvpp2026">`, "<typDoc>MVP00001</typDoc>", "<obdobie>102026</obdobie>",
		"<denVyplaty>12.11.2026</denVyplaty>", "<typVykazu>R</typVykazu>", "<variabilnySymbol>1234567890</variabilnySymbol>",
		"<ico>36123455</ico>", "<iban>" + ibanFirm + "</iban>", "<np>", "<npZamtel>", "<rfs>", "<rfsZamtel>",
		"<spoluPoistne>" + total.String() + "</spoluPoistne>", "<priezvisko>Mzdová</priezvisko>", "<zostavil>Eva Mzdová</zostavil>",
		"<datum>05.11.2026</datum>",
		`pc="2" rc="8551011007" pocDni="31" typZec="ZEC" vynimkaVZ="0" vzNp="1500.00"`,
		`rc="0601011004" pocDni="31" typZec="ZECD3V" vynimkaVZ="1" vzNp="0.00" vzSp="200.00"`,
		`spZamnec="8.00"`, `pocetHodin="176.00"`,
	}
	for _, w := range want {
		if !strings.Contains(s, w) {
			t.Errorf("chýba %q", w)
		}
	}
	for _, bad := range []string{"<rfsZamnec>", "upZamnec="} {
		if strings.Contains(s, bad) {
			t.Errorf("nemá byť %q", bad)
		}
	}
	if name != "mvpp-2026-10.xml" {
		t.Errorf("názov %s", name)
	}
	if t.Failed() {
		t.Log(s)
	}
}
