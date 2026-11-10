// Package sorder: prijaté objednávky od zákazníkov. Objednávka má
// odberateľa, riadky s cenami ako faktúra a požadovaný termín dodania.
// Kým nie je vybavená, rezervuje skladové položky (zvyšok na dodanie).
// Dodáva sa po častiach dodacími listami alebo faktúrami; každé dodanie
// si objednávka pamätá s množstvami po riadkoch, aby sa dalo vrátiť,
// keď sa doklad zmaže alebo stornuje.
package sorder

import (
	"strconv"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

// Status je stav objednávky.
type Status string

const (
	Open      Status = "open"      // prijatá, nič nedodané
	Partial   Status = "partial"   // čiastočne dodaná
	Done      Status = "done"      // vybavená (dodané všetko)
	Cancelled Status = "cancelled" // zrušená
)

// Active: objednávka čaká na dodanie a rezervuje tovar.
func (s Status) Active() bool { return s == Open || s == Partial }

// Line je riadok objednávky: riadok ako na faktúre a dodané množstvo.
type Line struct {
	invoice.Line
	// Key je stály kľúč riadku (dodania sa naň odvolávajú aj po úprave).
	Key       int       `json:"key"`
	Delivered money.Dec `json:"delivered"` // dodané (vypočítané z dodaní)
	Rest      money.Dec `json:"remaining"` // zostáva dodať (vypočítané)
	// Available: koľko je na sklade po odpočítaní rezervácií iných
	// objednávok (dopĺňa aplikácia pri skladovej položke).
	Available *money.Dec `json:"available,omitempty"`
	// Short: koľko zo zvyšku na dodanie na sklade chýba (pri Available).
	Short *money.Dec `json:"short,omitempty"`
}

// Remaining vráti ešte nedodané množstvo riadku.
func (l Line) Remaining() money.Dec {
	r := l.Quantity.Sub(l.Delivered)
	if r.Sign() < 0 {
		return money.Zero
	}
	return r
}

// Fulfil je jedno dodanie: dodací list alebo faktúra a množstvá podľa
// kľúča riadku.
type Fulfil struct {
	Kind   string            `json:"kind"` // "delivery" alebo "invoice"
	ID     int64             `json:"id"`
	Number string            `json:"number"` // dopĺňa aplikácia
	Qty    map[int]money.Dec `json:"qty"`
}

// Druhy dokladov dodania.
const (
	ByDelivery = "delivery"
	ByInvoice  = "invoice"
)

// Order je prijatá objednávka.
type Order struct {
	ID          int64         `json:"id"`
	CompanyID   int64         `json:"companyId"`
	Number      string        `json:"number"`
	Status      Status        `json:"status"`
	Customer    invoice.Party `json:"customer"`
	ContactID   int64         `json:"contactId"`
	Date        invoice.Date  `json:"date"`  // prijatá dňa
	Due         invoice.Date  `json:"due"`   // požadované dodanie
	CustomerRef string        `json:"ref"`   // číslo objednávky u zákazníka
	Place       string        `json:"place"` // miesto dodania
	Note        string        `json:"note"`  // poznámka
	PriceMode   vat.PriceMode `json:"priceMode"`
	NonVATPayer bool          `json:"nonVatPayer"`
	Lines       []Line        `json:"lines"`
	Summary     vat.Summary   `json:"summary"`
	Fulfils     []Fulfil      `json:"fulfils"`
}

// FieldError je chyba poľa (rovnaký tvar ako pri faktúre).
type FieldError = invoice.FieldError

func lineField(i int, f string) string { return "lines[" + strconv.Itoa(i) + "]." + f }

// Recalculate oreže texty, pridelí kľúče novým riadkom, prepočíta sumy,
// dodané množstvá a stav.
func (o *Order) Recalculate() error {
	o.CustomerRef, o.Place, o.Note = strings.TrimSpace(o.CustomerRef), strings.TrimSpace(o.Place), strings.TrimSpace(o.Note)
	if o.Status == "" {
		o.Status = Open
	}
	next := 0
	for _, l := range o.Lines {
		if l.Key > next {
			next = l.Key
		}
	}
	seen := map[int]bool{}
	for i := range o.Lines {
		if o.Lines[i].Key <= 0 || seen[o.Lines[i].Key] {
			next++
			o.Lines[i].Key = next
		}
		seen[o.Lines[i].Key] = true
	}
	inv := invoice.Invoice{Kind: invoice.KindInvoice, Customer: o.Customer, ContactID: o.ContactID, IssueDate: o.Date, DeliveryDate: o.Date,
		DueDate: o.Date, Currency: "EUR", PriceMode: o.PriceMode, NonVATPayer: o.NonVATPayer}
	for _, l := range o.Lines {
		inv.Lines = append(inv.Lines, l.Line)
	}
	if err := inv.Recalculate(); err != nil {
		return err
	}
	for i := range o.Lines {
		o.Lines[i].Line = inv.Lines[i]
	}
	o.Summary, o.PriceMode = inv.Summary, inv.PriceMode
	o.UpdateDelivered()
	return nil
}

// UpdateDelivered spočíta dodané množstvá z dodaní a nastaví stav.
func (o *Order) UpdateDelivered() {
	got := map[int]money.Dec{}
	for _, f := range o.Fulfils {
		for k, q := range f.Qty {
			got[k] = got[k].Add(q)
		}
	}
	any, all := false, len(o.Lines) > 0
	for i := range o.Lines {
		l := &o.Lines[i]
		l.Delivered = got[l.Key]
		l.Rest = l.Remaining()
		if l.Delivered.Sign() > 0 {
			any = true
		}
		if l.Rest.Sign() > 0 {
			all = false
		}
	}
	if o.Status == Cancelled {
		return
	}
	switch {
	case all:
		o.Status = Done
	case any:
		o.Status = Partial
	default:
		o.Status = Open
	}
}

// Validate overí objednávku. Riadok s dodaným množstvom sa nedá odstrániť
// ani znížiť pod dodané.
func (o *Order) Validate() []FieldError {
	var out []FieldError
	if strings.TrimSpace(o.Customer.Name) == "" {
		out = append(out, FieldError{Field: "customer.name", Message: "Zadajte odberateľa"})
	}
	if o.Date.IsZero() {
		out = append(out, FieldError{Field: "date", Message: "Zadajte dátum objednávky"})
	}
	if !o.Due.IsZero() && o.Due.Before(o.Date) {
		out = append(out, FieldError{Field: "due", Message: "Dodanie nemôže byť pred dátumom objednávky"})
	}
	if len(o.Lines) == 0 {
		out = append(out, FieldError{Field: "lines", Message: "Pridajte aspoň jednu položku"})
	}
	keys := map[int]bool{}
	for i, l := range o.Lines {
		keys[l.Key] = true
		if strings.TrimSpace(l.Description) == "" {
			out = append(out, FieldError{Field: lineField(i, "description"), Message: "Zadajte popis položky"})
		}
		switch {
		case l.Quantity.Sign() <= 0:
			out = append(out, FieldError{Field: lineField(i, "quantity"), Message: "Množstvo musí byť kladné"})
		case l.Delivered.Cmp(l.Quantity) > 0:
			out = append(out, FieldError{Field: lineField(i, "quantity"), Message: "Menej, ako je už dodané (" + l.Delivered.String() + ")"})
		}
		if l.UnitPrice.Sign() < 0 {
			out = append(out, FieldError{Field: lineField(i, "unitPrice"), Message: "Cena nemôže byť záporná"})
		}
	}
	for _, f := range o.Fulfils {
		for k, q := range f.Qty {
			if !keys[k] && q.Sign() > 0 {
				out = append(out, FieldError{Field: "lines", Message: "Dodanú položku nemožno odstrániť (doklad " + f.Number + ")"})
				return out
			}
		}
	}
	return out
}

// Editable: obsah možno meniť, kým objednávka nie je vybavená ani zrušená.
func (o *Order) Editable() bool { return o.Status.Active() }

// Pick vyberie množstvá na dodanie (kľúč riadku → množstvo) a vráti riadky
// dokladu. Bez výberu sa dodá celý zvyšok.
func (o *Order) Pick(qty map[int]money.Dec) ([]invoice.Line, map[int]money.Dec, []FieldError) {
	if !o.Status.Active() {
		return nil, nil, []FieldError{{Field: "status", Message: "Objednávka je vybavená alebo zrušená"}}
	}
	if len(qty) == 0 {
		qty = map[int]money.Dec{}
		for _, l := range o.Lines {
			qty[l.Key] = l.Remaining()
		}
	}
	var lines []invoice.Line
	used := map[int]money.Dec{}
	var errs []FieldError
	for i, l := range o.Lines {
		q, ok := qty[l.Key]
		if !ok || q.IsZero() {
			continue
		}
		f := lineField(i, "pick")
		switch {
		case q.Sign() < 0:
			errs = append(errs, FieldError{Field: f, Message: "Množstvo musí byť kladné"})
		case q.Cmp(l.Remaining()) > 0:
			errs = append(errs, FieldError{Field: f, Message: "Najviac " + l.Remaining().String() + " " + l.Unit})
		default:
			x := l.Line
			x.Quantity = q
			lines = append(lines, x)
			used[l.Key] = q
		}
	}
	if len(errs) > 0 {
		return nil, nil, errs
	}
	if len(lines) == 0 {
		return nil, nil, []FieldError{{Field: "lines", Message: "Zadajte množstvo na dodanie"}}
	}
	return lines, used, nil
}

// Reserved vráti rezervované množstvá skladových položiek (zvyšky na
// dodanie aktívnych objednávok), okrem objednávky skip.
func Reserved(orders []Order, skip int64) map[int64]money.Dec {
	out := map[int64]money.Dec{}
	for _, o := range orders {
		if o.ID == skip || !o.Status.Active() {
			continue
		}
		for _, l := range o.Lines {
			if l.ItemID != 0 && l.Remaining().Sign() > 0 {
				out[l.ItemID] = out[l.ItemID].Add(l.Remaining())
			}
		}
	}
	return out
}
