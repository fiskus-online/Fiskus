package household

import (
	"sort"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/money"
)

// Partner je kontakt z adresára, s ktorým sa porovnávajú pohyby domácnosti.
type Partner struct {
	ID   int64
	Name string
	IBAN string
}

func normIBAN(s string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
}

// MatchPartner nájde partnera k pohybu z banky: najprv podľa IBAN
// protiúčtu, potom podľa rovnakého mena (bez diakritiky a veľkosti
// písmen). Pri viacerých partneroch s rovnakým menom nevyberie nikoho.
// Vráti 0, ak sa nenašiel.
func MatchPartner(ps []Partner, iban, name string) int64 {
	if iban = normIBAN(iban); iban != "" {
		for _, p := range ps {
			if normIBAN(p.IBAN) == iban {
				return p.ID
			}
		}
	}
	n := fold(name)
	if n == "" {
		return 0
	}
	var id int64
	for _, p := range ps {
		if fold(p.Name) == n {
			if id != 0 {
				return 0
			}
			id = p.ID
		}
	}
	return id
}

// OfPartner povie, či pohyb patrí partnerovi: má jeho ContactID, alebo
// nemá žiadny a príjemca sa volá rovnako (staršie a ručne písané pohyby).
func (e Entry) OfPartner(p Partner) bool {
	if e.ContactID != 0 {
		return e.ContactID == p.ID
	}
	n := fold(p.Name)
	return n != "" && fold(e.Payee) == n
}

// OfPartner povie, či záväzok patrí partnerovi (ako pri pohybe, podľa veriteľa).
func (o Obligation) OfPartner(p Partner) bool {
	if o.ContactID != 0 {
		return o.ContactID == p.ID
	}
	n := fold(p.Name)
	return n != "" && fold(o.Creditor) == n
}

// PartnerTotals sú platby s partnerom spolu a v roku Year.
type PartnerTotals struct {
	Year         int       `json:"year"`
	Paid         money.Dec `json:"paid"`         // zaplatené partnerovi spolu
	Received     money.Dec `json:"received"`     // prijaté od partnera spolu
	YearPaid     money.Dec `json:"yearPaid"`     // zaplatené v roku Year
	YearReceived money.Dec `json:"yearReceived"` // prijaté v roku Year
	Count        int       `json:"count"`        // počet pohybov
}

// PartnerEntries vyberie pohyby partnera (bez prevodov medzi účtami),
// najnovšie prvé, a spočíta platby spolu a v roku year.
func PartnerEntries(es []Entry, p Partner, year int) ([]Entry, PartnerTotals) {
	t := PartnerTotals{Year: year}
	var out []Entry
	for _, e := range es {
		if e.Transfer() || !e.OfPartner(p) {
			continue
		}
		out = append(out, e)
		inYear := e.Date.Year() == year
		if e.Amount.Sign() < 0 {
			t.Paid = t.Paid.Add(e.Amount.Neg())
			if inYear {
				t.YearPaid = t.YearPaid.Add(e.Amount.Neg())
			}
		} else {
			t.Received = t.Received.Add(e.Amount)
			if inYear {
				t.YearReceived = t.YearReceived.Add(e.Amount)
			}
		}
	}
	t.Count = len(out)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Date, out[j].Date
		if a.Before(b) || b.Before(a) {
			return b.Before(a)
		}
		return out[i].ID > out[j].ID
	})
	return out, t
}
