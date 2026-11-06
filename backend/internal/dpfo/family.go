package dpfo

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Deti (daňový bonus § 33), manžel/ka (NČZD § 11 ods. 3) a daňové straty
// z predchádzajúcich rokov (§ 30). Postup podľa pokynov k DPFOBv25
// a podpora.financnasprava.sk 790897 (bonus 2025), 215354 (bonus 2026),
// 426769 (manželka 2025), 614044 (manželka 2026).

// Child je vyživované dieťa (r. 33). Bez vybraných mesiacov = celý rok.
type Child struct {
	Name   string   `json:"name"`
	RC     string   `json:"rc"`     // rodné číslo — z neho sa určí vek
	Months [12]bool `json:"months"` // mesiace, v ktorých dieťa žilo v domácnosti
}

// Spouse je manžel/ka (r. 31, 32) na nezdaniteľnú časť.
type Spouse struct {
	Name      string    `json:"name"`
	RC        string    `json:"rc"`
	OwnIncome money.Dec `json:"ownIncome"` // vlastný príjem znížený o poistné
	Months    int       `json:"months"`    // počet mesiacov, keď boli splnené podmienky
}

// Loss je daňová strata vykázaná v predchádzajúcom roku (r. 47 – 50a).
type Loss struct {
	Year   int       `json:"year"`
	Amount money.Dec `json:"amount"` // vykázaná strata
	Used   money.Dec `json:"used"`   // odpočítaná v predchádzajúcich rokoch
}

// allYear: žiadny mesiac nevybraný = celý rok.
func (c Child) allYear() bool {
	for _, m := range c.Months {
		if m {
			return false
		}
	}
	return true
}

// BirthDate určí dátum narodenia z rodného čísla (aj s lomkou). Kontroluje
// formát a pri 10-miestnom čísle deliteľnosť 11.
func BirthDate(rc string) (time.Time, error) {
	d := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(rc), "/", ""), " ", "")
	if len(d) != 9 && len(d) != 10 {
		return time.Time{}, fmt.Errorf("rodné číslo má 9 alebo 10 číslic")
	}
	if _, err := strconv.ParseUint(d, 10, 64); err != nil {
		return time.Time{}, fmt.Errorf("rodné číslo má iba číslice")
	}
	yy, _ := strconv.Atoi(d[0:2])
	mm, _ := strconv.Atoi(d[2:4])
	dd, _ := strconv.Atoi(d[4:6])
	switch {
	case mm > 70:
		mm -= 70
	case mm > 50:
		mm -= 50
	case mm > 20:
		mm -= 20
	}
	year := 1900 + yy
	if len(d) == 10 {
		if yy < 54 {
			year = 2000 + yy
		}
		n, _ := strconv.ParseUint(d, 10, 64)
		head, _ := strconv.ParseUint(d[:9], 10, 64)
		if n%11 != 0 && !(head%11 == 10 && d[9] == '0') {
			return time.Time{}, fmt.Errorf("rodné číslo nemá správny kontrolný súčet")
		}
	}
	t := time.Date(year, time.Month(mm), dd, 0, 0, 0, 0, time.UTC)
	if mm < 1 || mm > 12 || t.Day() != dd {
		return time.Time{}, fmt.Errorf("rodné číslo nemá platný dátum")
	}
	return t, nil
}

// childMonth vráti bonus dieťaťa v mesiaci (1 – 12) podľa veku: do mesiaca
// dovŕšenia 15 rokov vyšší, potom do mesiaca dovŕšenia 18 rokov nižší;
// od mesiaca narodenia.
func childMonth(born time.Time, year, month int, p Params) money.Dec {
	if year < born.Year() || (year == born.Year() && month < int(born.Month())) {
		return money.Zero
	}
	age := year - born.Year() // vek dovŕšený v tomto roku
	switch {
	case age < 15 || (age == 15 && month <= int(born.Month())):
		return p.BonusYoung
	case age < 18 || (age == 18 && month <= int(born.Month())):
		return p.BonusOld
	}
	return money.Zero
}

// bonusPct je strop bonusu v percentách základu podľa počtu detí.
func bonusPct(n int) int64 {
	if n <= 0 {
		return 0
	}
	if n > 6 {
		n = 6
	}
	return 22 + int64(n)*7 // 29, 36, 43, 50, 57, 64
}

