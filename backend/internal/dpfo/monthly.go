package dpfo

import (
	"fmt"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Mesačné výpočty zamestnávateľa pri mzde (§ 35 zákona o dani z príjmov):
// nezdaniteľná časť na daňovníka, preddavok na daň a daňový bonus z
// mesačného základu dane (zdaniteľná mzda mínus poistné zamestnanca).

var twelve = money.FromInt(12)

// MonthlyNCZD je 1/12 nezdaniteľnej časti na daňovníka podľa mesačného
// základu dane (ročná hranica a vzorec prepočítané na mesiac), zaokrúhlená
// na eurocent nahor.
func MonthlyNCZD(base money.Dec, p Params) money.Dec {
	if base.Sign() <= 0 {
		return money.Zero
	}
	year := base.Mul(twelve)
	v := p.NCZD
	if year.Cmp(p.NCZDLimit) > 0 {
		v = p.NCZDConst.Sub(year.MulRatio(money.FromInt(1), money.FromInt(p.NCZDDiv)))
	}
	if v.Sign() <= 0 {
		return money.Zero
	}
	return v.MulRatio(money.FromInt(1), twelve).CeilCents()
}

// MonthlyTax je preddavok na daň z mesačného základu dane (pred daňovým
// bonusom): pásma sadzieb sú 1/12 ročných, výsledok na eurocent nadol.
func MonthlyTax(base money.Dec, p Params) money.Dec {
	if base.Sign() <= 0 {
		return money.Zero
	}
	tax, from := money.Zero, money.Zero
	for _, b := range p.Brackets {
		part := base.Sub(from)
		if !b.Upto.IsZero() {
			upto := b.Upto.MulRatio(money.FromInt(1), twelve).RoundCents()
			part = minDec(base, upto).Sub(from)
			from = upto
		}
		if part.Sign() > 0 {
			tax = tax.Add(part.MulRatio(money.FromInt(b.Rate), money.FromInt(100)))
		}
		if b.Upto.IsZero() || base.Cmp(from) <= 0 {
			break
		}
	}
	return tax.FloorCents()
}

// MonthlyBonus je daňový bonus na deti v mesiaci (1 – 12): suma podľa veku
// dieťaťa znížená o 1/10 základu nad 1/12 ročnej hranice, spolu najviac
// percento základu podľa počtu detí.
func MonthlyBonus(children []Child, base money.Dec, month int, p Params) (money.Dec, error) {
	if base.Sign() <= 0 || len(children) == 0 {
		return money.Zero, nil
	}
	red := money.Zero
	if over := base.Sub(p.BonusLimit.MulRatio(money.FromInt(1), twelve).RoundCents()); over.Sign() > 0 {
		red = over.MulRatio(money.FromInt(1), money.FromInt(10)).RoundCents()
	}
	sum, n := money.Zero, 0
	for i, c := range children {
		if !c.allYear() && !c.Months[month-1] {
			continue
		}
		born, err := BirthDate(c.RC)
		if err != nil {
			return money.Zero, fmt.Errorf("dieťa %d (%s): %w", i+1, c.Name, err)
		}
		a := childMonth(born, p.Year, month, p)
		if a.IsZero() {
			continue
		}
		n++
		sum = sum.Add(maxDec(money.Zero, a.Sub(red)))
	}
	if n == 0 {
		return money.Zero, nil
	}
	limit := base.MulRatio(money.FromInt(bonusPct(n)), money.FromInt(100)).RoundCents()
	return minDec(sum, limit), nil
}
