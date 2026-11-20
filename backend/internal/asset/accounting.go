package asset

import (
	"fmt"

	"github.com/fiskus-online/fiskus/backend/internal/money"
)

// Účtovné odpisy (§ 28 zákona o účtovníctve, podvojné účtovníctvo): podľa
// odpisového plánu účtovnej jednotky, nezávisle od daňových odpisov.
// Fiškus ich počíta rovnomerne mesačne z vstupnej ceny počas zadanej doby
// použiteľnosti — od mesiaca nasledujúceho po zaradení, naposledy za
// mesiac pred vyradením. Bez zadanej doby sú účtovné odpisy rovnaké ako
// daňové (bez krátenia podielom na podnikaní). Rozdiel medzi účtovnými
// a daňovými odpismi je pripočítateľná alebo odpočítateľná položka.

// Accounts sú syntetické účty dlhodobého majetku, ktoré sa odpisujú.
var Accounts = []string{"013", "014", "019", "021", "022", "023", "025", "026", "029"}

// DefaultAccount je účet majetku podľa druhu a odpisovej skupiny.
func (a *Asset) DefaultAccount() string {
	switch {
	case a.Kind == Intangible:
		return "013" // softvér
	case a.Group >= 5:
		return "021" // stavby
	default:
		return "022" // samostatné hnuteľné veci
	}
}

// AccountOrDefault je účet majetku (zadaný alebo predvolený).
func (a *Asset) AccountOrDefault() string {
	if a.Account != "" {
		return a.Account
	}
	return a.DefaultAccount()
}

// DepAccount je účet oprávok k účtu majetku (013 → 073, 022 → 082).
func DepAccount(account string) string {
	if len(account) != 3 || account[0] != '0' {
		return ""
	}
	switch account[1] {
	case '1':
		return "07" + account[2:]
	case '2':
		return "08" + account[2:]
	}
	return ""
}

func validAccount(acc string) bool {
	for _, a := range Accounts {
		if a == acc {
			return true
		}
	}
	return false
}

// validateAccounting skontroluje údaje účtovného odpisovania.
func (a *Asset) validateAccounting(add func(f, m string)) {
	if a.Account != "" && !validAccount(a.Account) {
		add("account", "Vyberte účet dlhodobého majetku (013 – 029)")
	}
	if a.AccMonths < 0 || a.AccMonths > 1200 {
		add("accMonths", "Doba účtovného odpisovania je 1 až 1 200 mesiacov")
	}
}

// AccYear je rok účtovného odpisového plánu.
type AccYear struct {
	Year        int       `json:"year"`
	Months      int       `json:"months"`      // počet odpisovaných mesiacov (0 pri odpisoch podľa dane)
	Amount      money.Dec `json:"amount"`      // účtovný odpis (551 / 08x)
	Accumulated money.Dec `json:"accumulated"` // oprávky na konci roka
	Residual    money.Dec `json:"residual"`    // účtovná zostatková cena na konci roka
	Disposal    money.Dec `json:"disposal"`    // zostatková cena pri vyradení
	Tax         money.Dec `json:"tax"`         // daňový odpis (pred krátením podielom)
	Diff        money.Dec `json:"diff"`        // účtovný − daňový (+ pripočítať, − odpočítať)
}

