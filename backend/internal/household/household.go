// Package household je domáce účtovníctvo: účty domácnosti (hotovosť,
// banka, karta, sporenie), príjmy a výdavky podľa kategórií, prevody medzi
// účtami a mesačný rozpočet. Domácnosť je v programe samostatná „firma“
// bez IČO, DPH a dokladov; peniaze zo živnosti do nej prichádzajú ako
// výber podnikateľa (príjem domácnosti).
package household

import (
	"fmt"
	"github.com/fiskus-online/fiskus/backend/internal/ekasa"
	"sort"
	"strings"
	"unicode"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
)

// AccountKind je druh účtu domácnosti.
type AccountKind string

const (
	Cash    AccountKind = "cash"    // hotovosť, peňaženka
	Bank    AccountKind = "bank"    // bežný účet
	Card    AccountKind = "card"    // kreditná karta (zostatok býva záporný)
	Savings AccountKind = "savings" // sporiaci účet, vkladná knižka
)

// Account je účet domácnosti.
type Account struct {
	ID        int64       `json:"id"`
	CompanyID int64       `json:"companyId"`
	Name      string      `json:"name"`
	Kind      AccountKind `json:"kind"`
	IBAN      string      `json:"iban"`
	// Opening je počiatočný zostatok (stav pred prvým pohybom vo Fiškuse).
	Opening money.Dec `json:"opening"`
	Closed  bool      `json:"closed"` // zrušený účet sa neponúka pri nových pohyboch
}

// Entry je pohyb peňazí v domácnosti. Amount má znamienko: príjem kladný,
// výdavok záporný. Prevod medzi účtami má ToAccountID a kladnú sumu, ktorá
// z účtu AccountID odíde na účet ToAccountID; kategóriu nemá.
type Entry struct {
	ID          int64        `json:"id"`
	CompanyID   int64        `json:"companyId"`
	Date        invoice.Date `json:"date"`
	AccountID   int64        `json:"accountId"`
	ToAccountID int64        `json:"toAccountId,omitempty"`
	Amount      money.Dec    `json:"amount"`
	Category    string       `json:"category"`
	Payee       string       `json:"payee"` // obchod, firma, osoba
	// ContactID je partner z adresára (0 = iba text v Payee).
	ContactID int64  `json:"contactId,omitempty"`
	Note      string `json:"note"`
	// Split je rozdelenie pohybu na položky s vlastnou kategóriou (bloček
	// s potravinami aj drogériou); súčet položiek je Amount. Category je
	// potom kategória najväčšej položky.
	Split []Split `json:"split,omitempty"`
	// ObligationID je záväzok, ktorého splátkou pohyb je (0 = žiadny).
	ObligationID int64 `json:"obligationId,omitempty"`
	// Source je pôvod pohybu z importu ("bank:<odtlačok>", "biz:<firma>:<pohyb>");
	// pohyb s rovnakým pôvodom sa druhýkrát neuloží.
	Source string `json:"source,omitempty"`
	// Receipt sú údaje bločku (z eKasy alebo prečítané z fotky) — predajca,
	// predajňa, pokladnica, položky s cenou za kus a DPH, rekapitulácia DPH.
	// Ukladajú sa pri zápise bločku a ďalej sa nemenia.
	Receipt *ekasa.Receipt `json:"receipt,omitempty"`
	// Paired je pôvod pohybu z banky ("bank:<odtlačok>"), s ktorým je bloček
	// spárovaný (zaplatený kartou): bloček je potom na bankovom účte a ten
	// pohyb z banky sa už znova nenačíta.
	Paired string `json:"paired,omitempty"`
}

// Split je položka rozdeleného pohybu (suma so znamienkom ako pohyb).
type Split struct {
	Category string    `json:"category"`
	Amount   money.Dec `json:"amount"`
	Note     string    `json:"note"` // názov položky z bločku
	Quantity string    `json:"quantity,omitempty"`
}

// Parts vráti časti pohybu na súčty: položky rozdeleného pohybu, inak
// pohyb celý.
func (e Entry) Parts() []Split {
	if len(e.Split) > 0 {
		return e.Split
	}
	return []Split{{Category: e.Category, Amount: e.Amount}}
}

// Transfer: je pohyb prevod medzi účtami?
func (e Entry) Transfer() bool { return e.ToAccountID != 0 }

// Category je kategória príjmov alebo výdavkov s mesačným rozpočtom
// (pri výdavkoch najviac, pri príjmoch očakávaná suma; nula = bez rozpočtu).
type Category struct {
	Name   string    `json:"name"`
	Income bool      `json:"income"`
	Budget money.Dec `json:"budget"`
}

