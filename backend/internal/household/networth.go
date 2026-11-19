package household

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
)

// AssetKind je druh majetku.
type AssetKind string

const (
	Realty     AssetKind = "realty"     // byt, dom, pozemok
	Vehicle    AssetKind = "vehicle"    // auto, motorka
	Investment AssetKind = "investment" // fondy, akcie, dôchodkové sporenie, kryptomeny
	OtherAsset AssetKind = "other"      // iné (cennosti, vybavenie)
)

// AssetValue je hodnota majetku k dátumu (odhad, výpis z fondu, predaj = 0).
type AssetValue struct {
	Date  invoice.Date `json:"date"`
	Value money.Dec    `json:"value"`
}

// Asset je majetok domácnosti mimo účtov; hodnotu zadáva používateľ ručne.
type Asset struct {
	ID     int64        `json:"id"`
	Name   string       `json:"name"`
	Kind   AssetKind    `json:"kind"`
	Note   string       `json:"note"`
	Values []AssetValue `json:"values"` // zoradené podľa dátumu
}

// Normalize zoradí hodnoty podľa dátumu a orezá texty.
func (a *Asset) Normalize() {
	a.Name = strings.TrimSpace(a.Name)
	a.Note = strings.TrimSpace(a.Note)
	for i := range a.Values {
		a.Values[i].Value = a.Values[i].Value.RoundCents()
	}
	sort.SliceStable(a.Values, func(i, j int) bool { return a.Values[i].Date.Before(a.Values[j].Date) })
}

// Validate skontroluje majetok (po Normalize).
func (a Asset) Validate() error {
	var fe []FieldError
	if a.Name == "" {
		fe = append(fe, FieldError{Field: "name", Message: "Zadajte názov majetku"})
	}
	switch a.Kind {
	case Realty, Vehicle, Investment, OtherAsset:
	default:
		fe = append(fe, FieldError{Field: "kind", Message: "Vyberte druh majetku"})
	}
	if len(a.Values) == 0 {
		fe = append(fe, FieldError{Field: "values", Message: "Zadajte hodnotu majetku k dátumu"})
	}
	for i, v := range a.Values {
		switch {
		case v.Date.IsZero():
			fe = append(fe, FieldError{Field: fieldIdx("values", i), Message: "Zadajte dátum hodnoty"})
		case v.Value.Sign() < 0:
			fe = append(fe, FieldError{Field: fieldIdx("values", i), Message: "Hodnota nemôže byť záporná (dlh patrí medzi záväzky)"})
		case i > 0 && v.Date == a.Values[i-1].Date:
			fe = append(fe, FieldError{Field: fieldIdx("values", i), Message: "K jednému dátumu môže byť iba jedna hodnota"})
		}
	}
	return verr(fe)
}

// ValueAt vráti poslednú hodnotu majetku k dátumu (nula pred prvou).
func (a Asset) ValueAt(d invoice.Date) money.Dec {
	v := money.Zero
	for _, x := range a.Values {
		if x.Date.After(d) {
			break
		}
		v = x.Value
	}
	return v
}

// Liability: záväzok s konečnou sumou, ktorý sa ráta do dlhov.
func (o Obligation) Liability() bool {
	return o.Principal.Sign() > 0 && (o.Kind == Loan || o.Kind == Debt || o.Kind == Execution || o.Kind == Fine)
}

// OwedAt vráti, koľko zo záväzku ostáva splatiť k dátumu (pri požičaných
// niekomu koľko ešte vrátia). Záväzok sa ráta od mesiaca pred prvou
// splátkou; pravidelné platby a ukončené záväzky sú nula.
func OwedAt(o Obligation, entries []Entry, d invoice.Date) money.Dec {
	if o.Closed || o.Principal.Sign() <= 0 || (!o.Liability() && o.Kind != Lent) || o.FirstDue.AddMonths(-1).After(d) {
		return money.Zero
	}
	upto := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if !e.Date.After(d) {
			upto = append(upto, e)
		}
	}
	rest := o.Principal.Sub(Paid(o, upto))
	if rest.Sign() < 0 {
		return money.Zero
	}
	return rest
}

// NetWorth je čistý majetok domácnosti k dátumu.
type NetWorth struct {
	Date        invoice.Date `json:"date"`
	Accounts    money.Dec    `json:"accounts"`    // zostatky účtov (karta v mínuse znižuje)
	Assets      money.Dec    `json:"assets"`      // majetok (posledná zadaná hodnota)
	Receivables money.Dec    `json:"receivables"` // požičané niekomu, ešte nevrátené
	Debts       money.Dec    `json:"debts"`       // zostatok pôžičiek, dlhov, exekúcií
	Total       money.Dec    `json:"total"`       // účty + majetok + pohľadávky − dlhy
}

// NetWorthAt spočíta čistý majetok k dátumu.
func NetWorthAt(accs []Account, entries []Entry, assets []Asset, obls []Obligation, d invoice.Date) NetWorth {
	n := NetWorth{Date: d}
	for _, a := range accs {
		n.Accounts = n.Accounts.Add(BalanceAt(a, entries, d))
	}
	for _, a := range assets {
		n.Assets = n.Assets.Add(a.ValueAt(d))
	}
	for _, o := range obls {
		owed := OwedAt(o, entries, d)
		if o.Kind == Lent {
			n.Receivables = n.Receivables.Add(owed)
		} else {
			n.Debts = n.Debts.Add(owed)
		}
	}
	n.Total = n.Accounts.Add(n.Assets).Add(n.Receivables).Sub(n.Debts)
	return n
}

// MonthEnds vráti konce posledných n mesiacov pred dnešným mesiacom a dnešok
// (od najstaršieho).
func MonthEnds(today invoice.Date, n int) []invoice.Date {
	first := invoice.NewDate(today.Year(), time.Month(today.Month()), 1)
	out := make([]invoice.Date, 0, n+1)
	for i := n; i >= 1; i-- {
		out = append(out, first.AddMonths(-i+1).AddDays(-1))
	}
	return append(out, today)
}

func fieldIdx(name string, i int) string { return name + "." + strconv.Itoa(i) }