// AccSchedule zostaví účtovný odpisový plán.
func (a *Asset) AccSchedule() ([]AccYear, error) {
	tax := *a
	tax.Share = 100
	plan, err := tax.Schedule()
	if err != nil {
		return nil, err
	}
	taxBy := map[int]money.Dec{}
	for _, y := range plan {
		taxBy[y.Year] = y.Claimed
	}
	var out []AccYear
	acc := money.Zero
	add := func(y AccYear) {
		acc = acc.Add(y.Amount)
		y.Accumulated = acc
		cost := a.CostAt(y.Year)
		y.Residual = cost.Sub(acc)
		if !y.Disposal.IsZero() {
			y.Accumulated, y.Residual = cost, money.Zero
		}
		y.Tax = taxBy[y.Year]
		y.Diff = y.Amount.Sub(y.Tax)
		out = append(out, y)
	}

	if a.AccMonths == 0 {
		// účtovné odpisy = daňové (bez krátenia podielom)
		for _, y := range plan {
			ay := AccYear{Year: y.Year, Amount: y.Claimed}
			if !y.Disposal.IsZero() || (!a.Disposed.IsZero() && a.Disposed.Year() == y.Year) {
				ay.Disposal = a.CostAt(y.Year).Sub(acc)
			}
			add(ay)
		}
		return out, nil
	}

	// zložky: obstarávacia cena na AccMonths mesiacov od mesiaca po zaradení,
	// každé technické zhodnotenie na zvyšok doby od mesiaca po jeho dokončení
	// (najmenej jeden mesiac)
	type part struct {
		first, months, done int
		amount, monthly     money.Dec
	}
	first := a.Acquired.Year()*12 + a.Acquired.Month() // = (mesiac zaradenia + 1) − 1
	last := first + a.AccMonths - 1
	parts := []*part{{first: first, months: a.AccMonths, amount: a.Cost}}
	for _, t := range a.Improvements {
		f := t.Date.Year()*12 + t.Date.Month()
		m := last - f + 1
		if m < 1 {
			m = 1
		}
		parts = append(parts, &part{first: f, months: m, amount: t.Amount})
	}
	end := last
	for _, p := range parts {
		p.monthly = p.amount.MulRatio(money.FromInt(1), money.FromInt(int64(p.months))).RoundCents()
		if e := p.first + p.months - 1; e > end {
			end = e
		}
	}
	stop := -1 // posledný mesiac pred vyradením
	if !a.Disposed.IsZero() {
		stop = a.Disposed.Year()*12 + a.Disposed.Month() - 2
	}
	for year := a.Acquired.Year(); ; year++ {
		y := AccYear{Year: year}
		for mi := year * 12; mi < year*12+12; mi++ {
			if stop >= 0 && mi > stop {
				continue
			}
			counted := false
			for _, p := range parts {
				if mi < p.first || mi >= p.first+p.months {
					continue
				}
				p.done++
				v := p.monthly
				if p.done == p.months {
					v = p.amount.Sub(p.monthly.Mul(money.FromInt(int64(p.months - 1)))) // posledný mesiac vyrovná zaokrúhlenie
				}
				y.Amount = y.Amount.Add(v)
				counted = true
			}
			if counted {
				y.Months++
			}
		}
		disposedNow := !a.Disposed.IsZero() && a.Disposed.Year() == year
		if disposedNow {
			y.Disposal = a.CostAt(year).Sub(acc).Sub(y.Amount)
		}
		add(y)
		if disposedNow || (year*12+11 >= end) {
			break
		}
		if year > a.Acquired.Year()+110 {
			return nil, fmt.Errorf("odpisový plán: %w", ErrNoSchedule)
		}
	}
	return out, nil
}

// AccInYear vráti riadok účtovného plánu za rok (nulový mimo plánu).
func (a *Asset) AccInYear(year int) AccYear {
	plan, err := a.AccSchedule()
	if err == nil {
		for _, y := range plan {
			if y.Year == year {
				return y
			}
		}
	}
	return AccYear{Year: year}
}

// AccResidualAt je účtovná zostatková cena na konci roka.
func (a *Asset) AccResidualAt(year int) money.Dec {
	if a.Acquired.IsZero() || year < a.Acquired.Year() || (!a.Disposed.IsZero() && year >= a.Disposed.Year()) {
		return money.Zero
	}
	plan, err := a.AccSchedule()
	if err != nil {
		return money.Zero
	}
	res := a.CostAt(year)
	for _, y := range plan {
		if y.Year <= year {
			res = y.Residual
		}
	}
	return res
}
