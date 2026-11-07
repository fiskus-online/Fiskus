// Package forms je engine na tlačivá: tlačivo je definícia (polia,
// tabuľka položiek, rozloženie a papier), vyplnené tlačivo sú hodnoty polí.
// Vstavané tlačivá (pokladničné doklady, dodací list, objednávka …) sú
// definície v kóde, vlastné si firma zadá sama. Sumy počíta Compute —
// prehliadač ich iba zobrazuje.
package forms

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

// FieldType je druh poľa.
type FieldType string

const (
	Text     FieldType = "text"
	Textarea FieldType = "textarea" // viac riadkov (adresa, text splnomocnenia)
	Money    FieldType = "money"
	Number   FieldType = "number"
	Date     FieldType = "date"
	Rate     FieldType = "rate" // sadzba DPH; prázdne = bez DPH
)

var fieldTypes = []FieldType{Text, Textarea, Money, Number, Date, Rate}

// Layout je rozloženie tlače.
type Layout string

const (
	LayoutCash     Layout = "cash"     // pokladničný doklad (rámček, suma slovom, podpisy)
	LayoutDocument Layout = "document" // list: hlavička, strany, polia, položky, podpisy
)

// Field je pole tlačiva.
type Field struct {
	Key      string    `json:"key"`
	Label    string    `json:"label"`
	Type     FieldType `json:"type"`
	Required bool      `json:"required"`
	Wide     bool      `json:"wide"` // cez celú šírku
	Hint     string    `json:"hint,omitempty"`
	// Role je význam poľa pre tlač a zápis do pokladne: number, date,
	// issuer (vystavil – firma), partner, purpose, amount, rate, note,
	// sign1, sign2 (podpisy).
	Role string `json:"role,omitempty"`
}

