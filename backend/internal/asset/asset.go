// Package asset je evidencia dlhodobého majetku a daňové odpisy podľa
// zákona č. 595/2003 Z. z. o dani z príjmov (§ 22 – § 29):
//
//   - hmotný majetok so vstupnou cenou nad 1 700 € a nehmotný nad 2 400 €;
//   - odpisové skupiny 0 – 6 s dobou odpisovania 2, 4, 6, 8, 12, 20, 40 rokov;
//   - rovnomerné odpisovanie (§ 27): ročný odpis = vstupná cena / doba;
//   - zrýchlené odpisovanie (§ 28) iba v skupinách 2 a 3: prvý rok
//     VC / k1, ďalšie roky 2 × ZC / (k − n), koeficienty 6/7 a 8/9;
//   - v prvom roku sa uplatní pomerná časť ročného odpisu podľa mesiacov
//     od mesiaca zaradenia do užívania; neuplatnená časť sa doodpíše v roku
//     nasledujúcom po uplynutí doby odpisovania;
//   - v roku vyradenia (predaj, likvidácia) sa odpis neuplatní, zostatková
//     cena je výdavkom (§ 22 ods. 11, § 24 ods. 2);
//   - pri čiastočnom používaní na podnikanie sa uplatní pomerná časť;
//   - technické zhodnotenie (§ 29) zvyšuje vstupnú cenu: v prvom roku
//     odpisovania od začiatku, neskôr pri rovnomernom odpisovaní ročný odpis
//     = zvýšená VC / doba odpisovania (doba sa predĺži, kým sa neodpíše),
//     pri zrýchlenom zvyšuje zostatkovú cenu spolu s neuplatnenou časťou
//     prvého roka a ďalej sa počíta koeficientom pre zvýšenú ZC (§ 28 ods. 3);
//   - prerušenie odpisovania hmotného majetku (§ 22 ods. 9) na celé roky:
//     v odpisovaní sa potom pokračuje, akoby prerušené nebolo.
package asset

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
)

// Kind je druh majetku.
type Kind string

const (
	Tangible   Kind = "tangible"   // hmotný
	Intangible Kind = "intangible" // nehmotný
)

// Method je spôsob odpisovania.
type Method string

const (
	Straight    Method = "straight"    // rovnomerné (§ 27)
	Accelerated Method = "accelerated" // zrýchlené (§ 28)
)

// Disposal je dôvod vyradenia.
type Disposal string

const (
	DisposalSale        Disposal = "sale"        // predaj
	DisposalLiquidation Disposal = "liquidation" // likvidácia, škoda
	DisposalOther       Disposal = "other"       // vklad, dar, iné
)

// Hranice vstupnej ceny (§ 22 ods. 1 a 2).
var (
	TangibleLimit   = money.FromInt(1700)
	IntangibleLimit = money.FromInt(2400)
)

// GroupYears je doba odpisovania podľa odpisovej skupiny (§ 26 ods. 1).
var GroupYears = []int{2, 4, 6, 8, 12, 20, 40}

// accel sú koeficienty zrýchleného odpisovania (§ 28 ods. 1):
// [prvý rok, ďalšie roky, zvýšená zostatková cena].
var accel = map[int][3]int64{2: {6, 7, 6}, 3: {8, 9, 8}}

// Improvement je technické zhodnotenie (§ 29): dokončené a zaradené do
// užívania v deň Date.
type Improvement struct {
	Date       invoice.Date `json:"date"`
	Amount     money.Dec    `json:"amount"`
	Note       string       `json:"note,omitempty"`
	PurchaseID int64        `json:"purchaseId,omitempty"` // prijatá faktúra
	PurchaseNo string       `json:"purchaseNumber,omitempty"`
}

// Asset je položka dlhodobého majetku.
type Asset struct {
	ID        int64        `json:"id"`
	CompanyID int64        `json:"companyId"`
	Number    string       `json:"number"` // inventárne číslo
	Name      string       `json:"name"`
	Kind      Kind         `json:"kind"`
	Group     int          `json:"group"` // odpisová skupina 0–6 (hmotný)
	Years     int          `json:"years"` // doba odpisovania nehmotného majetku
	Method    Method       `json:"method"`
	Acquired  invoice.Date `json:"acquired"` // zaradenie do užívania
	Cost      money.Dec    `json:"cost"`     // vstupná cena
	// Share je percento používania na podnikanie (1–100).
	Share        int          `json:"share"`
	PurchaseID   int64        `json:"purchaseId,omitempty"` // prijatá faktúra
	PurchaseNo   string       `json:"purchaseNumber,omitempty"`
	Disposed     invoice.Date `json:"disposed"` // vyradenie (prázdne = v užívaní)
	DisposalKind Disposal     `json:"disposal,omitempty"`
	Note         string       `json:"note"`
	// Podvojné účtovníctvo: účet majetku (prázdny = podľa druhu a skupiny)
	// a doba účtovného odpisovania v mesiacoch (0 = ako daňové odpisy).
	Account   string `json:"account,omitempty"`
	AccMonths int    `json:"accMonths,omitempty"`
	// Technické zhodnotenia (zoradené podľa dátumu) a roky, v ktorých je
	// daňové odpisovanie prerušené (§ 22 ods. 9, iba hmotný majetok).
	Improvements []Improvement `json:"improvements,omitempty"`
	Paused       []int         `json:"paused,omitempty"`
}

