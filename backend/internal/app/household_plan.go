package app

import (
	"context"
	"fmt"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/household"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Plán platieb domácnosti po mesiacoch: čo a kedy treba zaplatiť (splátky,
// pravidelné platby, pokuty, škola…), koľko to je za mesiac a čo treba
// doplatiť po splatnosti. Zostavuje sa zo splátkových kalendárov záväzkov.

// PlanItem je jedna platba v pláne.
type PlanItem struct {
	UpcomingDue
	Income bool `json:"income"` // príjem (požičané niekomu)
}

// PlanMonth sú platby jedného mesiaca.
type PlanMonth struct {
	Month   string     `json:"month"` // 2026-11
	Items   []PlanItem `json:"items"`
	Pay     money.Dec  `json:"pay"`     // treba zaplatiť
	Receive money.Dec  `json:"receive"` // majú vrátiť
}

// HouseholdPlanView je plán platieb na niekoľko mesiacov.
type HouseholdPlanView struct {
	From           string      `json:"from"`
	Overdue        []PlanItem  `json:"overdue"` // po splatnosti — treba doplatiť
	OverduePay     money.Dec   `json:"overduePay"`
	OverdueReceive money.Dec   `json:"overdueReceive"`
	Months         []PlanMonth `json:"months"`
	Pay            money.Dec   `json:"pay"` // spolu za mesiace (bez doplatkov)
	Receive        money.Dec   `json:"receive"`
}

// HouseholdPlan zostaví plán platieb od mesiaca from (rok, mesiac) na months mesiacov.
func (a *App) HouseholdPlan(ctx context.Context, companyID int64, year, month, months int) (HouseholdPlanView, error) {
	if month < 1 || month > 12 || year < 2000 || year > 2200 {
		return HouseholdPlanView{}, vErr("from", "Zadajte mesiac v tvare RRRR-MM")
	}
	if months < 1 || months > 24 {
		months = 12
	}
	start := invoice.NewDate(year, time.Month(month), 1)
	end := start.AddMonths(months).AddDays(-1)
	dues, err := a.obligationDues(ctx, companyID, end)
	if err != nil {
		return HouseholdPlanView{}, err
	}
	v := HouseholdPlanView{From: fmt.Sprintf("%04d-%02d", year, month), Overdue: []PlanItem{}}
	idx := map[string]int{}
	for i := 0; i < months; i++ {
		d := start.AddMonths(i)
		key := fmt.Sprintf("%04d-%02d", d.Year(), d.Month())
		idx[key] = i
		v.Months = append(v.Months, PlanMonth{Month: key, Items: []PlanItem{}})
	}
	for _, d := range dues {
		it := PlanItem{UpcomingDue: d, Income: d.Kind == household.Lent}
		if d.Status == household.DueOverdue {
			v.Overdue = append(v.Overdue, it)
			if it.Income {
				v.OverdueReceive = v.OverdueReceive.Add(d.Rest)
			} else {
				v.OverduePay = v.OverduePay.Add(d.Rest)
			}
			continue
		}
		i, ok := idx[fmt.Sprintf("%04d-%02d", d.Date.Year(), d.Date.Month())]
		if !ok {
			continue // pred začiatkom plánu
		}
		m := &v.Months[i]
		m.Items = append(m.Items, it)
		if it.Income {
			m.Receive = m.Receive.Add(d.Rest)
			v.Receive = v.Receive.Add(d.Rest)
		} else {
			m.Pay = m.Pay.Add(d.Rest)
			v.Pay = v.Pay.Add(d.Rest)
		}
	}
	return v, nil
}
