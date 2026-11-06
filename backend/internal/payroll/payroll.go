// Package payroll počíta mzdy zamestnancov a odmeny z dohôd: poistné do
// Sociálnej poisťovne po fondoch, zdravotné poistenie, preddavok na daň,
// daňový bonus, čistú mzdu a cenu práce.
//
// Zdroje hodnôt: Sociálna poisťovňa (tabuľky platenia poistného od 1. 1. 2026,
// vymeriavacie základy 2026), zákon 580/2004 (zdravotné poistenie 2026 – 2027
// podľa konsolidačného balíka), podpora.financnasprava.sk 260857 (NČZD 2026).
// Daň a daňový bonus počíta balík dpfo (rovnaké hodnoty ako priznanie).
package payroll

import (
	"errors"
	"fmt"

	"github.com/palo-krnac/fiscus/backend/internal/dpfo"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Contract je druh pracovného vzťahu.
type Contract string

const (
	TPP   Contract = "tpp"   // pracovný pomer
	DoPC  Contract = "dopc"  // dohoda o pracovnej činnosti
	DoVP  Contract = "dovp"  // dohoda o vykonaní práce
	DoBPS Contract = "dobps" // dohoda o brigádnickej práci študentov
)

// Contracts sú druhy pracovných vzťahov v poradí na výber.
var Contracts = []Contract{TPP, DoPC, DoVP, DoBPS}

// Valid je známy druh vzťahu.
func (c Contract) Valid() bool {
	for _, x := range Contracts {
		if x == c {
			return true
		}
	}
	return false
}

// Agreement je dohoda (nie pracovný pomer).
func (c Contract) Agreement() bool { return c == DoPC || c == DoVP || c == DoBPS }

// Pension je dôchodok, ktorý mení poistenie zamestnanca.
type Pension string

const (
	PensionNone     Pension = ""
	PensionOld      Pension = "old"      // starobný, predčasný starobný alebo výsluhový po dôchodkovom veku
	PensionDisabled Pension = "disabled" // invalidný, pokles schopnosti nad 70 %
)

// Fondy sociálneho poistenia.
const (
	FundSickness     = "nemocenske"
	FundOldAge       = "starobne"
	FundDisability   = "invalidne"
	FundUnemployment = "nezamestnanost"
	FundShortTime    = "kurzarbeit" // financovanie podpory v čase skrátenej práce
	FundGuarantee    = "garancne"
	FundAccident     = "urazove"
	FundReserve      = "rfs" // rezervný fond solidarity
)

// FundNames sú názvy fondov na výplatnej páske.
var FundNames = map[string]string{
	FundSickness:     "Nemocenské",
	FundOldAge:       "Starobné",
	FundDisability:   "Invalidné",
	FundUnemployment: "V nezamestnanosti",
	FundShortTime:    "Podpora pri skrátenej práci",
	FundGuarantee:    "Garančné",
	FundAccident:     "Úrazové",
	FundReserve:      "Rezervný fond solidarity",
}

// Rate je sadzba fondu v percentách.
type Rate struct {
	Fund               string
	Employee, Employer money.Dec
	Capped             bool // vymeriavací základ je obmedzený maximom (úrazové nie)
}

// Params sú zákonné hodnoty za rok.
type Params struct {
	Year        int
	Social      []Rate
	SocialMax   money.Dec // maximálny mesačný vymeriavací základ
	OOP         money.Dec // odvodová odpočítateľná položka (dohody dôchodcov, DoBPŠ)
	Health      money.Dec // zdravotné poistenie zamestnanca (%)
	HealthER    money.Dec // zdravotné poistenie zamestnávateľa (%)
	HealthZTP   money.Dec // zamestnanec so zdravotným postihnutím (%)
	HealthZTPER money.Dec
	MinWage     money.Dec // minimálna mesačná mzda
	MinHourly   money.Dec // minimálna hodinová mzda
	SickPayDays int       // náhradu príjmu pri PN platí zamestnávateľ do tohto dňa
	TaxParams   dpfo.Params
}

var m = money.MustParse

func socialRates() []Rate {
	return []Rate{
		{FundSickness, m("1.4"), m("1.4"), true},
		{FundOldAge, m("4"), m("14"), true},
		{FundDisability, m("3"), m("3"), true},
		{FundUnemployment, m("1"), m("0.5"), true},
		{FundShortTime, money.Zero, m("0.5"), true},
		{FundGuarantee, money.Zero, m("0.25"), true},
		{FundAccident, money.Zero, m("0.8"), false},
		{FundReserve, money.Zero, m("4.75"), true},
	}
}

var years = map[int]Params{
	2025: {Year: 2025, Social: socialRates(), SocialMax: m("15730"), OOP: m("200"),
		Health: m("4"), HealthER: m("11"), HealthZTP: m("2"), HealthZTPER: m("5.5"),
		MinWage: m("816"), MinHourly: m("4.69"), SickPayDays: 10},
	2026: {Year: 2026, Social: socialRates(), SocialMax: m("16764"), OOP: m("200"),
		Health: m("5"), HealthER: m("11"), HealthZTP: m("2.5"), HealthZTPER: m("5.5"),
		MinWage: m("915"), MinHourly: m("5.259"), SickPayDays: 14},
}

// ErrYear — mzdy za rok Fiškus nevie vypočítať.
var ErrYear = errors.New("mzdy za tento rok zatiaľ Fiškus nepočíta")

// ParamsFor vráti zákonné hodnoty za rok.
func ParamsFor(year int) (Params, error) {
	p, ok := years[year]
	if !ok {
		return Params{}, fmt.Errorf("%w (%d)", ErrYear, year)
	}
	tp, err := dpfo.ParamsFor(year)
	if err != nil {
		return Params{}, err
	}
	p.TaxParams = tp
	return p, nil
}

// Years sú roky, za ktoré sa dajú počítať mzdy.
func Years() []int { return []int{2025, 2026} }

// Employee je zamestnanec alebo dohodár.
type Employee struct {
	ID        int64        `json:"id"`
	FirstName string       `json:"firstName"`
	LastName  string       `json:"lastName"`
	Title     string       `json:"title"`
	RC        string       `json:"rc"` // rodné číslo
	Street    string       `json:"street"`
	City      string       `json:"city"`
	Zip       string       `json:"zip"`
	IBAN      string       `json:"iban"`
	Email     string       `json:"email"`
	Insurer   string       `json:"insurer"` // zdravotná poisťovňa: 24 Dôvera, 25 VšZP, 27 Union
	Contract  Contract     `json:"contract"`
	Regular   bool         `json:"regular"` // dohoda s pravidelným mesačným príjmom
	Start     string       `json:"start"`   // RRRR-MM-DD
	End       string       `json:"end"`     // prázdne = na neurčito
	Position  string       `json:"position"`
	Hourly    bool         `json:"hourly"` // hodinová mzda (inak mesačná)
	Wage      money.Dec    `json:"wage"`   // mesačná mzda alebo hodinová sadzba
	Hours     money.Dec    `json:"hours"`  // týždenný pracovný čas
	NCZD      bool         `json:"nczd"`   // podpísané vyhlásenie na NČZD
	Bonus     bool         `json:"bonus"`  // uplatňuje daňový bonus na deti
	Children  []dpfo.Child `json:"children"`
	Pension   Pension      `json:"pension"`
	ZTP       bool         `json:"ztp"` // zdravotné postihnutie — znížená sadzba zdravotného poistenia
	OOP       bool         `json:"oop"` // uplatňuje odvodovú výnimku 200 € (dohoda dôchodcu, DoBPŠ)
	Note      string       `json:"note"`
}

// Name je meno na zoznamoch (priezvisko, meno).
func (e Employee) Name() string {
	n := e.LastName
	if e.FirstName != "" {
		n += " " + e.FirstName
	}
	return n
}

// ActiveIn: pracovný vzťah trvá aspoň časť mesiaca (RRRR-MM).
func (e Employee) ActiveIn(ym string) bool {
	if e.Start != "" && e.Start[:7] > ym {
		return false
	}
	return e.End == "" || e.End[:7] >= ym
}

// Input sú údaje zamestnanca za mesiac.
type Input struct {
	EmployeeID int64     `json:"employeeId"`
	Hours      money.Dec `json:"hours"`     // odpracované hodiny (hodinová mzda)
	Base       money.Dec `json:"base"`      // základná mzda — iná ako podľa zmluvy (napr. časť mesiaca); 0 = podľa zmluvy
	Extra      money.Dec `json:"extra"`     // odmeny, prémie, príplatky
	Leave      money.Dec `json:"leave"`     // náhrada mzdy (dovolenka, sviatok, prekážky)
	SickPay    money.Dec `json:"sickPay"`   // náhrada príjmu pri PN (bez poistného)
	Deduction  money.Dec `json:"deduction"` // zrážky z čistej mzdy
	Note       string    `json:"note"`
}

// Fund je poistné jedného fondu.
type Fund struct {
	Fund     string    `json:"fund"`
	Name     string    `json:"name"`
	Base     money.Dec `json:"base"`
	Employee money.Dec `json:"employee"`
	Employer money.Dec `json:"employer"`
}

// Result je mzda zamestnanca za mesiac.
type Result struct {
	EmployeeID     int64     `json:"employeeId"`
	BaseWage       money.Dec `json:"baseWage"`   // základná mzda
	Gross          money.Dec `json:"gross"`      // hrubá mzda (vymeriavací základ)
	SickPay        money.Dec `json:"sickPay"`    // náhrada príjmu pri PN
	Income         money.Dec `json:"income"`     // zdaniteľný príjem (aj náhrada pri PN)
	SocialBase     money.Dec `json:"socialBase"` // vymeriavací základ po výnimke a maxime
	Social         []Fund    `json:"social"`
	SocialEmployee money.Dec `json:"socialEmployee"`
	SocialEmployer money.Dec `json:"socialEmployer"`
	HealthBase     money.Dec `json:"healthBase"`
	HealthEmployee money.Dec `json:"healthEmployee"`
	HealthEmployer money.Dec `json:"healthEmployer"`
	Contrib        money.Dec `json:"contrib"`   // poistné zamestnanca spolu (sociálne + zdravotné)
	ContribER      money.Dec `json:"contribER"` // poistné zamestnávateľa spolu
	TaxBase        money.Dec `json:"taxBase"`
	NCZD           money.Dec `json:"nczd"`
	Tax            money.Dec `json:"tax"`       // preddavok pred bonusom
	TaxBonus       money.Dec `json:"taxBonus"`  // daňový bonus na deti
	TaxPaid        money.Dec `json:"taxPaid"`   // preddavok na úhradu (po bonuse, nie záporný)
	BonusPaid      money.Dec `json:"bonusPaid"` // bonus nad preddavok — vyplatí zamestnávateľ
	Net            money.Dec `json:"net"`       // čistá mzda
	Deduction      money.Dec `json:"deduction"`
	ToPay          money.Dec `json:"toPay"` // na výplatu
	Cost           money.Dec `json:"cost"`  // cena práce
	Notes          []string  `json:"notes"`
}

// applies: platí fond pre zamestnanca? (§ 4 a § 5 zákona o sociálnom poistení)
func applies(fund string, e Employee) bool {
	// dohoda s nepravidelným príjmom a DoBPŠ: bez nemocenského a poistenia v nezamestnanosti
	irregular := e.Contract == DoBPS || (e.Contract.Agreement() && !e.Regular)
	switch fund {
	case FundSickness:
		return !irregular
	case FundUnemployment, FundShortTime:
		return !irregular && e.Pension == PensionNone
	case FundDisability:
		return e.Pension != PensionOld
	}
	return true
}

// Compute vypočíta mzdu zamestnanca za mesiac (1 – 12).
func Compute(e Employee, in Input, p Params, month int) (Result, error) {
	r := Result{EmployeeID: e.ID, Notes: []string{}}
	if month < 1 || month > 12 {
		return r, fmt.Errorf("neplatný mesiac %d", month)
	}
	switch {
	case !in.Base.IsZero():
		r.BaseWage = in.Base
	case e.Hourly:
		r.BaseWage = e.Wage.Mul(in.Hours).RoundCents()
	default:
		r.BaseWage = e.Wage
	}
	r.Gross = r.BaseWage.Add(in.Extra).Add(in.Leave)
	r.SickPay = in.SickPay
	r.Income = r.Gross.Add(in.SickPay)

	// sociálne poistenie
	sb := r.Gross
	if e.OOP && (e.Contract == DoBPS || (e.Contract.Agreement() && e.Pension != PensionNone)) {
		sb = maxDec(money.Zero, sb.Sub(p.OOP))
		r.Notes = append(r.Notes, fmt.Sprintf("Odvodová výnimka %s € — sociálne poistenie zo sumy nad ňu.", p.OOP.Display()))
	}
	capped := minDec(sb, p.SocialMax)
	if sb.Cmp(p.SocialMax) > 0 {
		r.Notes = append(r.Notes, fmt.Sprintf("Sociálne poistenie z maximálneho vymeriavacieho základu %s €.", p.SocialMax.Display()))
	}
	r.SocialBase = capped
	for _, rt := range p.Social {
		if !applies(rt.Fund, e) {
			continue
		}
		base := capped
		if !rt.Capped {
			base = sb
		}
		f := Fund{Fund: rt.Fund, Name: FundNames[rt.Fund], Base: base,
			Employee: base.PctFloorCents(rt.Employee), Employer: base.PctFloorCents(rt.Employer)}
		r.Social = append(r.Social, f)
		r.SocialEmployee = r.SocialEmployee.Add(f.Employee)
		r.SocialEmployer = r.SocialEmployer.Add(f.Employer)
	}

	// zdravotné poistenie (bez maxima, aj z dohôd)
	hp, hpER := p.Health, p.HealthER
	if e.ZTP {
		hp, hpER = p.HealthZTP, p.HealthZTPER
	}
	r.HealthBase = r.Gross
	r.HealthEmployee = r.Gross.PctFloorCents(hp)
	r.HealthEmployer = r.Gross.PctFloorCents(hpER)

	r.Contrib = r.SocialEmployee.Add(r.HealthEmployee)
	r.ContribER = r.SocialEmployer.Add(r.HealthEmployer)

	// preddavok na daň a daňový bonus (§ 35, § 33)
	tp := p.TaxParams
	r.TaxBase = maxDec(money.Zero, r.Income.Sub(r.SocialEmployee).Sub(r.HealthEmployee))
	if e.NCZD {
		r.NCZD = minDec(dpfo.MonthlyNCZD(r.TaxBase, tp), r.TaxBase)
	}
	r.Tax = dpfo.MonthlyTax(r.TaxBase.Sub(r.NCZD), tp)
	if e.Bonus && e.NCZD && len(e.Children) > 0 {
		b, err := dpfo.MonthlyBonus(e.Children, r.TaxBase, month, tp)
		if err != nil {
			return r, err
		}
		r.TaxBonus = b
	} else if e.Bonus && !e.NCZD {
		r.Notes = append(r.Notes, "Daňový bonus sa uplatní iba s podpísaným vyhlásením na nezdaniteľnú časť.")
	}
	if r.TaxBonus.Cmp(r.Tax) > 0 {
		r.BonusPaid = r.TaxBonus.Sub(r.Tax)
	} else {
		r.TaxPaid = r.Tax.Sub(r.TaxBonus)
	}

	r.Net = r.Income.Sub(r.SocialEmployee).Sub(r.HealthEmployee).Sub(r.TaxPaid).Add(r.BonusPaid)
	r.Deduction = in.Deduction
	r.ToPay = r.Net.Sub(in.Deduction)
	r.Cost = r.Income.Add(r.SocialEmployer).Add(r.HealthEmployer)

	if e.Contract == TPP && !e.Hourly && in.Base.IsZero() && e.Wage.Cmp(p.MinWage) < 0 && fullTime(e) {
		r.Notes = append(r.Notes, fmt.Sprintf("Mzda je nižšia ako minimálna mzda %s € (pri plnom úväzku).", p.MinWage.Display()))
	}
	if e.Hourly && e.Wage.Sign() > 0 && e.Wage.Cmp(p.MinHourly) < 0 {
		r.Notes = append(r.Notes, fmt.Sprintf("Hodinová sadzba je nižšia ako minimálna %s €.", p.MinHourly.String()))
	}
	return r, nil
}

func fullTime(e Employee) bool { return e.Hours.IsZero() || e.Hours.Cmp(m("40")) >= 0 }

func maxDec(a, b money.Dec) money.Dec {
	if a.Cmp(b) > 0 {
		return a
	}
	return b
}

func minDec(a, b money.Dec) money.Dec {
	if a.Cmp(b) < 0 {
		return a
	}
	return b
}

// Totals sú súčty mzdového obdobia na úhradu.
type Totals struct {
	Gross          money.Dec            `json:"gross"`
	Social         money.Dec            `json:"social"` // Sociálna poisťovňa spolu (zamestnanci + zamestnávateľ)
	SocialEmployee money.Dec            `json:"socialEmployee"`
	SocialEmployer money.Dec            `json:"socialEmployer"`
	Health         map[string]money.Dec `json:"health"` // zdravotná poisťovňa → spolu
	HealthTotal    money.Dec            `json:"healthTotal"`
	Tax            money.Dec            `json:"tax"` // preddavky po bonuse mínus vyplatené bonusy → daňový úrad
	Net            money.Dec            `json:"net"`
	ToPay          money.Dec            `json:"toPay"`
	Cost           money.Dec            `json:"cost"`
}

// Sum spočíta mzdy; insurers mapuje zamestnanca na zdravotnú poisťovňu.
func Sum(results []Result, insurers map[int64]string) Totals {
	t := Totals{Health: map[string]money.Dec{}}
	for _, r := range results {
		t.Gross = t.Gross.Add(r.Gross)
		t.SocialEmployee = t.SocialEmployee.Add(r.SocialEmployee)
		t.SocialEmployer = t.SocialEmployer.Add(r.SocialEmployer)
		h := r.HealthEmployee.Add(r.HealthEmployer)
		t.Health[insurers[r.EmployeeID]] = t.Health[insurers[r.EmployeeID]].Add(h)
		t.HealthTotal = t.HealthTotal.Add(h)
		t.Tax = t.Tax.Add(r.TaxPaid).Sub(r.BonusPaid)
		t.Net = t.Net.Add(r.Net)
		t.ToPay = t.ToPay.Add(r.ToPay)
		t.Cost = t.Cost.Add(r.Cost)
	}
	t.Social = t.SocialEmployee.Add(t.SocialEmployer)
	return t
}