// CostAt vráti vstupnú cenu zvýšenú o technické zhodnotenia do konca roka.
func (a *Asset) CostAt(year int) money.Dec {
	c := a.Cost
	for _, t := range a.Improvements {
		if t.Date.Year() <= year {
			c = c.Add(t.Amount)
		}
	}
	return c
}

// TotalCost je vstupná cena so všetkými technickými zhodnoteniami.
func (a *Asset) TotalCost() money.Dec { return a.CostAt(9999) }

func (a *Asset) paused(year int) bool {
	for _, y := range a.Paused {
		if y == year {
			return true
		}
	}
	return false
}

// Period vráti dobu odpisovania v rokoch.
func (a *Asset) Period() int {
	if a.Kind == Intangible {
		return a.Years
	}
	if a.Group < 0 || a.Group >= len(GroupYears) {
		return 0
	}
	return GroupYears[a.Group]
}

// Normalize doplní predvolené hodnoty.
func (a *Asset) Normalize() {
	a.Name, a.Number, a.Note = strings.TrimSpace(a.Name), strings.TrimSpace(a.Number), strings.TrimSpace(a.Note)
	if a.Kind == "" {
		a.Kind = Tangible
	}
	if a.Method == "" {
		a.Method = Straight
	}
	if a.Share == 0 {
		a.Share = 100
	}
	if a.Kind == Intangible {
		a.Group, a.Method = 0, Straight
	} else {
		a.Years = 0
	}
	if a.Disposed.IsZero() {
		a.DisposalKind = ""
	}
	for i := range a.Improvements {
		a.Improvements[i].Note = strings.TrimSpace(a.Improvements[i].Note)
	}
	sort.SliceStable(a.Improvements, func(i, j int) bool { return a.Improvements[i].Date.Before(a.Improvements[j].Date) })
	sort.Ints(a.Paused)
	if a.Kind == Intangible {
		a.Paused = nil
	}
}

// Validate skontroluje položku majetku.
func (a *Asset) Validate() error {
	var fe []invoice.FieldError
	add := func(f, m string) { fe = append(fe, invoice.FieldError{Field: f, Message: m}) }
	if a.Name == "" {
		add("name", "Zadajte názov majetku")
	}
	costSet := a.Cost.Sign() > 0
	if !costSet {
		add("cost", "Zadajte vstupnú cenu")
	}
	switch a.Kind {
	case Tangible:
		if a.Group < 0 || a.Group >= len(GroupYears) {
			add("group", "Vyberte odpisovú skupinu 0 až 6")
		}
		if costSet && a.Cost.Cmp(TangibleLimit) <= 0 {
			add("cost", "Hmotný majetok sa odpisuje od vstupnej ceny nad 1\u00a0700\u00a0€ — lacnejší je výdavok naraz")
		}
	case Intangible:
		if a.Years < 1 || a.Years > 40 {
			add("years", "Zadajte dobu odpisovania nehmotného majetku (podľa odpisového plánu)")
		}
		if costSet && a.Cost.Cmp(IntangibleLimit) <= 0 {
			add("cost", "Nehmotný majetok sa odpisuje od vstupnej ceny nad 2\u00a0400\u00a0€ — lacnejší je výdavok naraz")
		}
	default:
		add("kind", "Vyberte hmotný alebo nehmotný majetok")
	}
	switch a.Method {
	case Straight:
	case Accelerated:
		if _, ok := accel[a.Group]; !ok || a.Kind != Tangible {
			add("method", "Zrýchlene sa odpisuje iba hmotný majetok v skupine 2 a 3")
		}
	default:
		add("method", "Neznámy spôsob odpisovania")
	}
	if a.Acquired.IsZero() {
		add("acquired", "Zadajte dátum zaradenia do užívania")
	}
	if !a.Cost.Equal(a.Cost.RoundCents()) {
		add("cost", "Vstupná cena môže mať najviac dve desatinné miesta")
	}
	if a.Share < 1 || a.Share > 100 {
		add("share", "Podiel na podnikaní musí byť 1 až 100 %")
	}
	if !a.Disposed.IsZero() {
		switch {
		case a.Disposed.Before(a.Acquired):
			add("disposed", "Vyradenie nemôže byť pred zaradením")
		case a.DisposalKind != DisposalSale && a.DisposalKind != DisposalLiquidation && a.DisposalKind != DisposalOther:
			add("disposal", "Vyberte dôvod vyradenia")
		}
	}
	for i, t := range a.Improvements {
		f := fmt.Sprintf("improvements.%d", i)
		switch {
		case t.Amount.Sign() <= 0 || !t.Amount.Equal(t.Amount.RoundCents()):
			add(f, "Zadajte sumu technického zhodnotenia (najviac dve desatinné miesta)")
		case t.Date.IsZero() || (!a.Acquired.IsZero() && t.Date.Before(a.Acquired)):
			add(f, "Technické zhodnotenie musí byť po zaradení majetku do užívania")
		case !a.Disposed.IsZero() && t.Date.After(a.Disposed):
			add(f, "Technické zhodnotenie nemôže byť po vyradení majetku")
		}
	}
	for i, y := range a.Paused {
		f := fmt.Sprintf("paused.%d", i)
		switch {
		case !a.Acquired.IsZero() && y < a.Acquired.Year():
			add(f, "Prerušiť sa dá iba rok od zaradenia do užívania")
		case !a.Disposed.IsZero() && y >= a.Disposed.Year():
			add(f, "V roku vyradenia a neskôr sa odpisovanie neprerušuje")
		case i > 0 && a.Paused[i-1] == y:
			add(f, "Rok je prerušený dvakrát")
		}
	}
	a.validateAccounting(add)
	if len(fe) > 0 {
		return &invoice.ValidationError{Fields: fe}
	}
	return nil
}

