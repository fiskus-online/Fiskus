package demo

import (
	"context"
	"fmt"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/dpfo"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/payroll"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// seedPayroll pridá zamestnanca s deťmi, dohodárku a študenta na brigáde;
// predchádzajúci mesiac je uzavretý, aktuálny otvorený s odmenou a hodinami.
func seedPayroll(ctx context.Context, a *app.App, c store.Company) error {
	today := a.Today()
	if _, err := payroll.ParamsFor(today.Year()); err != nil {
		return nil // mzdy za tento rok Fiškus ešte nepočíta
	}
	start := fmt.Sprintf("%d-01-01", today.Year())
	list := []payroll.Employee{
		{FirstName: "Martin", LastName: "Kováč", Position: "elektrikár", Contract: payroll.TPP, Insurer: "25", Start: "2024-03-01",
			Wage: m.MustParse("1450"), Hours: m.MustParse("40"), NCZD: true, Bonus: true, RC: "880512/0005",
			Children: []dpfo.Child{{Name: "Ema", RC: "130620/0005"}, {Name: "Samuel", RC: "200304/0006"}},
			Street:   "Okružná 14", City: "Žilina", Zip: "01001", IBAN: "SK3112000000198742637541"},
		{FirstName: "Lucia", LastName: "Hronská", Position: "administratíva", Contract: payroll.DoPC, Regular: true, Insurer: "24", Start: start,
			Wage: m.MustParse("420"), Hours: m.MustParse("10"), NCZD: false, City: "Žilina"},
		{FirstName: "Tomáš", LastName: "Bielik", Position: "pomocník na montážach", Contract: payroll.DoBPS, Insurer: "27", Start: start,
			Hourly: true, Wage: m.MustParse("7"), OOP: true, NCZD: true, City: "Bytča"},
	}
	ids := make([]int64, len(list))
	for i, e := range list {
		s, err := a.SaveEmployee(ctx, c.ID, e)
		if err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		ids[i] = s.ID
	}
	prev := today.AddMonths(-1)
	if prev.Year() == today.Year() {
		if _, err := a.SavePayroll(ctx, c.ID, prev.Year(), prev.Month(), []payroll.Input{{EmployeeID: ids[2], Hours: m.MustParse("24")}}); err != nil {
			return err
		}
		if _, err := a.ClosePayroll(ctx, c.ID, prev.Year(), prev.Month()); err != nil {
			return err
		}
	}
	_, err := a.SavePayroll(ctx, c.ID, today.Year(), today.Month(), []payroll.Input{
		{EmployeeID: ids[0], Extra: m.MustParse("150"), Note: "odmena za montáž v Martine"},
		{EmployeeID: ids[2], Hours: m.MustParse("36")},
	})
	return err
}
