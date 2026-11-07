package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/calc"
	"github.com/fiskus-online/fiskus/backend/internal/dpfo"
	"github.com/fiskus-online/fiskus/backend/internal/fx"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/payroll"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

// Kalkulačky fungujú aj bez založenej firmy: nepotrebujú companyID a nič
// neukladajú. Zákonné hodnoty (sadzby DPH, ECB, prirážky) sú z Legislatívy.

// CalcInfo sú číselníky pre kalkulačky.
type CalcInfo struct {
	VATRates   []vat.Rate     `json:"vatRates"`
	WageYears  []int          `json:"wageYears"`
	Currencies []fx.Currency  `json:"currencies"`
	Today      invoice.Date   `json:"today"`
	Law        calcLawSummary `json:"law"`
}

type calcLawSummary struct {
	ECB            money.Dec `json:"ecb"`
	BusinessMargin money.Dec `json:"businessMargin"`
	ConsumerMargin money.Dec `json:"consumerMargin"`
	FlatFee        money.Dec `json:"flatFee"`
}

// CalcInfo vráti sadzby DPH platné dnes, roky mzdovej kalkulačky a meny.
func (a *App) CalcInfo(ctx context.Context) CalcInfo {
	today := a.Today()
	law := a.Law()
	return CalcInfo{VATRates: vat.RatesOn(today.String()), WageYears: payroll.Years(), Currencies: fx.Currencies, Today: today,
		Law: calcLawSummary{ECB: law.ECB.On(today), BusinessMargin: law.BusinessMargin, ConsumerMargin: law.ConsumerMargin, FlatFee: law.FlatFee}}
}

// calcErr prevedie chybu vstupu na chybu poľa (422).
func calcErr(err error) error {
	var fe *calc.FieldError
	if errors.As(err, &fe) {
		return vErr(fe.Field, fe.Message)
	}
	return err
}

// CalcVAT rozpočíta sumu na základ a DPH.
func (a *App) CalcVAT(in calc.VATInput) (calc.VATResult, error) {
	r, err := calc.VAT(in)
	return r, calcErr(err)
}

// CalcLateInterest vypočíta úrok z omeškania so zákonnou sadzbou z Legislatívy.
func (a *App) CalcLateInterest(in calc.LateInput) (calc.LateResult, error) {
	r, err := calc.LateInterest(in, a.Law())
	return r, calcErr(err)
}

// CalcLoan vypočíta splátku úveru, RPMN a splátkový kalendár.
func (a *App) CalcLoan(in calc.LoanInput) (calc.LoanResult, error) {
	r, err := calc.Loan(in)
	return r, calcErr(err)
}

// CalcSavings vypočíta sporenie.
func (a *App) CalcSavings(in calc.SavingsInput) (calc.SavingsResult, error) {
	r, err := calc.Savings(in)
	return r, calcErr(err)
}

// WageInput je vstup mzdovej kalkulačky (hrubá → čistá).
type WageInput struct {
	Year     int              `json:"year"`
	Month    int              `json:"month"`
	Gross    money.Dec        `json:"gross"`
	Contract payroll.Contract `json:"contract"`
	Regular  bool             `json:"regular"` // dohoda s pravidelným príjmom
	NCZD     bool             `json:"nczd"`
	Pension  payroll.Pension  `json:"pension"`
	OOP      bool             `json:"oop"`
	ZTP      bool             `json:"ztp"`
	// Children sú veky detí (rokov) na daňový bonus.
	Children []int `json:"children"`
}

// CalcWage vypočíta čistú mzdu a cenu práce rovnako ako modul Mzdy.
func (a *App) CalcWage(in WageInput) (payroll.Result, error) {
	if in.Gross.Sign() <= 0 {
		return payroll.Result{}, vErr("gross", "Zadajte hrubú mzdu")
	}
	if in.Month == 0 {
		in.Month = int(a.Today().Month())
	}
	if in.Contract == "" {
		in.Contract = payroll.TPP
	}
	if !in.Contract.Valid() {
		return payroll.Result{}, vErr("contract", "Neznámy druh pracovného vzťahu")
	}
	p, err := payroll.ParamsFor(in.Year)
	if err != nil {
		return payroll.Result{}, vErr("year", fmt.Sprintf("Mzdy sa dajú počítať za roky %s", joinInts(payroll.Years())))
	}
	e := payroll.Employee{Contract: in.Contract, Regular: in.Regular, Wage: in.Gross, NCZD: in.NCZD, Pension: in.Pension, OOP: in.OOP, ZTP: in.ZTP,
		Bonus: len(in.Children) > 0, Start: fmt.Sprintf("%d-01-01", in.Year)}
	for i, age := range in.Children {
		if age < 0 || age > 25 {
			return payroll.Result{}, vErr(fmt.Sprintf("children[%d]", i), "Vek dieťaťa je 0 až 25 rokov")
		}
		e.Children = append(e.Children, dpfo.Child{RC: birthRC(in.Year - age), Months: allMonths})
	}
	r, err := payroll.Compute(e, payroll.Input{}, p, in.Month)
	if err != nil {
		return r, vErr("gross", err.Error())
	}
	return r, nil
}

// birthRC je rodné číslo dieťaťa narodeného 1. januára (iba nosič dátumu
// narodenia, z ktorého sa určí vek) so správnym kontrolným súčtom.
func birthRC(year int) string {
	base := int64(year%100)*100_000_000 + 1_010_000
	return fmt.Sprintf("%010d", base+(11-base%11)%11)
}

var allMonths = [12]bool{true, true, true, true, true, true, true, true, true, true, true, true}

func joinInts(xs []int) string {
	s := make([]string, len(xs))
	for i, x := range xs {
		s[i] = fmt.Sprint(x)
	}
	return strings.Join(s, ", ")
}

// FXInput je prepočet medzi eurom a cudzou menou kurzom ECB.
type FXInput struct {
	Amount   money.Dec    `json:"amount"`
	Currency string       `json:"currency"`
	Date     invoice.Date `json:"date"`
	ToEUR    bool         `json:"toEur"` // suma je v cudzej mene
}

// FXResult je výsledok prepočtu.
type FXResult struct {
	Rate     money.Dec `json:"rate"`     // jednotiek meny za 1 EUR
	RateDate string    `json:"rateDate"` // deň vyhlásenia kurzu
	Result   money.Dec `json:"result"`
}

// CalcFX prepočíta sumu kurzom ECB vyhláseným v daný deň alebo naposledy pred ním.
func (a *App) CalcFX(ctx context.Context, in FXInput) (FXResult, error) {
	var r FXResult
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if !fx.Known(in.Currency) || in.Currency == "EUR" {
		return r, vErr("currency", "Vyberte menu")
	}
	if in.Amount.Sign() <= 0 {
		return r, vErr("amount", "Zadajte sumu")
	}
	if in.Date.IsZero() {
		in.Date = a.Today()
	}
	rates, err := a.Store.ListRates(ctx, fx.Filter{Currency: in.Currency})
	if err != nil {
		return r, err
	}
	best, found := fx.Before(rates, in.Currency, in.Date.AddDays(1).String())
	if !found {
		return r, vErr("date", "Kurz "+in.Currency+" k "+in.Date.Display()+" nie je stiahnutý (Nastavenia → Kurzy mien)")
	}
	r.Rate, r.RateDate = best.Rate, best.Date
	if in.ToEUR {
		r.Result = in.Amount.MulRatio(money.FromInt(1), best.Rate).RoundCents()
	} else {
		r.Result = in.Amount.Mul(best.Rate).RoundCents()
	}
	return r, nil
}