// Year je rok v odpisovom pláne.
type Year struct {
	Year     int       `json:"year"`
	Annual   money.Dec `json:"annual"`   // ročný odpis podľa zákona (pred krátením podielom)
	Claimed  money.Dec `json:"claimed"`  // uplatnený odpis (× podiel na podnikaní)
	Residual money.Dec `json:"residual"` // daňová zostatková cena na konci roka
	// Disposal je zostatková cena, ktorá je výdavkom pri vyradení (× podiel).
	Disposal money.Dec `json:"disposal"`
	Note     string    `json:"note,omitempty"`
}

// ErrNoSchedule — majetok nemá platné údaje na odpisový plán.
var ErrNoSchedule = errors.New("odpisový plán sa nedá zostaviť")

// Schedule zostaví daňový odpisový plán od roku zaradenia po posledný odpis
// alebo vyradenie.
func (a *Asset) Schedule() ([]Year, error) {
	n := a.Period()
	if n <= 0 || a.Acquired.IsZero() || a.Cost.Sign() <= 0 {
		return nil, ErrNoSchedule
	}
	twelve := money.FromInt(12)
	share := func(d money.Dec) money.Dec {
		if a.Share >= 100 {
			return d
		}
		return d.MulRatio(money.FromInt(int64(a.Share)), money.FromInt(100)).RoundCents()
	}
	start := a.Acquired.Year()
	lastTZ := start
	for _, t := range a.Improvements {
		if t.Date.Year() > lastTZ {
			lastTZ = t.Date.Year()
		}
	}
	improved := func(year int) money.Dec {
		sum := money.Zero
		for _, t := range a.Improvements {
			if t.Date.Year() == year {
				sum = sum.Add(t.Amount)
			}
		}
		return sum
	}
	accelerated := a.Method == Accelerated
	k := accel[a.Group]

	var out []Year
	base := a.Cost.Add(improved(start)) // TZ v prvom roku zvyšuje VC od začiatku
	// rest je ešte neodpísaná časť (pri zrýchlenom ZC na účely výpočtu
	// odpisov), unclaimed neuplatnená pomerná časť prvého roka
	rest, unclaimed := base, money.Zero
	annual := money.Zero
	i := 0                 // poradie odpisovaného roka (bez prerušených)
	coef, since := k[1], 0 // zrýchlené: koeficient a roky od posledného zvýšenia ZC
	started := false

	for year := start; ; year++ {
		if year > start+200 {
			return nil, ErrNoSchedule
		}
		y := Year{Year: year}
		if tz := improved(year); year > start && tz.Sign() > 0 {
			if accelerated && started {
				// § 28 ods. 3: ZC sa zvýši o TZ a neuplatnenú časť prvého roka
				rest = rest.Add(tz).Add(unclaimed)
				unclaimed = money.Zero
				coef, since = k[2], 0
			} else {
				rest = rest.Add(tz)
			}
			base = base.Add(tz)
			if !accelerated && started {
				annual = base.MulRatio(money.FromInt(1), money.FromInt(int64(n))).RoundCents()
			}
			y.Note = "technické zhodnotenie " + tz.Display() + " €"
		}
		if !a.Disposed.IsZero() && a.Disposed.Year() == year {
			// rok vyradenia: odpis sa neuplatní, ZC (vrátane neuplatnenej
			// časti prvého roka) je výdavkom
			y.Disposal = share(rest.Add(unclaimed))
			y.Note = map[Disposal]string{DisposalSale: "predaj", DisposalLiquidation: "likvidácia", DisposalOther: "vyradenie"}[a.DisposalKind]
			out = append(out, y)
			return out, nil
		}
		if rest.IsZero() && unclaimed.IsZero() {
			if year > lastTZ {
				return out, nil
			}
			continue // odpísané; možno ešte príde technické zhodnotenie
		}
		if a.paused(year) && a.Kind == Tangible {
			y.Note = joinNote(y.Note, "odpisovanie prerušené")
			y.Residual = rest.Add(unclaimed)
			out = append(out, y)
			continue
		}
		var claimed money.Dec
		switch {
		case !started:
			// prvý odpisovaný rok: pomerná časť podľa mesiacov od zaradenia;
			// ak je odpisovanie prerušené už v roku zaradenia, prvým rokom
			// odpisovania je až ďalší rok a uplatní sa celý ročný odpis
			started = true
			if accelerated {
				annual = base.MulRatio(money.FromInt(1), money.FromInt(k[0])).RoundCents()
			} else {
				annual = base.MulRatio(money.FromInt(1), money.FromInt(int64(n))).RoundCents()
			}
			months := money.FromInt(int64(13 - a.Acquired.Month()))
			if year != start {
				months = twelve
			}
			claimed = annual
			if !months.Equal(twelve) {
				claimed = annual.Mul(months).MulRatio(money.FromInt(1), twelve).RoundCents()
				y.Note = joinNote(y.Note, fmt.Sprintf("pomerná časť %d/12", 13-a.Acquired.Month()))
			}
			unclaimed = annual.Sub(claimed)
			rest = rest.Sub(annual)
			y.Annual = annual
		case accelerated && rest.Sign() > 0:
			since++
			den := coef - int64(since)
			if coef == k[2] {
				den = coef - int64(since-1) // v roku zvýšenia ZC sa delí koeficientom pre zvýšenú ZC
			}
			v := rest
			if den > 2 {
				v = rest.MulRatio(money.FromInt(2), money.FromInt(den)).RoundCents()
			}
			if v.Cmp(rest) > 0 {
				v = rest
			}
			rest = rest.Sub(v)
			claimed, y.Annual = v, v
		case !accelerated && rest.Sign() > 0:
			v := annual
			// posledný riadny rok vyrovná zaokrúhlenie
			if rest.Cmp(v) < 0 || (i == n-1 && rest.Sub(v).Cmp(money.FromCents(int64(n))) <= 0) {
				v = rest
			}
			rest = rest.Sub(v)
			claimed, y.Annual = v, annual
			// po uplynutí doby sa v rámci ročného odpisu doodpíše aj
			// neuplatnená časť prvého roka
			if i >= n && unclaimed.Sign() > 0 {
				room := annual.Sub(v)
				if room.Cmp(unclaimed) > 0 {
					room = unclaimed
				}
				claimed, unclaimed = claimed.Add(room), unclaimed.Sub(room)
				if room.Sign() > 0 {
					y.Note = joinNote(y.Note, "aj neuplatnená časť prvého roka")
				}
			}
		default:
			// odpísané: v nasledujúcom roku neuplatnená časť prvého roka
			claimed, unclaimed = unclaimed, money.Zero
			y.Annual = claimed
			y.Note = joinNote(y.Note, "neuplatnená časť prvého roka")
		}
		i++
		y.Claimed = share(claimed)
		y.Residual = rest.Add(unclaimed)
		out = append(out, y)
	}
}

func joinNote(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

// InYear vráti riadok plánu za rok (nulový, ak v tom roku nič nie je).
func (a *Asset) InYear(year int) Year {
	plan, err := a.Schedule()
	if err != nil {
		return Year{Year: year}
	}
	for _, y := range plan {
		if y.Year == year {
			return y
		}
	}
	// pred zaradením: nič; po poslednom odpise: ZC 0
	return Year{Year: year}
}

// ResidualAt vráti daňovú zostatkovú cenu na konci roka (0 po vyradení
// alebo doodpisovaní, VC pred zaradením sa neráta — majetok ešte nie je).
func (a *Asset) ResidualAt(year int) money.Dec {
	if a.Acquired.IsZero() || year < a.Acquired.Year() {
		return money.Zero
	}
	if !a.Disposed.IsZero() && year >= a.Disposed.Year() {
		return money.Zero
	}
	plan, err := a.Schedule()
	if err != nil {
		return money.Zero
	}
	for _, y := range plan {
		if y.Year == year {
			return y.Residual
		}
	}
	return money.Zero
}
