package app

import (
	"errors"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/dpfo"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/payroll"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func TestPayroll(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	cid := c.ID
	var ve *invoice.ValidationError

	// kontrola údajov zamestnanca
	_, err := a.SaveEmployee(ctx, cid, payroll.Employee{RC: "123", IBAN: "SK00", Bonus: true, Contract: "x", OOP: true})
	if !errors.As(err, &ve) {
		t.Fatalf("chyby: %v", err)
	}
	fields := map[string]bool{}
	for _, f := range ve.Fields {
		fields[f.Field] = true
	}
	for _, f := range []string{"lastName", "contract", "rc", "iban", "insurer", "start", "wage", "oop", "children"} {
		if !fields[f] {
			t.Errorf("chýba chyba poľa %s (%v)", f, ve.Fields)
		}
	}

	jana, err := a.SaveEmployee(ctx, cid, payroll.Employee{FirstName: "Jana", LastName: "Nová", Contract: payroll.TPP, Insurer: "25",
		Start: "2026-01-01", Wage: m.MustParse("1500"), Hours: m.MustParse("40"), NCZD: true, Bonus: true,
		Children: []dpfo.Child{{Name: "Ema", RC: "130620/0005"}}, RC: "856215/0003"})
	if err != nil {
		t.Fatal(err)
	}
	if !jana.Regular {
		t.Error("pracovný pomer je pravidelný príjem")
	}
	peter, err := a.SaveEmployee(ctx, cid, payroll.Employee{FirstName: "Peter", LastName: "Brigádnik", Contract: payroll.DoVP, Insurer: "24",
		Start: "2026-03-01", End: "2026-03-31", Hourly: true, Wage: m.MustParse("7")})
	if err != nil {
		t.Fatal(err)
	}
	list, _ := a.Employees(ctx, cid)
	if len(list) != 2 || list[0].ID != peter.ID {
		t.Fatalf("zoznam podľa priezviska: %+v", list)
	}

	// február: iba Jana; marec aj Peter
	feb, err := a.Payroll(ctx, cid, 2026, 2)
	if err != nil || len(feb.Rows) != 1 {
		t.Fatalf("február: %+v %v", feb, err)
	}
	mar, err := a.SavePayroll(ctx, cid, 2026, 3, []payroll.Input{{EmployeeID: jana.ID, Extra: m.MustParse("200")}, {EmployeeID: peter.ID, Hours: m.MustParse("20")}})
	if err != nil {
		t.Fatal(err)
	}
	if len(mar.Rows) != 2 || !mar.Rows[0].Result.Gross.Equal(m.MustParse("140")) || !mar.Rows[1].Result.Gross.Equal(m.MustParse("1700")) {
		t.Fatalf("marec: %+v", mar.Rows)
	}
	if !mar.Totals.Health["24"].Equal(m.MustParse("22.40")) || !mar.Totals.Health["25"].Equal(m.MustParse("272")) {
		t.Errorf("zdravotné podľa poisťovní: %v", mar.Totals.Health)
	}
	if _, err := a.SavePayroll(ctx, cid, 2026, 3, []payroll.Input{{EmployeeID: jana.ID, Extra: m.MustParse("-1")}}); !errors.As(err, &ve) {
		t.Fatalf("záporná suma: %v", err)
	}
	if _, err := a.Payroll(ctx, cid, 2024, 1); !errors.As(err, &ve) {
		t.Fatalf("rok 2024: %v", err)
	}

	// uzavretie drží výsledky aj po zmene mzdy
	closed, err := a.ClosePayroll(ctx, cid, 2026, 3)
	if err != nil || !closed.Closed {
		t.Fatalf("uzavretie: %v", err)
	}
	jana.Wage = m.MustParse("2000")
	if _, err := a.SaveEmployee(ctx, cid, jana); err != nil {
		t.Fatal(err)
	}
	again, _ := a.Payroll(ctx, cid, 2026, 3)
	if !again.Rows[1].Result.Gross.Equal(m.MustParse("1700")) || !again.Totals.ToPay.Equal(closed.Totals.ToPay) {
		t.Error("uzavretý mesiac sa prepočítal")
	}
	if _, err := a.SavePayroll(ctx, cid, 2026, 3, nil); !errors.As(err, &ve) {
		t.Fatalf("zmena v uzavretom mesiaci: %v", err)
	}
	if err := a.DeleteEmployee(ctx, cid, peter.ID); !errors.As(err, &ve) {
		t.Fatalf("zmazanie s uzavretou mzdou: %v", err)
	}
	if _, err := a.ReopenPayroll(ctx, cid, 2026, 3, "x"); !errors.As(err, &ve) {
		t.Fatalf("krátky dôvod: %v", err)
	}
	open, err := a.ReopenPayroll(ctx, cid, 2026, 3, "oprava odmeny")
	if err != nil || open.Closed || !open.Rows[1].Result.Gross.Equal(m.MustParse("2200")) {
		t.Fatalf("otvorenie: %+v %v", open, err)
	}
	if err := a.DeleteEmployee(ctx, cid, peter.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteEmployee(ctx, cid, peter.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("druhé zmazanie: %v", err)
	}
}