// DefaultCategories sú kategórie novej domácnosti.
var DefaultCategories = []Category{
	{Name: "Mzda a príjem zo živnosti", Income: true},
	{Name: "Dávky a príspevky", Income: true},
	{Name: "Iné príjmy", Income: true},
	{Name: "Bývanie"},
	{Name: "Energie"},
	{Name: "Potraviny"},
	{Name: "Drogéria a domácnosť"},
	{Name: "Auto a doprava"},
	{Name: "Telefón a internet"},
	{Name: "Zdravie"},
	{Name: "Deti a škola"},
	{Name: "Oblečenie"},
	{Name: "Voľný čas"},
	{Name: "Reštaurácie"},
	{Name: "Poistenie"},
	{Name: "Splátky a dlhy"},
	{Name: "Dary"},
	{Name: "Iné výdavky"},
}

// FieldError je chyba poľa formulára (ako pri dokladoch).
type FieldError = invoice.FieldError

func verr(fe []FieldError) error {
	if len(fe) == 0 {
		return nil
	}
	return &invoice.ValidationError{Fields: fe}
}

// Normalize upraví účet pred uložením.
func (a *Account) Normalize() {
	a.Name = strings.TrimSpace(a.Name)
	a.IBAN = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(a.IBAN), " ", ""))
	if a.Kind == "" {
		a.Kind = Bank
	}
	a.Opening = a.Opening.RoundCents()
}

// Validate skontroluje účet.
func (a Account) Validate() error {
	var fe []FieldError
	if a.Name == "" {
		fe = append(fe, FieldError{Field: "name", Message: "Zadajte názov účtu"})
	}
	switch a.Kind {
	case Cash, Bank, Card, Savings:
	default:
		fe = append(fe, FieldError{Field: "kind", Message: "Neznámy druh účtu"})
	}
	return verr(fe)
}

// Normalize upraví pohyb pred uložením.
func (e *Entry) Normalize() {
	e.Category = strings.TrimSpace(e.Category)
	e.Payee = strings.TrimSpace(e.Payee)
	e.Note = strings.TrimSpace(e.Note)
	e.Amount = e.Amount.RoundCents()
	if e.Transfer() {
		e.Category = ""
		e.Split = nil
	}
	if len(e.Split) > 0 {
		// kategória pohybu = kategória s najväčšou sumou (na zobrazenie)
		by := map[string]money.Dec{}
		best := ""
		for i := range e.Split {
			x := &e.Split[i]
			x.Category = strings.TrimSpace(x.Category)
			x.Note = strings.TrimSpace(x.Note)
			x.Amount = x.Amount.RoundCents()
			by[x.Category] = by[x.Category].Add(x.Amount)
		}
		for _, x := range e.Split {
			if best == "" || absDec(by[x.Category]).Cmp(absDec(by[best])) > 0 {
				best = x.Category
			}
		}
		e.Category = best
	}
}

func absDec(d money.Dec) money.Dec {
	if d.Sign() < 0 {
		return d.Neg()
	}
	return d
}

// Validate skontroluje pohyb.
func (e Entry) Validate() error {
	var fe []FieldError
	if e.Date.IsZero() {
		fe = append(fe, FieldError{Field: "date", Message: "Zadajte dátum"})
	}
	if e.AccountID == 0 {
		fe = append(fe, FieldError{Field: "accountId", Message: "Vyberte účet"})
	}
	if e.Amount.IsZero() {
		fe = append(fe, FieldError{Field: "amount", Message: "Zadajte sumu"})
	}
	if e.Transfer() {
		if e.ToAccountID == e.AccountID {
			fe = append(fe, FieldError{Field: "toAccountId", Message: "Prevod musí ísť na iný účet"})
		}
		if e.Amount.Sign() < 0 {
			fe = append(fe, FieldError{Field: "amount", Message: "Suma prevodu je kladná"})
		}
	} else if e.Category == "" {
		fe = append(fe, FieldError{Field: "category", Message: "Vyberte kategóriu"})
	}
	if len(e.Split) > 0 {
		sum := money.Zero
		for i, x := range e.Split {
			if x.Category == "" {
				fe = append(fe, FieldError{Field: fmt.Sprintf("split.%d", i), Message: "Vyberte kategóriu položky"})
			}
			sum = sum.Add(x.Amount)
		}
		if !sum.Equal(e.Amount) {
			fe = append(fe, FieldError{Field: "split", Message: fmt.Sprintf("Súčet položiek %s € nesedí so sumou %s €", sum.Display(), e.Amount.Display())})
		}
	}
	return verr(fe)
}