// Bonus vypočíta daňový bonus na deti (r. 117) z navýšeného základu (r. 116a)
// po mesiacoch: bonus každého dieťaťa sa zníži o 1/10 rozdielu základu nad
// hranicou (pomerne na mesiac), súčet za mesiac najviac percento základu
// podľa počtu detí (pomerne na mesiac).
func Bonus(children []Child, base money.Dec, p Params) (money.Dec, error) {
	if base.Sign() <= 0 || len(children) == 0 {
		return money.Zero, nil
	}
	born := make([]time.Time, len(children))
	for i, c := range children {
		t, err := BirthDate(c.RC)
		if err != nil {
			return money.Zero, fmt.Errorf("dieťa %d (%s): %w", i+1, c.Name, err)
		}
		born[i] = t
	}
	red := money.Zero
	if over := base.Sub(p.BonusLimit); over.Sign() > 0 {
		red = over.MulRatio(money.FromInt(1), money.FromInt(120)).RoundCents() // 1/10 ročne → na mesiac
	}
	total := money.Zero
	for month := 1; month <= 12; month++ {
		sum, n := money.Zero, 0
		for i, c := range children {
			if !c.allYear() && !c.Months[month-1] {
				continue
			}
			a := childMonth(born[i], p.Year, month, p)
			if a.IsZero() {
				continue
			}
			n++
			sum = sum.Add(maxDec(money.Zero, a.Sub(red)))
		}
		if n == 0 {
			continue
		}
		limit := base.MulRatio(money.FromInt(bonusPct(n)), money.FromInt(1200)).RoundCents()
		total = total.Add(minDec(sum, limit))
	}
	return total.RoundCents(), nil
}

// spouseNCZD vypočíta nezdaniteľnú časť na manžela/ku (r. 74) zo základu r. 72.
func spouseNCZD(s *Spouse, base money.Dec, p Params) money.Dec {
	if s == nil || s.Months <= 0 || base.Sign() <= 0 {
		return money.Zero
	}
	months := min(s.Months, 12)
	full := p.SpouseFull
	if base.Cmp(p.SpouseLimit) > 0 {
		full = p.SpouseConst.Sub(base.MulRatio(money.FromInt(1), money.FromInt(p.SpouseDiv)).RoundCents())
	}
	year := maxDec(money.Zero, full.Sub(maxDec(money.Zero, s.OwnIncome.RoundCents())))
	if months == 12 {
		return year
	}
	return year.MulRatio(money.FromInt(int64(months)), money.FromInt(12)).RoundCents()
}

// lossRows sú riadky strát r. 47 – 50a (najstarší rok prvý) a ich odpočty r. 51 – 54a.
var lossRows = [5][2]string{{"47", "51"}, {"48", "52"}, {"49", "53"}, {"50", "54"}, {"50a", "54a"}}

// deductLosses odpočíta straty z predchádzajúcich 5 rokov od r. 45
// v poradí od najstaršej. Strata za 2020 najviac do 50 % r. 45; ostatné
// do r. 45 pri mikrodaňovníkovi, inak do 50 % r. 45 (spolu).
func deductLosses(r map[string]money.Dec, losses []Loss, year int, micro bool) money.Dec {
	base := r["45"]
	cap := base
	if !micro {
		cap = pct(base, 50)
	}
	used := money.Zero
	for i := 0; i < 5; i++ {
		y := year - 5 + i
		var l Loss
		for _, x := range losses {
			if x.Year == y {
				l = x
			}
		}
		left := maxDec(money.Zero, l.Amount.RoundCents().Sub(l.Used.RoundCents()))
		if l.Amount.Sign() > 0 {
			r["l"+lossRows[i][0]+".02"] = l.Amount.RoundCents()
			r["l"+lossRows[i][0]+".03"] = l.Used.RoundCents()
			r["l"+lossRows[i][0]+".04"] = left
		}
		limit := cap
		if y == 2020 {
			limit = minDec(limit, pct(base, 50))
		}
		take := minDec(left, maxDec(money.Zero, limit.Sub(used)))
		if take.Sign() > 0 {
			r[lossRows[i][1]] = take
			used = used.Add(take)
		}
	}
	return used
}