// Form je definícia tlačiva.
type Form struct {
	Key         string  `json:"key"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Layout      Layout  `json:"layout"`
	Paper       string  `json:"paper"` // A4, A5
	Fields      []Field `json:"fields"`
	Items       bool    `json:"items"` // tabuľka položiek (popis, množstvo, MJ, cena)
	// Cash: tlačivo sa dá zapísať do pokladne ako účtovný doklad ("in", "out").
	Cash    string `json:"cash,omitempty"`
	Builtin bool   `json:"builtin"`
}

// Item je riadok tabuľky položiek.
type Item struct {
	Desc  string    `json:"desc"`
	Qty   money.Dec `json:"qty"`
	Unit  string    `json:"unit"`
	Price money.Dec `json:"price"`
}

// Values sú hodnoty polí (kľúč → text, sumy a dátumy v tvare pre API).
type Values map[string]string

// Computed sú dopočítané hodnoty na tlač.
type Computed struct {
	Base  money.Dec   `json:"base"`
	VAT   money.Dec   `json:"vat"`
	Total money.Dec   `json:"total"`
	Rate  *vat.Rate   `json:"rate,omitempty"`
	Lines []money.Dec `json:"lines"` // suma každej položky
	// ItemsTotal je súčet položiek (bez DPH).
	ItemsTotal money.Dec `json:"itemsTotal"`
}

func cashFields(partner string) []Field {
	return []Field{
		{Key: "issuer", Label: "Organizácia", Type: Textarea, Role: "issuer", Hint: "názov, adresa, IČO"},
		{Key: "number", Label: "Číslo dokladu", Type: Text, Role: "number"},
		{Key: "date", Label: "Dátum", Type: Date, Required: true, Role: "date"},
		{Key: "partner", Label: partner, Type: Textarea, Required: true, Role: "partner", Hint: "meno a adresa"},
		{Key: "purpose", Label: "Účel platby", Type: Text, Required: true, Wide: true, Role: "purpose"},
		{Key: "amount", Label: "Suma €", Type: Money, Required: true, Role: "amount", Hint: "vrátane DPH"},
		{Key: "rate", Label: "Sadzba DPH", Type: Rate, Role: "rate"},
		{Key: "accounting", Label: "Účtovací predpis", Type: Text, Hint: "MD / D"},
		{Key: "made", Label: "Vyhotovil", Type: Text, Role: "sign1"},
		{Key: "approved", Label: "Schválil", Type: Text, Role: "sign2"},
		{Key: "note", Label: "Poznámka", Type: Text, Wide: true, Role: "note"},
	}
}

// Builtin sú vstavané tlačivá.
func Builtin() []Form {
	return []Form{
		{Key: "ppd", Name: "Príjmový pokladničný doklad", Description: "Príjem hotovosti do pokladne.", Layout: LayoutCash, Paper: "A5",
			Fields: cashFields("Prijaté od"), Cash: "in", Builtin: true},
		{Key: "vpd", Name: "Výdavkový pokladničný doklad", Description: "Výdaj hotovosti z pokladne.", Layout: LayoutCash, Paper: "A5",
			Fields: cashFields("Vyplatené komu"), Cash: "out", Builtin: true},
		{Key: "dodaci-list", Name: "Dodací list", Description: "Potvrdenie o dodaní tovaru s položkami.", Layout: LayoutDocument, Paper: "A4", Items: true, Builtin: true,
			Fields: []Field{
				{Key: "number", Label: "Číslo", Type: Text, Role: "number"},
				{Key: "date", Label: "Dátum dodania", Type: Date, Required: true, Role: "date"},
				{Key: "issuer", Label: "Dodávateľ", Type: Textarea, Required: true, Role: "issuer"},
				{Key: "partner", Label: "Odberateľ", Type: Textarea, Required: true, Role: "partner"},
				{Key: "order", Label: "Číslo objednávky", Type: Text},
				{Key: "place", Label: "Miesto dodania", Type: Text},
				{Key: "note", Label: "Poznámka", Type: Textarea, Wide: true, Role: "note"},
				{Key: "made", Label: "Odovzdal", Type: Text, Role: "sign1"},
				{Key: "approved", Label: "Prevzal", Type: Text, Role: "sign2"},
			}},
		{Key: "objednavka", Name: "Objednávka", Description: "Objednávka tovaru alebo služby u dodávateľa.", Layout: LayoutDocument, Paper: "A4", Items: true, Builtin: true,
			Fields: []Field{
				{Key: "number", Label: "Číslo", Type: Text, Role: "number"},
				{Key: "date", Label: "Dátum", Type: Date, Required: true, Role: "date"},
				{Key: "issuer", Label: "Objednávateľ", Type: Textarea, Required: true, Role: "issuer"},
				{Key: "partner", Label: "Dodávateľ", Type: Textarea, Required: true, Role: "partner"},
				{Key: "deadline", Label: "Termín dodania", Type: Date},
				{Key: "place", Label: "Miesto dodania", Type: Text},
				{Key: "note", Label: "Poznámka", Type: Textarea, Wide: true, Role: "note"},
				{Key: "made", Label: "Vystavil", Type: Text, Role: "sign1"},
			}},
		{Key: "prevzatie", Name: "Potvrdenie o prevzatí", Description: "Prevzatie peňazí, vecí alebo dokladov.", Layout: LayoutDocument, Paper: "A5", Builtin: true,
			Fields: []Field{
				{Key: "date", Label: "Dátum", Type: Date, Required: true, Role: "date"},
				{Key: "issuer", Label: "Odovzdávajúci", Type: Textarea, Required: true, Role: "issuer"},
				{Key: "partner", Label: "Preberajúci", Type: Textarea, Required: true, Role: "partner"},
				{Key: "subject", Label: "Predmet prevzatia", Type: Textarea, Required: true, Wide: true, Role: "purpose"},
				{Key: "amount", Label: "Suma €", Type: Money, Role: "amount", Hint: "ak ide o peniaze"},
				{Key: "place", Label: "Miesto", Type: Text},
				{Key: "made", Label: "Odovzdal", Type: Text, Role: "sign1"},
				{Key: "approved", Label: "Prevzal", Type: Text, Role: "sign2"},
			}},
		{Key: "splnomocnenie", Name: "Splnomocnenie", Description: "Plnomocenstvo na zastupovanie (úrad, pošta, prevzatie zásielky).", Layout: LayoutDocument, Paper: "A4", Builtin: true,
			Fields: []Field{
				{Key: "issuer", Label: "Splnomocniteľ", Type: Textarea, Required: true, Role: "issuer", Hint: "meno, adresa, dátum narodenia alebo IČO"},
				{Key: "partner", Label: "Splnomocnenec", Type: Textarea, Required: true, Role: "partner", Hint: "meno, adresa, dátum narodenia"},
				{Key: "subject", Label: "Na čo splnomocňujem", Type: Textarea, Required: true, Wide: true, Role: "purpose"},
				{Key: "place", Label: "Miesto", Type: Text},
				{Key: "date", Label: "Dátum", Type: Date, Required: true, Role: "date"},
				{Key: "made", Label: "Podpis splnomocniteľa", Type: Text, Role: "sign1"},
				{Key: "approved", Label: "Splnomocnenie prijímam", Type: Text, Role: "sign2"},
			}},
	}
}

// Find vráti tlačivo podľa kľúča.
func Find(forms []Form, key string) (Form, bool) {
	for _, f := range forms {
		if f.Key == key {
			return f, true
		}
	}
	return Form{}, false
}

// FieldError je chyba poľa.
type FieldError = invoice.FieldError

var keyRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// Normalize upraví vlastnú definíciu tlačiva: kľúče polí z názvov, papier,
// rozloženie dokumentu.
func (f *Form) Normalize() {
	f.Name, f.Description = strings.TrimSpace(f.Name), strings.TrimSpace(f.Description)
	f.Builtin, f.Cash, f.Layout = false, "", LayoutDocument
	if f.Paper != "A5" {
		f.Paper = "A4"
	}
	used := map[string]bool{}
	for i := range f.Fields {
		fl := &f.Fields[i]
		fl.Label = strings.TrimSpace(fl.Label)
		if fl.Type == "" {
			fl.Type = Text
		}
		if !keyRe.MatchString(fl.Key) || used[fl.Key] {
			base := slug(fl.Label)
			if base == "" {
				base = "pole"
			}
			fl.Key = base
			for n := 2; used[fl.Key]; n++ {
				fl.Key = fmt.Sprintf("%s-%d", base, n)
			}
		}
		used[fl.Key] = true
	}
}

// Validate overí vlastnú definíciu.
func (f Form) Validate() error {
	var fe []FieldError
	if f.Name == "" {
		fe = append(fe, FieldError{Field: "name", Message: "Zadajte názov tlačiva"})
	}
	if !keyRe.MatchString(f.Key) {
		fe = append(fe, FieldError{Field: "key", Message: "Neplatný kľúč tlačiva"})
	}
	if len(f.Fields) == 0 && !f.Items {
		fe = append(fe, FieldError{Field: "fields", Message: "Pridajte aspoň jedno pole alebo položky"})
	}
	if len(f.Fields) > 60 {
		fe = append(fe, FieldError{Field: "fields", Message: "Najviac 60 polí"})
	}
	for i, fl := range f.Fields {
		if fl.Label == "" {
			fe = append(fe, FieldError{Field: fmt.Sprintf("fields[%d].label", i), Message: "Zadajte názov poľa"})
		}
		ok := false
		for _, t := range fieldTypes {
			ok = ok || t == fl.Type
		}
		if !ok {
			fe = append(fe, FieldError{Field: fmt.Sprintf("fields[%d].type", i), Message: "Neznámy druh poľa"})
		}
	}
	if len(fe) > 0 {
		return &invoice.ValidationError{Fields: fe}
	}
	return nil
}

var slugMap = strings.NewReplacer("á", "a", "ä", "a", "č", "c", "ď", "d", "é", "e", "í", "i", "ĺ", "l", "ľ", "l", "ň", "n", "ó", "o", "ô", "o", "ŕ", "r", "š", "s", "ť", "t", "ú", "u", "ý", "y", "ž", "z")

func slug(s string) string {
	s = slugMap.Replace(strings.ToLower(strings.TrimSpace(s)))
	var b strings.Builder
	dash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if b.Len() > 0 && !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.TrimSuffix(b.String(), "-")
	if len(out) > 32 {
		out = strings.TrimSuffix(out[:32], "-")
	}
	return out
}

// Slug je kľúč z názvu (pre nové vlastné tlačivo).
func Slug(s string) string { return slug(s) }

// Compute overí hodnoty a dopočíta sumy. Chyby polí vráti spolu s tým, čo
// sa dalo vypočítať (náhľad sa ukáže aj pri neúplnom tlačive).
func Compute(f Form, v Values, items []Item) (Computed, error) {
	var c Computed
	var fe []FieldError
	c.Lines = []money.Dec{}
	for _, fl := range f.Fields {
		s := strings.TrimSpace(v[fl.Key])
		if s == "" {
			if fl.Required {
				fe = append(fe, FieldError{Field: fl.Key, Message: "Vyplňte: " + fl.Label})
			}
			continue
		}
		switch fl.Type {
		case Money, Number:
			if _, err := money.Parse(s); err != nil {
				fe = append(fe, FieldError{Field: fl.Key, Message: fl.Label + ": zadajte číslo"})
			}
		case Date:
			if _, err := invoice.ParseDate(s); err != nil {
				fe = append(fe, FieldError{Field: fl.Key, Message: fl.Label + ": neplatný dátum"})
			}
		case Rate:
			var r int
			if _, err := fmt.Sscan(s, &r); err != nil || !vat.Rate(r).Known() {
				fe = append(fe, FieldError{Field: fl.Key, Message: "Neznáma sadzba DPH"})
			}
		}
	}
	// suma s DPH a sadzba (pole s rolou amount a rate)
	var amount money.Dec
	var rate *vat.Rate
	for _, fl := range f.Fields {
		s := strings.TrimSpace(v[fl.Key])
		switch {
		case fl.Role == "amount" && s != "":
			amount, _ = money.Parse(s)
		case fl.Role == "rate" && s != "":
			var r int
			if _, err := fmt.Sscan(s, &r); err == nil && vat.Rate(r).Known() {
				rr := vat.Rate(r)
				rate = &rr
			}
		}
	}
	for i, it := range items {
		if strings.TrimSpace(it.Desc) == "" && it.Qty.IsZero() && it.Price.IsZero() {
			c.Lines = append(c.Lines, money.Zero)
			continue
		}
		if strings.TrimSpace(it.Desc) == "" {
			fe = append(fe, FieldError{Field: fmt.Sprintf("items[%d].desc", i), Message: fmt.Sprintf("Položka %d: zadajte popis", i+1)})
		}
		line := vat.LineAmount(vat.Line{Quantity: it.Qty, UnitPrice: it.Price})
		c.Lines = append(c.Lines, line)
		c.ItemsTotal = c.ItemsTotal.Add(line)
	}
	if amount.Sign() < 0 {
		fe = append(fe, FieldError{Field: "amount", Message: "Suma nemôže byť záporná"})
	} else if !amount.IsZero() {
		c.Total, c.Base = amount.RoundCents(), amount.RoundCents()
		if rate != nil && *rate > 0 {
			s, err := vat.Calculate([]vat.Line{{Quantity: money.FromInt(1), UnitPrice: amount, Rate: *rate}}, vat.PriceGross, false)
			if err == nil {
				c.Base, c.VAT, c.Total = s.Base, s.VAT, s.Total
			}
		}
		c.Rate = rate
	}
	if len(fe) > 0 {
		return c, &invoice.ValidationError{Fields: fe}
	}
	return c, nil
}

// Doc je uložené (vyplnené) tlačivo alebo vzor.
type Doc struct {
	ID        int64  `json:"id"`
	CompanyID int64  `json:"companyId"`
	FormKey   string `json:"formKey"`
	Title     string `json:"title"`
	Values    Values `json:"values"`
	Items     []Item `json:"items"`
	// Template: vzor — nové tlačivo z neho prevezme hodnoty.
	Template bool `json:"template"`
	// CashID je pokladničný doklad, ktorým bolo tlačivo zapísané do účtovníctva.
	CashID     int64        `json:"cashId,omitempty"`
	CashNumber string       `json:"cashNumber,omitempty"`
	Updated    invoice.Date `json:"updated"`
}