// Balances vráti zostatky účtov (počiatočný stav + pohyby).
func Balances(accounts []Account, entries []Entry) map[int64]money.Dec {
	out := make(map[int64]money.Dec, len(accounts))
	for _, a := range accounts {
		out[a.ID] = a.Opening
	}
	for _, e := range entries {
		if e.Transfer() {
			out[e.AccountID] = out[e.AccountID].Sub(e.Amount)
			out[e.ToAccountID] = out[e.ToAccountID].Add(e.Amount)
			continue
		}
		out[e.AccountID] = out[e.AccountID].Add(e.Amount)
	}
	return out
}

// CategoryMonth je súčet kategórie po mesiacoch (index 0 = január).
type CategoryMonth struct {
	Name   string        `json:"name"`
	Income bool          `json:"income"`
	Budget money.Dec     `json:"budget"` // mesačný rozpočet
	Months [12]money.Dec `json:"months"`
	Total  money.Dec     `json:"total"`
}

// Overview je ročný prehľad domácnosti.
type Overview struct {
	Year       int             `json:"year"`
	Income     [12]money.Dec   `json:"income"`  // príjmy po mesiacoch
	Expense    [12]money.Dec   `json:"expense"` // výdavky po mesiacoch (kladné)
	TotalIn    money.Dec       `json:"totalIn"`
	TotalOut   money.Dec       `json:"totalOut"`
	Net        money.Dec       `json:"net"` // príjmy − výdavky
	Categories []CategoryMonth `json:"categories"`
}

// Summarize zostaví prehľad roka. Prevody medzi účtami sa nerátajú. Výdavky
// sú v prehľade kladné; vrátenie peňazí (kladná suma vo výdavkovej
// kategórii) výdavky kategórie zníži.
func Summarize(year int, cats []Category, entries []Entry) Overview {
	ov := Overview{Year: year}
	idx := map[string]int{}
	for _, c := range cats {
		idx[c.Name] = len(ov.Categories)
		ov.Categories = append(ov.Categories, CategoryMonth{Name: c.Name, Income: c.Income, Budget: c.Budget})
	}
	for _, e := range entries {
		if e.Transfer() || e.Date.Year() != year {
			continue
		}
		for _, part := range e.Parts() {
			i, ok := idx[part.Category]
			if !ok {
				// kategória, ktorá už v zozname nie je — znamienko pohybu určí druh
				i = len(ov.Categories)
				idx[part.Category] = i
				ov.Categories = append(ov.Categories, CategoryMonth{Name: part.Category, Income: e.Amount.Sign() > 0})
			}
			c := &ov.Categories[i]
			m := e.Date.Month() - 1
			v := part.Amount
			if !c.Income {
				v = v.Neg()
				ov.Expense[m] = ov.Expense[m].Add(v)
				ov.TotalOut = ov.TotalOut.Add(v)
			} else {
				ov.Income[m] = ov.Income[m].Add(v)
				ov.TotalIn = ov.TotalIn.Add(v)
			}
			c.Months[m] = c.Months[m].Add(v)
			c.Total = c.Total.Add(v)
		}
	}
	ov.Net = ov.TotalIn.Sub(ov.TotalOut)
	return ov
}

