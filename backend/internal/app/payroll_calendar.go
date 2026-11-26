package app

import (
	"context"
	"fmt"
	"time"

	"github.com/fiskus-online/fiskus/backend/internal/bank"
	"github.com/fiskus-online/fiskus/backend/internal/calendar"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// Termíny miezd v kalendári. Mzda za mesiac sa vypláca v nasledujúcom
// mesiaci v deň určený na výplatu. V ten deň je splatné poistné do
// Sociálnej poisťovne (§ 143 ods. 2 zákona č. 461/2003 Z. z.) aj preddavok
// na zdravotné poistenie; ak deň výplaty určený nie je, poistné je splatné
// posledným dňom nasledujúceho mesiaca. Preddavky na daň zamestnávateľ
// odvedie do 5 dní od výplaty (§ 35 ods. 6 zákona o dani z príjmov).

// payDate je deň výplaty v mesiaci (deň presahujúci mesiac = posledný deň).
func payDate(year int, month time.Month, day int) invoice.Date {
	last := invoice.NewDate(year, month+1, 0).Day()
	if day > last {
		day = last
	}
	return invoice.NewDate(year, month, day)
}

// payrollDeadline je jeden termín za mzdový mesiac.
type payrollDeadline struct {
	id, title, detail string
	date              invoice.Date
	shifted           invoice.Date
	keys              []string // úhrady, ktoré termín splnia (kind alebo prefix)
	kind              calendar.Kind
}

// payrollDeadlines vráti termíny za mzdy mesiaca year/month.
func payrollDeadlines(year int, month time.Month, payDay int, days *calendar.Days) []payrollDeadline {
	period := fmt.Sprintf("%d/%d", month, year)
	pay := invoice.NewDate(year, month+1, 1) // mesiac výplaty
	id := fmt.Sprintf("%04d-%02d", year, month)
	shift := func(d invoice.Date) (invoice.Date, invoice.Date) {
		w := days.NextWorkday(d)
		if w != d {
			return w, d
		}
		return w, invoice.Date{}
	}
	if payDay <= 0 {
		last := payDate(pay.Year(), time.Month(pay.Month()), 31)
		due, orig := shift(last)
		return []payrollDeadline{{id: "pay-ins-" + id, title: "Poistné SP a ZP za mzdy " + period, date: due, shifted: orig, kind: calendar.KindTax,
			keys:   []string{"social", "health"},
			detail: "Deň výplaty nie je určený, poistné je splatné posledným dňom mesiaca. Deň výplaty doplňte v platobných údajoch miezd — preddavky dane sa odvádzajú do 5 dní od výplaty."}}
	}
	payday := payDate(pay.Year(), time.Month(pay.Month()), payDay)
	ins, insOrig := shift(payday)
	tax, taxOrig := shift(payday.AddDays(5))
	return []payrollDeadline{
		{id: "pay-wage-" + id, title: "Výplata miezd za " + period, date: payday, kind: calendar.KindPay, keys: []string{"wage"},
			detail: "Deň určený na výplatu."},
		{id: "pay-ins-" + id, title: "Poistné SP a ZP za mzdy " + period, date: ins, shifted: insOrig, kind: calendar.KindTax,
			keys: []string{"social", "health"}, detail: "Poistné je splatné v deň určený na výplatu."},
		{id: "pay-tax-" + id, title: "Preddavky dane za mzdy " + period, date: tax, shifted: taxOrig, kind: calendar.KindTax,
			keys: []string{"tax"}, detail: "Preddavky znížené o daňový bonus odveďte do 5 dní od výplaty."},
	}
}

// payrollEvents: termíny miezd mesiacov, ktorých výplata padne do rozsahu.
// Splnené sú, keď sú všetky úhrady termínu spárované s platbou z banky.
func (a *App) payrollEvents(ctx context.Context, companyID int64, from, to, today invoice.Date, days *calendar.Days) ([]calendar.Event, error) {
	b, err := a.employeeBook(ctx, companyID)
	if err != nil || len(b.List) == 0 {
		return nil, err
	}
	s, err := a.payrollPaySettings(ctx, companyID)
	if err != nil {
		return nil, err
	}
	var txs []bank.Transaction
	loaded := map[int]bool{}
	outTx := func(y int) []bank.Transaction {
		if !loaded[y] {
			loaded[y] = true
			list, err := a.Store.ListBankTransactions(ctx, companyID, store.BankFilter{Year: y})
			if err == nil {
				for _, t := range list {
					if t.Direction == bank.Out {
						txs = append(txs, t)
					}
				}
			}
		}
		return txs
	}
	start := minDate(from, today.AddDays(-31)).AddMonths(-2)
	var out []calendar.Event
	for m := invoice.NewDate(start.Year(), time.Month(start.Month()), 1); !m.After(to); m = m.AddMonths(1) {
		pp, err := a.PayrollPaymentsFor(ctx, companyID, m.Year(), int(m.Month()))
		if err != nil {
			return nil, err
		}
		if !pp.Closed && m.AddMonths(1).Before(invoice.NewDate(today.Year(), time.Month(today.Month()), 1)) {
			continue // staršie neuzavreté mesiace (pred začiatkom miezd) sa nehlásia
		}
		for _, d := range payrollDeadlines(m.Year(), time.Month(m.Month()), s.PayDay, days) {
			var amount money.Dec
			n, paid := 0, 0
			for _, p := range pp.Payments {
				if !hasKind(d.keys, p.Kind) || p.Amount.Sign() <= 0 {
					continue
				}
				n++
				amount = amount.Add(p.Amount)
				for _, t := range outTx(d.date.Year()) {
					if matchPayroll([]PayrollPayment{p}, t.CounterIBAN, t.VS, t.Amount) != nil {
						paid++
						break
					}
				}
			}
			e := calendar.Event{ID: d.id, Date: d.date, Kind: d.kind, Title: d.title, Detail: d.detail, Shifted: d.shifted,
				Link: fmt.Sprintf("/mzdy?rok=%d&mesiac=%d", m.Year(), m.Month())}
			if pp.Closed {
				if n == 0 {
					continue // nie je čo uhradiť
				}
				e.Amount, e.Curr = &amount, "EUR"
				e.Done = paid == n
			}
			if d.kind == calendar.KindTax { // výplata v hotovosti sa s bankou nespáruje
				e.Overdue = taxOverdue(e.Done, e.Date, today)
			}
			out = append(out, e)
		}
	}
	return out, nil
}

func hasKind(keys []string, kind string) bool {
	for _, k := range keys {
		if k == kind {
			return true
		}
	}
	return false
}
