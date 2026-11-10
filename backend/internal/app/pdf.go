package app

import (
	"context"
	"errors"

	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/porder"
	"github.com/fiskus-online/fiskus/backend/internal/printout"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// Document je hotový doklad v PDF (na stiahnutie alebo do prílohy e-mailu).
type Document struct {
	Name string // názov súboru, napr. „Faktura-2026001.pdf“
	Data []byte
	Type string // MIME typ; prázdny = application/pdf
}

// supplier vráti údaje dodávateľa pre tlač: firmu a logo (ak je nahrané).
func (a *App) supplier(ctx context.Context, companyID int64) (printout.Supplier, error) {
	co, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return printout.Supplier{}, err
	}
	sup := printout.Supplier{Company: co}
	if l, err := a.Store.GetLogo(ctx, companyID); err == nil {
		sup.Logo = l.Data
	} else if !errors.Is(err, store.ErrNotFound) {
		return printout.Supplier{}, err
	}
	return sup, nil
}

// fileKind vráti druh dokladu do názvu súboru (bez diakritiky).
func fileKind(k invoice.Kind) string {
	switch k {
	case invoice.KindCreditNote:
		return "Dobropis"
	case invoice.KindProforma:
		return "Zalohova-faktura"
	case invoice.KindAdvanceReceipt:
		return "Doklad-k-platbe"
	}
	return "Faktura"
}

// InvoicePDF vytvorí PDF faktúry.
func (a *App) InvoicePDF(ctx context.Context, companyID, id int64) (Document, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return Document{}, err
	}
	inv, err := a.Store.GetInvoice(ctx, companyID, id)
	if err != nil {
		return Document{}, err
	}
	sup, err := a.supplier(ctx, companyID)
	if err != nil {
		return Document{}, err
	}
	data, err := printout.Invoice(inv, sup)
	if err != nil {
		return Document{}, err
	}
	return Document{Name: printout.FileName(fileKind(inv.Kind), inv.Number), Data: data}, nil
}

// QuotePDF vytvorí PDF cenovej ponuky.
func (a *App) QuotePDF(ctx context.Context, companyID, id int64) (Document, error) {
	q, err := a.Quote(ctx, companyID, id)
	if err != nil {
		return Document{}, err
	}
	sup, err := a.supplier(ctx, companyID)
	if err != nil {
		return Document{}, err
	}
	data, err := printout.Quote(q, sup)
	if err != nil {
		return Document{}, err
	}
	return Document{Name: printout.FileName("Ponuka", q.Number), Data: data}, nil
}

// ReminderPDF vytvorí PDF upomienky.
func (a *App) ReminderPDF(ctx context.Context, companyID, id int64) (Document, error) {
	r, err := a.Reminder(ctx, companyID, id)
	if err != nil {
		return Document{}, err
	}
	sup, err := a.supplier(ctx, companyID)
	if err != nil {
		return Document{}, err
	}
	data, err := printout.Reminder(r, sup)
	if err != nil {
		return Document{}, err
	}
	return Document{Name: printout.FileName("Upomienka", r.Number), Data: data}, nil
}

// DeliveryPDF vytvorí PDF dodacieho listu (s cenami alebo bez nich).
func (a *App) DeliveryPDF(ctx context.Context, companyID, id int64, prices bool) (Document, error) {
	n, err := a.Delivery(ctx, companyID, id)
	if err != nil {
		return Document{}, err
	}
	sup, err := a.supplier(ctx, companyID)
	if err != nil {
		return Document{}, err
	}
	data, err := printout.Delivery(n, prices, sup)
	if err != nil {
		return Document{}, err
	}
	return Document{Name: printout.FileName("Dodaci-list", n.Number), Data: data}, nil
}

// porderParty vráti dodávateľa objednávky z adresára (adresa, IČO, e-mail),
// inak iba názov z objednávky.
func (a *App) porderParty(ctx context.Context, companyID int64, o porder.Order) invoice.Party {
	if o.ContactID != 0 {
		if c, err := a.Store.GetContact(ctx, companyID, o.ContactID); err == nil {
			return PartyFromContact(c)
		}
	}
	return invoice.Party{Name: o.Supplier}
}

// POrderPDF vytvorí PDF objednávky dodávateľovi.
func (a *App) POrderPDF(ctx context.Context, companyID, id int64) (Document, error) {
	o, err := a.PurchaseOrder(ctx, companyID, id)
	if err != nil {
		return Document{}, err
	}
	sup, err := a.supplier(ctx, companyID)
	if err != nil {
		return Document{}, err
	}
	data, err := printout.POrder(o, a.porderParty(ctx, companyID, o), sup)
	if err != nil {
		return Document{}, err
	}
	return Document{Name: printout.FileName("Objednavka", o.Number), Data: data}, nil
}
