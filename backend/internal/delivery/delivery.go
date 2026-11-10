// Package delivery: dodací list. Dodací list potvrdzuje odovzdanie tovaru
// alebo prác zákazníkovi. Nie je daňový doklad; ceny na ňom slúžia na
// neskoršiu faktúru (jednu alebo zbernú z viacerých dodacích listov).
// Vydaním sa skladové položky vydajú zo skladu.
package delivery

import (
	"strconv"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

// Status je stav dodacieho listu.
type Status string

const (
	Draft     Status = "draft"     // koncept bez čísla
	Issued    Status = "issued"    // vydaný (tovar odovzdaný, sklad odpísaný)
	Invoiced  Status = "invoiced"  // vyfakturovaný
	Cancelled Status = "cancelled" // stornovaný (sklad vrátený)
)

// Note je dodací list.
type Note struct {
	ID        int64         `json:"id"`
	CompanyID int64         `json:"companyId"`
	Number    string        `json:"number"`
	Status    Status        `json:"status"`
	Customer  invoice.Party `json:"customer"`
	ContactID int64         `json:"contactId"`
	Date      invoice.Date  `json:"date"`
	Place     string        `json:"place"`    // miesto dodania
	Order     string        `json:"order"`    // číslo objednávky zákazníka
	Note      string        `json:"note"`     // text pod položkami
	Receiver  string        `json:"receiver"` // kto prevzal
	PriceMode vat.PriceMode `json:"priceMode"`
	// NonVATPayer: dodávateľ nie je platiteľ DPH (dopĺňa aplikácia z firmy).
	NonVATPayer bool           `json:"nonVatPayer"`
	Lines       []invoice.Line `json:"lines"`
	Summary     vat.Summary    `json:"summary"`
	// InvoiceID: faktúra z dodacieho listu; InvoiceNumber dopĺňa aplikácia.
	InvoiceID     int64  `json:"invoiceId,omitempty"`
	InvoiceNumber string `json:"invoiceNumber,omitempty"`
}

// FieldError je chyba poľa (rovnaký tvar ako pri faktúre).
type FieldError = invoice.FieldError

// asInvoice: dodací list ako faktúra kvôli spoločnému výpočtu súm.
func (n *Note) asInvoice() invoice.Invoice {
	return invoice.Invoice{Kind: invoice.KindInvoice, Customer: n.Customer, ContactID: n.ContactID, IssueDate: n.Date, DeliveryDate: n.Date,
		DueDate: n.Date, Currency: "EUR", PriceMode: n.PriceMode, NonVATPayer: n.NonVATPayer, Lines: append([]invoice.Line(nil), n.Lines...)}
}

// Recalculate oreže texty a prepočíta sumy riadkov a rekapituláciu.
func (n *Note) Recalculate() error {
	n.Place, n.Order, n.Note, n.Receiver = strings.TrimSpace(n.Place), strings.TrimSpace(n.Order), strings.TrimSpace(n.Note), strings.TrimSpace(n.Receiver)
	if n.Status == "" {
		n.Status = Draft
	}
	inv := n.asInvoice()
	if err := inv.Recalculate(); err != nil {
		return err
	}
	n.Lines, n.Summary, n.PriceMode = inv.Lines, inv.Summary, inv.PriceMode
	return nil
}

// Validate overí dodací list pred vydaním.
func (n *Note) Validate() []FieldError {
	var out []FieldError
	if strings.TrimSpace(n.Customer.Name) == "" {
		out = append(out, FieldError{Field: "customer.name", Message: "Zadajte odberateľa"})
	}
	if n.Date.IsZero() {
		out = append(out, FieldError{Field: "date", Message: "Zadajte dátum dodania"})
	}
	if len(n.Lines) == 0 {
		out = append(out, FieldError{Field: "lines", Message: "Pridajte aspoň jednu položku"})
	}
	for i, l := range n.Lines {
		if strings.TrimSpace(l.Description) == "" {
			out = append(out, FieldError{Field: lineField(i, "description"), Message: "Zadajte popis položky"})
		}
		if l.Quantity.Sign() <= 0 {
			out = append(out, FieldError{Field: lineField(i, "quantity"), Message: "Množstvo musí byť kladné"})
		}
		if l.UnitPrice.Sign() < 0 {
			out = append(out, FieldError{Field: lineField(i, "unitPrice"), Message: "Cena nemôže byť záporná"})
		}
	}
	return out
}

func lineField(i int, f string) string {
	return "lines[" + strconv.Itoa(i) + "]." + f
}

// Editable: obsah možno meniť (koncept a vydaný, ešte nevyfakturovaný).
func (n *Note) Editable() bool { return n.Status == Draft || n.Status == Issued }

// Invoiceable: vydaný a ešte nevyfakturovaný.
func (n *Note) Invoiceable() bool { return n.Status == Issued }