// fold zjednoduší text na porovnanie (bez diakritiky, malé písmená).
func fold(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if f, ok := foldMap[r]; ok {
			r = f
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// Words rozdelí text na slová bez diakritiky a malými písmenami.
func Words(s string) []string { return strings.Fields(fold(s)) }

var foldMap = map[rune]rune{'á': 'a', 'ä': 'a', 'č': 'c', 'ď': 'd', 'é': 'e', 'ě': 'e', 'í': 'i', 'ĺ': 'l', 'ľ': 'l', 'ň': 'n',
	'ó': 'o', 'ô': 'o', 'ö': 'o', 'ŕ': 'r', 'ř': 'r', 'š': 's', 'ť': 't', 'ú': 'u', 'ů': 'u', 'ü': 'u', 'ý': 'y', 'ž': 'z'}

// rules sú známe obchody a firmy → kategória (ak história nič nevie).
var rules = []struct {
	words []string
	cat   string
}{
	{[]string{"lidl", "tesco", "billa", "kaufland", "coop jednota", "jednota", "fresh", "terno", "albert", "potraviny", "mercury market"}, "Potraviny"},
	{[]string{"dm drogerie", "dm drogeria", "teta", "rossmann", "drogeria", "ikea", "hornbach", "obi", "baumax", "mountfield"}, "Drogéria a domácnosť"},
	{[]string{"slovnaft", "omv", "shell", "orlen", "mol ", "tankovanie", "cerpacia", "dialnicna", "eznamka", "parkovanie", "zssk", "regiojet", "dpb", "flixbus", "bolt", "uber"}, "Auto a doprava"},
	{[]string{"orange", "telekom", "o2 slovakia", "4ka", "swan", "upc", "digi slovakia", "antik"}, "Telefón a internet"},
	{[]string{"zse", "sse", "vse", "spp", "plyn", "elektrina", "vodaren", "vodarensk", "stvps", "teplaren"}, "Energie"},
	{[]string{"lekaren", "dr max", "benu", "lekar", "nemocnic", "zubar", "optik"}, "Zdravie"},
	{[]string{"allianz", "kooperativa", "generali", "union poist", "uniqa", "komunalna poist", "poistovna", "poistenie"}, "Poistenie"},
	{[]string{"najom", "najomne", "spolocenstvo", "sbd", "fond oprav", "sprava domu", "hypoteka"}, "Bývanie"},
	{[]string{"exekut", "splatka", "home credit", "cofidis", "pohotovost", "provident", "leasing", "uver"}, "Splátky a dlhy"},
	{[]string{"restaur", "pizzeria", "bistro", "kaviaren", "mcdonald", "kfc", "burger", "wolt", "foodora", "bistro.sk"}, "Reštaurácie"},
	{[]string{"netflix", "spotify", "kino", "cinema city", "steam", "playstation", "decathlon", "sportisimo", "booking"}, "Voľný čas"},
	{[]string{"zara", "h m", "reserved", "c a", "deichmann", "ccc", "pepco", "kik"}, "Oblečenie"},
	{[]string{"skola", "skolka", "druzina", "krouzok", "kruzok", "skolne"}, "Deti a škola"},
}

// Suggest navrhne kategóriu podľa obchodu alebo popisu: najprv podľa
// posledného pohybu s rovnakým obchodom, potom podľa známych obchodov.
// Kategória musí byť v zozname (inak sa nenavrhne).
func Suggest(text string, income bool, history []Entry, cats []Category) string {
	have := map[string]bool{}
	for _, c := range cats {
		if c.Income == income {
			have[c.Name] = true
		}
	}
	key := fold(text)
	if key == "" {
		return ""
	}
	// história: najnovší pohyb s rovnakým obchodom
	hist := append([]Entry(nil), history...)
	sort.SliceStable(hist, func(i, j int) bool { return hist[j].Date.Before(hist[i].Date) })
	for _, e := range hist {
		if e.Transfer() || !have[e.Category] {
			continue
		}
		if p := fold(e.Payee); p != "" && (p == key || strings.HasPrefix(key, p+" ") || strings.HasPrefix(p, key+" ")) {
			return e.Category
		}
	}
	padded := " " + key + " "
	for _, r := range rules {
		if !have[r.cat] {
			continue
		}
		for _, w := range r.words {
			if strings.Contains(padded, " "+w) {
				return r.cat
			}
		}
	}
	return ""
}

// MonthBudget je čerpanie rozpočtu kategórie v mesiaci.
type MonthBudget struct {
	Name    string    `json:"name"`
	Income  bool      `json:"income"`
	Budget  money.Dec `json:"budget"`
	Actual  money.Dec `json:"actual"`
	Left    money.Dec `json:"left"`    // rozpočet − skutočnosť (výdavky)
	Percent int64     `json:"percent"` // čerpanie v percentách (zaokrúhlené nadol)
	Over    bool      `json:"over"`    // výdavky nad rozpočet
}

// Budget vráti čerpanie rozpočtu výdavkových kategórií v mesiaci
// (iba kategórie s rozpočtom alebo s výdavkom).
func Budget(ov Overview, month int) []MonthBudget {
	out := []MonthBudget{}
	if month < 1 || month > 12 {
		return out
	}
	for _, c := range ov.Categories {
		if c.Income {
			continue
		}
		act := c.Months[month-1]
		if c.Budget.IsZero() && act.IsZero() {
			continue
		}
		b := MonthBudget{Name: c.Name, Budget: c.Budget, Actual: act, Left: c.Budget.Sub(act)}
		if c.Budget.Sign() > 0 {
			b.Percent = act.MulRatio(money.FromInt(100), c.Budget).Raw() / money.FromInt(1).Raw()
			b.Over = act.Cmp(c.Budget) > 0
		}
		out = append(out, b)
	}
	return out
}
