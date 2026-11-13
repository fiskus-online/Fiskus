package household

import (
	"sort"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/money"
)

// Partner je kontakt z adresára, s ktorým sa porovnávajú pohyby domácnosti.
type Partner struct {
	ID    int64
	Name  string
	IBAN  string
	IBANs []string // ďalšie účty partnera (škola: stravné, poplatky…)
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
			for _, x := range p.IBANs {
				if normIBAN(x) == iban {
					return p.ID
				}
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

// ObligationMatch je záväzok, ku ktorému patrí platba z banky.
type ObligationMatch struct {
	ObligationID int64  // 0 = nedá sa jednoznačne určiť
	Category     string // kategória záväzku (aj keď je kandidátov viac s rovnakou kategóriou)
	ContactID    int64  // partner záväzku (ak majú všetci kandidáti rovnakého)
	Payee        string // komu (veriteľ), ak ho výpis nemá
}

// MatchObligation nájde otvorený záväzok k platbe z banky: rovnaký IBAN
// protiúčtu a suma splátky (výdavok pri dlhoch a platbách, príjem pri
// požičaných). Pri viacerých záväzkoch na ten istý účet (škola: hudobná
// pre troch súrodencov) rozhodne správa platby — slová z názvu záväzku
// alebo zo správy pre príjemcu (stačí rovnaký začiatok slova, aspoň tri
// písmená: Zuzana – Zuzka). Ak sa záväzok nedá určiť, vráti aspoň
// spoločnú kategóriu.
func MatchObligation(os []Obligation, iban string, amount money.Dec, text string) ObligationMatch {
	iban = normIBAN(iban)
	if iban == "" || amount.IsZero() {
		return ObligationMatch{}
	}
	income := amount.Sign() > 0
	abs := amount
	if !income {
		abs = amount.Neg()
	}
	var cands []Obligation
	for _, o := range os {
		if o.Closed || o.Income() != income || normIBAN(o.IBAN) != iban || o.Installment.Cmp(abs) != 0 {
			continue
		}
		cands = append(cands, o)
	}
	if len(cands) == 0 {
		return ObligationMatch{}
	}
	var m ObligationMatch
	m.Category, m.ContactID, m.Payee = cands[0].Category, cands[0].ContactID, cands[0].Creditor
	for _, o := range cands[1:] {
		if o.Category != m.Category {
			m.Category = ""
		}
		if o.ContactID != m.ContactID {
			m.ContactID = 0
		}
		if o.Creditor != m.Payee {
			m.Payee = ""
		}
	}
	if len(cands) == 1 {
		m.ObligationID = cands[0].ID
		return m
	}
	words := Words(text)
	has := func(w string) bool {
		for _, t := range words {
			if samePrefix(w, t, 3) {
				return true
			}
		}
		return false
	}
	best, bestScore, tie := int64(0), 0, false
	for _, o := range cands {
		score := 0
		// správa pre príjemcu pri záväzku sedí celá → rozhoduje
		if msg := longWords(o.Message); len(msg) > 0 {
			all := true
			for _, w := range msg {
				all = all && has(w)
			}
			if all {
				score += 10
			}
		}
		// slová názvu záväzku (meno dieťaťa): stačí rovnaký začiatok
		for _, w := range longWords(o.Name) {
			if has(w) {
				score++
			}
		}
		switch {
		case score > bestScore:
			best, bestScore, tie = o.ID, score, false
		case score == bestScore && score > 0:
			tie = true
		}
	}
	if bestScore > 0 && !tie {
		m.ObligationID = best
		for _, o := range cands {
			if o.ID == best {
				m.Category, m.ContactID, m.Payee = o.Category, o.ContactID, o.Creditor
			}
		}
	}
	return m
}

// samePrefix: obe slová majú aspoň n znakov a rovnaký začiatok n znakov.
func samePrefix(a, b string, n int) bool {
	ra, rb := []rune(a), []rune(b)
	if len(ra) < n || len(rb) < n {
		return false
	}
	return string(ra[:n]) == string(rb[:n])
}

// longWords sú slová textu s aspoň 4 znakmi (kratšie nerozlišujú).
func longWords(s string) []string {
	var out []string
	for _, w := range Words(s) {
		if len([]rune(w)) >= 4 {
			out = append(out, w)
		}
	}
	return out
}
