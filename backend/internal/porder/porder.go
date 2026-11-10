// Package porder: objednávky dodávateľom (vydané objednávky). Objednávka
// má dodávateľa, riadky s položkami cenníka alebo voľným textom, nákupné
// ceny bez DPH a stav. Príjem na sklad sa zapisuje k riadkom (aj čiastočne)
// a z prijatých množstiev vzniká príjemka.
package porder

import (
	"fmt"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
)

// Status je stav objednávky.
type Status string

const (
	Draft     Status = "draft"     // rozpracovaná
	Sent      Status = "sent"      // odoslaná dodávateľovi
	Partial   Status = "partial"   // čiastočne prijatá
	Received  Status = "received"  // prijatá celá
	Cancelled Status = "cancelled" // zrušená
)

// Open: objednávka čaká na dodanie (rozpracovaná, odoslaná, čiastočne prijatá).
func (s Status) Open() bool { return s == Draft || s == Sent || s == Partial }

// Line je riadok objednávky.
type Line struct {
	ItemID       int64     `json:"itemId,omitempty"`
	Name         string    `json:"name"`
	SupplierCode string    `json:"supplierCode"` // kód u dodávateľa
	Unit         string    `json:"unit"`
	Quantity     money.Dec `json:"quantity"`
	UnitPrice    money.Dec `json:"unitPrice"` // nákupná cena bez DPH
	Total        money.Dec `json:"total"`     // vypočítané
	Received     money.Dec `json:"received"`  // už prijaté množstvo
	Rest         money.Dec `json:"remaining"` // ešte neprijaté (vypočítané)
}

// Order je objednávka dodávateľovi.
type Order struct {
	ID        int64        `json:"id"`
	CompanyID int64        `json:"companyId"`
	Number    string       `json:"number"`
	Status    Status       `json:"status"`
	Date      invoice.Date `json:"date"`
	Expected  invoice.Date `json:"expected"` // očakávané dodanie
	ContactID int64        `json:"contactId"`
	Supplier  string       `json:"supplier"` // názov dodávateľa
	Note      string       `json:"note"`
	Lines     []Line       `json:"lines"`
	Total     money.Dec    `json:"total"` // bez DPH (vypočítané)
	Sent      invoice.Date `json:"sentDate"`
}

// FieldError je chyba poľa.
type FieldError struct {
	Field   string
	Message string
}

// Normalize oreže texty, doplní jednotky a spočíta sumy.
func (o *Order) Normalize() {
	o.Supplier, o.Note = strings.TrimSpace(o.Supplier), strings.TrimSpace(o.Note)
	if o.Status == "" {
		o.Status = Draft
	}
	o.Total = money.Zero
	for i := range o.Lines {
		l := &o.Lines[i]
		l.Name, l.SupplierCode, l.Unit = strings.TrimSpace(l.Name), strings.TrimSpace(l.SupplierCode), strings.TrimSpace(l.Unit)
		if l.Unit == "" {
			l.Unit = "ks"
		}
		l.Total = l.Quantity.Mul(l.UnitPrice).RoundCents()
		l.Rest = l.Remaining()
		o.Total = o.Total.Add(l.Total)
	}
}

// Validate overí objednávku.
func (o *Order) Validate() []FieldError {
	var out []FieldError
	if o.Supplier == "" {
		out = append(out, FieldError{"supplier", "Zadajte dodávateľa"})
	}
	if o.Date.IsZero() {
		out = append(out, FieldError{"date", "Zadajte dátum objednávky"})
	}
	if !o.Expected.IsZero() && o.Expected.Before(o.Date) {
		out = append(out, FieldError{"expected", "Dodanie nemôže byť pred dátumom objednávky"})
	}
	if len(o.Lines) == 0 {
		out = append(out, FieldError{"lines", "Pridajte aspoň jednu položku"})
	}
	for i, l := range o.Lines {
		f := func(s string) string { return fmt.Sprintf("lines[%d].%s", i, s) }
		if l.Name == "" {
			out = append(out, FieldError{f("name"), "Zadajte názov položky"})
		}
		if l.Quantity.Sign() <= 0 {
			out = append(out, FieldError{f("quantity"), "Zadajte množstvo"})
		} else if l.Received.Cmp(l.Quantity) > 0 {
			out = append(out, FieldError{f("quantity"), "Menej, ako je už prijaté (" + l.Received.String() + ")"})
		}
		if l.UnitPrice.Sign() < 0 {
			out = append(out, FieldError{f("unitPrice"), "Cena nemôže byť záporná"})
		}
	}
	return out
}

// Editable: obsah objednávky možno meniť, kým nie je prijatá celá ani zrušená.
func (o *Order) Editable() bool { return o.Status.Open() }

// Remaining vráti ešte neprijaté množstvo riadku.
func (l Line) Remaining() money.Dec {
	r := l.Quantity.Sub(l.Received)
	if r.Sign() < 0 {
		return money.Zero
	}
	return r
}

// UpdateStatus nastaví stav podľa prijatých množstiev (odoslaná ostane
// odoslaná, kým nič neprišlo).
func (o *Order) UpdateStatus() {
	if o.Status == Cancelled {
		return
	}
	any, all := false, len(o.Lines) > 0
	for i := range o.Lines {
		l := &o.Lines[i]
		l.Rest = l.Remaining()
		if l.Received.Sign() > 0 {
			any = true
		}
		if l.Remaining().Sign() > 0 {
			all = false
		}
	}
	switch {
	case all:
		o.Status = Received
	case any:
		o.Status = Partial
	case o.Status == Partial || o.Status == Received:
		o.Status = Sent
	}
}

// ErrState je chyba prechodu stavu.
type ErrState string

func (e ErrState) Error() string { return string(e) }

// Receive zapíše príjem množstiev (index riadku → množstvo). Vráti chybu,
// ak objednávka nie je otvorená alebo by sa prijalo viac, ako sa objednalo.
func (o *Order) Receive(qty map[int]money.Dec) []FieldError {
	if !o.Status.Open() {
		return []FieldError{{"status", "Objednávka je uzavretá"}}
	}
	var out []FieldError
	n := 0
	for i, q := range qty {
		if q.IsZero() {
			continue
		}
		if i < 0 || i >= len(o.Lines) {
			out = append(out, FieldError{"lines", "Neznámy riadok objednávky"})
			continue
		}
		f := fmt.Sprintf("lines[%d].receive", i)
		l := &o.Lines[i]
		switch {
		case q.Sign() < 0:
			out = append(out, FieldError{f, "Množstvo musí byť kladné"})
		case q.Cmp(l.Remaining()) > 0:
			out = append(out, FieldError{f, "Najviac " + l.Remaining().String() + " " + l.Unit})
		default:
			l.Received = l.Received.Add(q)
			n++
		}
	}
	if len(out) == 0 && n == 0 {
		out = append(out, FieldError{"lines", "Zadajte prijaté množstvo"})
	}
	if len(out) > 0 {
		return out
	}
	if o.Status == Draft {
		o.Status = Sent
	}
	o.UpdateStatus()
	return nil
}
