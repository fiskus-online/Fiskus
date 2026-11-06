// Package dpfo počíta daňové priznanie k dani z príjmov fyzickej osoby typ B
// pre živnostníka v jednoduchom účtovníctve (alebo s paušálnymi výdavkami)
// a vytvorí XML, ktoré sa načíta do formulára DPFOB na portáli finančnej
// správy. Postup riadkov je podľa pokynov k formuláru (kontroly polí
// formulára DPFOBv25); sumy sa zaokrúhľujú matematicky na centy postupne.
//
// Fiškus vyplní to, čo vie z účtovníctva (VI. oddiel, tabuľky 1 a 1a,
// základ dane, nezdaniteľné časti, daň, preddavky), a z údajov daňovníka
// deti a daňový bonus, manžela/ku, straty z predchádzajúcich rokov a počet
// príloh. Zahraničné príjmy a ďalšie prílohy doplní daňovník vo formulári.
package dpfo

import (
	"errors"
	"fmt"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Bracket je pásmo sadzby: do sumy Upto (vrátane) sadzba Rate %.
// Posledné pásmo má Upto nulové (bez hornej hranice).
type Bracket struct {
	Upto money.Dec
	Rate int64
}

// Params sú zákonné hodnoty za zdaňovacie obdobie.
type Params struct {
	Year int
	// Nezdaniteľná časť na daňovníka: plná suma do základu NCZDLimit, nad ňou
	// NCZDConst − základ / NCZDDiv (najmenej nula).
	NCZD, NCZDLimit, NCZDConst money.Dec
	NCZDDiv                    int64
	DDSMax                     money.Dec // príspevok na doplnkové dôchodkové sporenie
	Brackets                   []Bracket // § 15 písm. a) — zamestnanie, podnikanie nad hranicou príjmov
	FlatRate                   int64     // § 15 písm. a) bod 1 — podnikanie s príjmami do FlatLimit
	FlatLimit                  money.Dec
	ExpensePct                 int64     // paušálne výdavky § 6 ods. 10 (percento príjmov)
	ExpenseMax                 money.Dec // paušálne výdavky najviac (ročne)
	MinTax                     money.Dec // daň do tejto sumy sa nevyrubí
	NoTaxIncome                money.Dec // pri príjmoch do tejto sumy sa daň nevyrubí (50 % NČZD)
	Form                       string    // tlačivo, ktorého štruktúru XML používame
	// manžel/ka (§ 11 ods. 3): plná suma do základu SpouseLimit, nad ním
	// SpouseConst − základ / SpouseDiv; obe mínus vlastný príjem
	SpouseFull, SpouseLimit, SpouseConst money.Dec
	SpouseDiv                            int64
	// daňový bonus (§ 33): mesačne do 15 rokov a od 15 do 18 rokov;
	// BonusLimit je ročná hranica základu, nad ktorou sa bonus znižuje
	BonusYoung, BonusOld, BonusLimit money.Dec
}

var m = money.MustParse

// years sú overené hodnoty. Zdroje: kontroly polí formulára DPFOBv25
// (za 2025) a podpora.financnasprava.sk 214187, 081400 (za 2026); manžel/ka
// 426769, 614044; daňový bonus 790897, 215354.
var years = map[int]Params{
	2025: {
		Year: 2025, NCZD: m("5753.79"), NCZDLimit: m("25426.27"), NCZDConst: m("12110.36"), NCZDDiv: 4, DDSMax: m("180"),
		Brackets: []Bracket{{m("48441.43"), 19}, {money.Zero, 25}},
		FlatRate: 15, FlatLimit: m("100000"), ExpensePct: 60, ExpenseMax: m("20000"),
		MinTax: m("17"), NoTaxIncome: m("2876.90"), Form: "DPFOBv25",
		SpouseFull: m("5260.61"), SpouseLimit: m("48441.43"), SpouseConst: m("17370.97"), SpouseDiv: 4,
		BonusYoung: m("100"), BonusOld: m("50"), BonusLimit: m("25740"), // 1,5 × 12 × 1 430 € (priemerná mzda 2023)
	},
	2026: {
		Year: 2026, NCZD: m("5966.73"), NCZDLimit: m("26083.13"), NCZDConst: m("14661.11"), NCZDDiv: 3, DDSMax: m("180"),
		Brackets: []Bracket{{m("43983.32"), 19}, {m("60349.21"), 25}, {m("75010.32"), 30}, {money.Zero, 35}},
		FlatRate: 15, FlatLimit: m("100000"), ExpensePct: 60, ExpenseMax: m("20000"),
		MinTax: m("17"), NoTaxIncome: m("2983.37"), Form: "DPFOBv25",
		SpouseFull: m("5455.30"), SpouseLimit: m("43983.32"), SpouseConst: m("20116.40"), SpouseDiv: 3,
		BonusYoung: m("100"), BonusOld: m("50"), BonusLimit: m("27432"), // 1,5 × 12 × 1 524 € (priemerná mzda 2024)
	},
}

// ErrYear — pre rok nie sú v programe zákonné hodnoty.
var ErrYear = errors.New("pre tento rok Fiškus ešte nemá zákonné hodnoty daňového priznania")

// ParamsFor vráti zákonné hodnoty za rok.
func ParamsFor(year int) (Params, error) {
	p, ok := years[year]
	if !ok {
		return Params{}, fmt.Errorf("%w (%d)", ErrYear, year)
	}
	return p, nil
}

// Years vráti roky, za ktoré vie Fiškus priznanie zostaviť.
func Years() []int {
	out := make([]int, 0, len(years))
	for y := range years {
		out = append(out, y)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] > out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Taxpayer sú údaje o daňovníkovi (I. oddiel).
type Taxpayer struct {
	DIC       string `json:"dic"`
	BirthDate string `json:"birthDate"` // DD.MM.RRRR (iba bez DIČ)
	Surname   string `json:"surname"`
	Name      string `json:"name"`
	Title     string `json:"title"`
	TitleAft  string `json:"titleAfter"`
	Street    string `json:"street"`
	Number    string `json:"number"`
	Zip       string `json:"zip"`
	City      string `json:"city"`
	Country   string `json:"country"`
	NACE      string `json:"nace"`     // 5 číslic, napr. 43210
	Activity  string `json:"activity"` // činnosť s najvyšším príjmom
}

// Input sú údaje na výpočet. Sumy z účtovníctva dopĺňa program, ostatné
// zadá daňovník.
type Input struct {
	Year     int      `json:"year"`
	Taxpayer Taxpayer `json:"taxpayer"`
	// príjmy a výdavky zo živnosti (tabuľka č. 1, r. 2) z peňažného denníka
	Income    money.Dec `json:"income"`
	Expenses  money.Dec `json:"expenses"`  // preukázateľné výdavky vrátane poistného
	Insurance money.Dec `json:"insurance"` // zaplatené poistné (pri paušálnych výdavkoch sa pripočíta)
	// FlatExpenses: výdavky percentom z príjmov (§ 6 ods. 10) namiesto skutočných.
	FlatExpenses bool `json:"flatExpenses"`
	// tabuľka č. 1a — začiatok a koniec obdobia (6 riadkov)
	Table1a [6][2]money.Dec `json:"table1a"`
	// zamestnanie (r. 36, 37) a preddavky zrazené zamestnávateľom (r. 131)
	EmpIncome    money.Dec `json:"empIncome"`
	EmpInsurance money.Dec `json:"empInsurance"`
	EmpPrepaid   money.Dec `json:"empPrepaid"`
	DDS          money.Dec `json:"dds"`     // príspevky na DDS (r. 75)
	Prepaid      money.Dec `json:"prepaid"` // preddavky na daň z podnikania (r. 133)
	// podiel dane (§ 50): IČO a názov prijímateľa
	NGOICO    string `json:"ngoIco"`
	NGOName   string `json:"ngoName"`
	Volunteer bool   `json:"volunteer"` // dobrovoľník — 3 %
	// rodina a straty
	Children    []Child   `json:"children"`    // vyživované deti (r. 33, najviac 4 vo formulári)
	Spouse      *Spouse   `json:"spouse"`      // manžel/ka (r. 31, 32); nil = neuplatňuje
	Losses      []Loss    `json:"losses"`      // straty z predchádzajúcich 5 rokov
	EmpBonus    money.Dec `json:"empBonus"`    // bonus vyplatený zamestnávateľom (r. 119)
	Attachments int       `json:"attachments"` // počet príloh (r. 154)
	// vrátenie preplatku (a vyplatenie bonusu) na účet
	RefundIBAN string `json:"refundIban"`
	Date       string `json:"date"` // dátum vyhlásenia DD.MM.RRRR
}

// Result sú vypočítané riadky priznania (kód riadku → suma).
type Result struct {
	Params Params               `json:"-"`
	Rows   map[string]money.Dec `json:"rows"`
	// T1 je tabuľka č. 1, r. 2 a r. 10 (príjmy, výdavky).
	T1Income, T1Expenses money.Dec
	ToPay, Refund        money.Dec // r. 135 daň na úhradu, r. 136 daňový preplatok
	BonusPaid            money.Dec // r. 121 daňový bonus, ktorý správca dane vyplatí
	Notes                []string  `json:"notes"` // čo treba doplniť alebo skontrolovať
}

func pct(d money.Dec, p int64) money.Dec {
	return d.MulRatio(money.FromInt(p), money.FromInt(100)).RoundCents()
}

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

// progressive vypočíta daň podľa pásiem (postupné zaokrúhľovanie).
func progressive(base money.Dec, bs []Bracket) money.Dec {
	if base.Sign() <= 0 {
		return money.Zero
	}
	tax, prev := money.Zero, money.Zero
	for _, b := range bs {
		top := base
		if !b.Upto.IsZero() && b.Upto.Cmp(base) < 0 {
			top = b.Upto
		}
		if top.Cmp(prev) > 0 {
			tax = tax.Add(pct(top.Sub(prev), b.Rate))
		}
		if b.Upto.IsZero() || b.Upto.Cmp(base) >= 0 {
			break
		}
		prev = b.Upto
	}
	return tax.RoundCents()
}

// nczd je nezdaniteľná časť na daňovníka zo základu dane (r. 72).
func nczd(base money.Dec, p Params) money.Dec {
	if base.Sign() <= 0 {
		return money.Zero
	}
	if base.Cmp(p.NCZDLimit) <= 0 {
		return p.NCZD
	}
	part := base.MulRatio(money.FromInt(1), money.FromInt(p.NCZDDiv)).RoundCents()
	return maxDec(money.Zero, p.NCZDConst.Sub(part))
}

// Compute vypočíta priznanie.
func Compute(in Input) (Result, error) {
	p, err := ParamsFor(in.Year)
	if err != nil {
		return Result{}, err
	}
	r := map[string]money.Dec{}
	res := Result{Params: p, Rows: r}

	// tabuľka č. 1, r. 2 (živnosť) = r. 10
	inc := in.Income.RoundCents()
	exp := in.Expenses.RoundCents()
	if in.FlatExpenses {
		exp = minDec(pct(inc, p.ExpensePct), p.ExpenseMax).Add(in.Insurance.RoundCents())
	}
	res.T1Income, res.T1Expenses = inc, exp
	r["39"], r["40"] = inc, exp
	diff := inc.Sub(exp)
	if diff.Sign() >= 0 {
		r["41"] = diff
	} else {
		r["42"] = diff // záporné číslo
	}
	r["45"] = maxDec(money.Zero, diff)
	if diff.Sign() < 0 {
		r["46"] = diff
	}
	// straty z predchádzajúcich rokov (r. 47 – 54a); mikrodaňovník do 100 000 € príjmov
	lost := deductLosses(r, in.Losses, in.Year, inc.Cmp(p.FlatLimit) <= 0)
	r["55"] = r["45"].Sub(lost)
	r["57"] = r["55"]

	// zamestnanie
	r["36"], r["37"] = in.EmpIncome.RoundCents(), in.EmpInsurance.RoundCents()
	r["38"] = maxDec(money.Zero, r["36"].Sub(r["37"]))

	// nezdaniteľné časti (IV. oddiel)
	r["72"] = maxDec(money.Zero, r["38"].Add(r["57"]))
	r["73"] = nczd(r["72"], p)
	r["74"] = spouseNCZD(in.Spouse, r["72"], p)
	r["75"] = minDec(in.DDS.RoundCents(), p.DDSMax)
	if r["72"].IsZero() {
		r["75"] = money.Zero
	}
	r["77"] = minDec(r["72"], r["73"].Add(r["74"]).Add(r["75"]))
	r["78"] = maxDec(money.Zero, r["38"].Sub(r["77"]))
	r["80"] = r["78"]
	r["81"] = progressive(r["80"], p.Brackets)
	r["90"] = r["81"]
	if r["78"].IsZero() {
		r["91"] = maxDec(money.Zero, r["77"].Sub(r["38"]))
	}
	r["92"] = maxDec(money.Zero, r["57"].Sub(r["91"]))
	r["94"] = r["92"]
	r["95"] = inc
	if r["95"].Cmp(p.FlatLimit) <= 0 {
		r["96"] = pct(r["94"], p.FlatRate)
	} else {
		r["96"] = progressive(r["94"], p.Brackets)
	}
	r["105"] = r["96"]

	// daň celkom (r. 116) — do 17 € alebo pri príjmoch do 50 % NČZD sa nevyrubí
	tax := r["90"].Add(r["105"])
	if tax.Cmp(p.MinTax) <= 0 || r["36"].Add(r["95"]).Cmp(p.NoTaxIncome) <= 0 {
		tax = money.Zero
	}
	r["116"] = tax

	// daňový bonus na deti (r. 116a – 122)
	r["116a"] = r["38"].Add(r["45"])
	bonus, err := Bonus(in.Children, r["116a"], p)
	if err != nil {
		res.Notes = append(res.Notes, "Daňový bonus sa nedá vypočítať: "+err.Error())
		bonus = money.Zero
	}
	if len(in.Children) == 0 {
		r["116a"] = money.Zero
	}
	r["117"] = bonus
	r["118"] = tax.Sub(bonus)
	r["119"] = in.EmpBonus.RoundCents()
	r["120"] = maxDec(money.Zero, r["117"].Sub(r["119"]))
	r["121"] = maxDec(money.Zero, r["120"].Sub(r["116"]))
	r["122"] = maxDec(money.Zero, r["119"].Sub(r["117"]))
	r["124"] = r["118"]
	r["131"] = in.EmpPrepaid.RoundCents()
	r["133"] = in.Prepaid.RoundCents()
	// r. 135/136 = r. 116 − r. 117 + r. 119 + r. 121 − r. 131 − r. 133
	due := tax.Sub(r["117"]).Add(r["119"]).Add(r["121"]).Sub(r["131"]).Sub(r["133"])
	r["135"] = maxDec(money.Zero, due)
	r["136"] = maxDec(money.Zero, due.Neg())
	res.ToPay, res.Refund, res.BonusPaid = r["135"], r["136"], r["121"]

	// podiel dane pre prijímateľa (§ 50) — najmenej 3 €, iba z kladnej dane
	if in.NGOICO != "" && r["124"].Sign() > 0 {
		share := pct(r["124"], 2)
		if in.Volunteer {
			share = pct(r["124"], 3)
		}
		if share.Cmp(money.FromInt(3)) >= 0 {
			r["152"] = share
		} else {
			res.Notes = append(res.Notes, "Podiel dane je nižší ako 3 € — prijímateľovi sa neposiela")
		}
	}

	if in.FlatExpenses && in.Insurance.IsZero() {
		res.Notes = append(res.Notes, "Pri paušálnych výdavkoch sa pripočítava zaplatené poistné — skontrolujte, či je zadané")
	}
	if len(in.Children) > 4 {
		res.Notes = append(res.Notes, "Formulár má miesto na 4 deti — ďalšie uveďte v osobitných záznamoch (bonus je započítaný za všetky)")
	}
	if len(in.Children) > 0 && r["116a"].IsZero() {
		res.Notes = append(res.Notes, "Daňový bonus sa neuplatní — nemáte základ dane zo zamestnania ani podnikania")
	}
	if in.Spouse != nil && in.Spouse.Months > 0 && r["74"].IsZero() {
		res.Notes = append(res.Notes, "Nezdaniteľná časť na manžela/ku vychádza nulová — vlastný príjem je vyšší ako suma, alebo je základ dane vysoký")
	}
	if !r["121"].IsZero() && in.RefundIBAN == "" {
		res.Notes = append(res.Notes, "Daňový bonus vyšší ako daň vám finančná správa vyplatí — zadajte IBAN, inak príde poštovou poukážkou")
	}
	if in.Taxpayer.DIC == "" && in.Taxpayer.BirthDate == "" {
		res.Notes = append(res.Notes, "Chýba DIČ alebo dátum narodenia")
	}
	return res, nil
}
