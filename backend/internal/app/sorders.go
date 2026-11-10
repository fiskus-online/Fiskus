package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/delivery"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/link"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/quote"
	"github.com/fiskus-online/fiskus/backend/internal/sorder"
	"github.com/fiskus-online/fiskus/backend/internal/stock"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// Prijaté objednávky: prijatá → čiastočne dodaná → vybavená, alebo
// zrušená. Číslo (OP20260001) dostane hneď pri zápise. Kým je otvorená,
// zvyšok skladových položiek na dodanie je rezervovaný: odpočíta sa od
// disponibilného stavu a započíta do návrhu na objednanie. Dodáva sa
// konceptom dodacieho listu alebo faktúry s vybranými množstvami;
// zmazaním či stornom dokladu sa množstvá vrátia na dodanie.

const sorderSeries = "OP"

func sorderErr(errs []sorder.FieldError) error {
	if len(errs) == 0 {
		return nil
	}
	return &invoice.ValidationError{Fields: errs}
}

// refreshSalesOrder vynechá dodania, ktorých doklad už neplatí (zmazaný,
// stornovaný), doplní ich čísla a prepočíta stav. Vráti true pri zmene.
func (a *App) refreshSalesOrder(ctx context.Context, companyID int64, o *sorder.Order) bool {
	changed := false
	keep := o.Fulfils[:0:0]
	for _, f := range o.Fulfils {
		num, ok := "", true
		switch f.Kind {
		case sorder.ByDelivery:
			n, err := a.Store.GetDelivery(ctx, companyID, f.ID)
			if errors.Is(err, store.ErrNotFound) || (err == nil && n.Status == delivery.Cancelled) {
				ok = false
			} else if err == nil {
				num = firstNonBlank(n.Number, "koncept")
			}
		case sorder.ByInvoice:
			inv, err := a.Store.GetInvoice(ctx, companyID, f.ID)
			if errors.Is(err, store.ErrNotFound) || (err == nil && inv.Status == invoice.StatusCancelled) {
				ok = false
			} else if err == nil {
				num = firstNonBlank(inv.Number, "koncept")
			}
		}
		if !ok {
			changed = true
			continue
		}
		if num != "" && num != f.Number {
			f.Number, changed = num, true
		}
		keep = append(keep, f)
	}
	o.Fulfils = keep
	before := o.Status
	o.UpdateDelivered()
	return changed || before != o.Status
}

// salesOrders vráti objednávky so zosúladenými dodaniami (bez kontroly práv).
func (a *App) salesOrders(ctx context.Context, companyID int64) ([]sorder.Order, error) {
	list, err := a.Store.ListSalesOrders(ctx, companyID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if a.refreshSalesOrder(ctx, companyID, &list[i]) {
			if err := a.Store.SaveSalesOrder(ctx, companyID, &list[i]); err != nil {
				return nil, err
			}
		}
	}
	return list, nil
}

// reservedStock vráti rezervované množstvá skladových položiek okrem
// objednávky skip.
func (a *App) reservedStock(ctx context.Context, companyID, skip int64) (map[int64]money.Dec, error) {
	list, err := a.salesOrders(ctx, companyID)
	if err != nil {
		return nil, err
	}
	return sorder.Reserved(list, skip), nil
}

// SalesOrders vráti prijaté objednávky, najnovšie prvé.
func (a *App) SalesOrders(ctx context.Context, companyID int64) ([]sorder.Order, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return nil, err
	}
	return a.salesOrders(ctx, companyID)
}

// SalesOrder vráti objednávku; pri skladových položkách aktívnej
// objednávky doplní, koľko je na sklade po rezerváciách iných objednávok.
func (a *App) SalesOrder(ctx context.Context, companyID, id int64) (sorder.Order, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return sorder.Order{}, err
	}
	o, err := a.Store.GetSalesOrder(ctx, companyID, id)
	if err != nil {
		return o, err
	}
	if a.refreshSalesOrder(ctx, companyID, &o) {
		if err := a.Store.SaveSalesOrder(ctx, companyID, &o); err != nil {
			return o, err
		}
	}
	return o, a.fillAvailable(ctx, companyID, &o)
}

// fillAvailable doplní riadkom disponibilné množstvo na sklade.
func (a *App) fillAvailable(ctx context.Context, companyID int64, o *sorder.Order) error {
	if !o.Status.Active() || a.Require(ctx, companyID, auth.PermPurchasesRead) != nil {
		return nil
	}
	cfg, err := a.loadStock(ctx, companyID)
	if err != nil || len(cfg.Items) == 0 {
		return err
	}
	docs, err := a.Store.ListStockDocs(ctx, companyID)
	if err != nil {
		return err
	}
	cards, _, _ := stock.Valuate(docs, cfg.Method)
	have := map[int64]money.Dec{}
	for k, c := range cards {
		have[k.ItemID] = have[k.ItemID].Add(c.Quantity)
	}
	reserved, err := a.reservedStock(ctx, companyID, o.ID)
	if err != nil {
		return err
	}
	for i := range o.Lines {
		l := &o.Lines[i]
		if _, ok := cfg.item(l.ItemID); l.ItemID != 0 && ok {
			v := have[l.ItemID].Sub(reserved[l.ItemID])
			short := l.Rest.Sub(v)
			if short.Sign() < 0 {
				short = money.Zero
			}
			l.Available, l.Short = &v, &short
		}
	}
	return nil
}

// prepareSalesOrder doplní odberateľa z adresára, platiteľa DPH a prepočíta sumy.
func (a *App) prepareSalesOrder(ctx context.Context, companyID int64, o *sorder.Order) error {
	c, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return err
	}
	o.NonVATPayer = !c.VATPayer
	if o.ContactID != 0 && strings.TrimSpace(o.Customer.Name) == "" {
		ct, err := a.Store.GetContact(ctx, companyID, o.ContactID)
		if err != nil {
			return sorderErr([]sorder.FieldError{{Field: "contactId", Message: "Odberateľ sa v adresári nenašiel"}})
		}
		o.Customer = PartyFromContact(ct)
	}
	if o.Customer.Country == "" {
		o.Customer.Country = "SK"
	}
	for i := range o.Lines {
		o.Lines[i].Available, o.Lines[i].Short = nil, nil
	}
	if err := o.Recalculate(); err != nil {
		return err
	}
	return sorderErr(o.Validate())
}

// SaveSalesOrder zapíše novú objednávku (pridelí číslo) alebo upraví
// aktívnu; dodania a stav ostávajú.
func (a *App) SaveSalesOrder(ctx context.Context, companyID int64, o sorder.Order) (sorder.Order, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return o, err
	}
	var before *sorder.Order
	if o.ID != 0 {
		old, err := a.Store.GetSalesOrder(ctx, companyID, o.ID)
		if err != nil {
			return o, err
		}
		a.refreshSalesOrder(ctx, companyID, &old)
		if !old.Editable() {
			return o, sorderErr([]sorder.FieldError{{Field: "status", Message: "Vybavená alebo zrušená objednávka sa nemení"}})
		}
		before = &old
		o.Number, o.Status, o.Fulfils = old.Number, old.Status, old.Fulfils
	} else {
		o.Number, o.Status, o.Fulfils = "", sorder.Open, nil
		if o.Date.IsZero() {
			o.Date = a.Today()
		}
	}
	if err := a.prepareSalesOrder(ctx, companyID, &o); err != nil {
		return o, err
	}
	if o.ID == 0 {
		seq, err := a.Store.NextSequence(ctx, companyID, sorderSeries, o.Date.Year())
		if err != nil {
			return o, err
		}
		o.Number = fmt.Sprintf("%s%04d%04d", sorderSeries, o.Date.Year(), seq)
	}
	if err := a.Store.SaveSalesOrder(ctx, companyID, &o); err != nil {
		return o, err
	}
	action, what := "create", "Prijatá objednávka "+o.Number+" od "
	if before != nil {
		action, what = "update", "Úprava objednávky "+o.Number+" od "
	}
	a.record(ctx, companyID, audit.CatInvoices, action, "sales_order", o.ID, what+o.Customer.Name, before, o)
	return o, a.fillAvailable(ctx, companyID, &o)
}

// salesOrderTransition zmení stav objednávky a zapíše ho do auditu.
func (a *App) salesOrderTransition(ctx context.Context, companyID, id int64, action, what string, fn func(*sorder.Order) error) (sorder.Order, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return sorder.Order{}, err
	}
	o, err := a.Store.GetSalesOrder(ctx, companyID, id)
	if err != nil {
		return o, err
	}
	a.refreshSalesOrder(ctx, companyID, &o)
	before := o.Status
	if err := fn(&o); err != nil {
		return o, err
	}
	if err := a.Store.SaveSalesOrder(ctx, companyID, &o); err != nil {
		return o, err
	}
	type state struct {
		Status sorder.Status `json:"status"`
	}
	a.record(ctx, companyID, audit.CatInvoices, action, "sales_order", o.ID, what+" "+o.Number+" od "+o.Customer.Name, state{before}, state{o.Status})
	return o, a.fillAvailable(ctx, companyID, &o)
}

// CancelSalesOrder zruší aktívnu objednávku (rezervácia sa uvoľní).
func (a *App) CancelSalesOrder(ctx context.Context, companyID, id int64) (sorder.Order, error) {
	return a.salesOrderTransition(ctx, companyID, id, "cancel", "Zrušená objednávka", func(o *sorder.Order) error {
		if !o.Status.Active() {
			return sorderErr([]sorder.FieldError{{Field: "status", Message: "Zrušiť možno iba objednávku, ktorá čaká na dodanie"}})
		}
		o.Status = sorder.Cancelled
		return nil
	})
}

// ReopenSalesOrder obnoví zrušenú objednávku.
func (a *App) ReopenSalesOrder(ctx context.Context, companyID, id int64) (sorder.Order, error) {
	return a.salesOrderTransition(ctx, companyID, id, "reopen", "Obnovená objednávka", func(o *sorder.Order) error {
		if o.Status != sorder.Cancelled {
			return sorderErr([]sorder.FieldError{{Field: "status", Message: "Obnoviť možno iba zrušenú objednávku"}})
		}
		o.Status = sorder.Open
		o.UpdateDelivered()
		return nil
	})
}

// DeleteSalesOrder zmaže objednávku bez dodaní a jej prepojenia.
func (a *App) DeleteSalesOrder(ctx context.Context, companyID, id int64) error {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return err
	}
	o, err := a.Store.GetSalesOrder(ctx, companyID, id)
	if err != nil {
		return err
	}
	a.refreshSalesOrder(ctx, companyID, &o)
	if len(o.Fulfils) > 0 {
		return sorderErr([]sorder.FieldError{{Field: "status", Message: "Objednávka s dodacím listom alebo faktúrou sa nemaže, iba zruší"}})
	}
	links, err := a.Store.ListDocLinks(ctx, companyID, link.Ref{Kind: link.SOrder, ID: id})
	if err != nil {
		return err
	}
	for _, l := range links {
		if err := a.Store.DeleteDocLink(ctx, companyID, l.ID); err != nil {
			return err
		}
	}
	if err := a.Store.DeleteSalesOrder(ctx, companyID, id); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatInvoices, "delete", "sales_order", id, "Zmazaná objednávka "+o.Number+" od "+o.Customer.Name, o, nil)
	return nil
}

// SalesOrderFulfil je požiadavka na dodanie: doklad (dodací list alebo
// faktúra) a množstvá podľa kľúča riadku (prázdne = celý zvyšok).
type SalesOrderFulfil struct {
	To  string            `json:"to"`
	Qty map[int]money.Dec `json:"qty"`
}

// SalesOrderDoc je doklad vytvorený z objednávky.
type SalesOrderDoc struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id"`
}

// FulfilSalesOrder vytvorí z objednávky koncept dodacieho listu alebo
// faktúry s vybranými množstvami, zapíše dodanie a prepojí doklady.
func (a *App) FulfilSalesOrder(ctx context.Context, companyID, id int64, req SalesOrderFulfil) (SalesOrderDoc, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return SalesOrderDoc{}, err
	}
	o, err := a.Store.GetSalesOrder(ctx, companyID, id)
	if err != nil {
		return SalesOrderDoc{}, err
	}
	a.refreshSalesOrder(ctx, companyID, &o)
	lines, used, errs := o.Pick(req.Qty)
	if len(errs) > 0 {
		return SalesOrderDoc{}, sorderErr(errs)
	}
	ref := firstNonBlank(o.CustomerRef, o.Number)
	var doc SalesOrderDoc
	var num string
	switch req.To {
	case sorder.ByDelivery:
		n := delivery.Note{Customer: o.Customer, ContactID: o.ContactID, Date: a.Today(), Place: o.Place, Order: ref, PriceMode: o.PriceMode,
			Lines: lines, Note: "Podľa objednávky " + o.Number + "."}
		n, err := a.SaveDelivery(ctx, companyID, n)
		if err != nil {
			return doc, err
		}
		doc, num = SalesOrderDoc{Kind: sorder.ByDelivery, ID: n.ID}, "koncept"
	case sorder.ByInvoice:
		inv := invoice.Invoice{Kind: invoice.KindInvoice, Customer: o.Customer, ContactID: o.ContactID, IssueDate: a.Today(), PriceMode: o.PriceMode,
			Currency: "EUR", Lines: lines, Subject: "Objednávka " + ref}
		saved, err := a.saveDraft(ctx, companyID, a.withDocExtras(ctx, companyID, inv))
		if err != nil {
			return doc, err
		}
		a.record(ctx, companyID, audit.CatInvoices, "create", "invoice", saved.ID, "Koncept faktúry z objednávky "+o.Number+" pre "+saved.Customer.Name, nil, saved)
		doc, num = SalesOrderDoc{Kind: sorder.ByInvoice, ID: saved.ID}, "koncept"
	default:
		return doc, sorderErr([]sorder.FieldError{{Field: "to", Message: "Vyberte dodací list alebo faktúru"}})
	}
	before := o.Status
	o.Fulfils = append(o.Fulfils, sorder.Fulfil{Kind: doc.Kind, ID: doc.ID, Number: num, Qty: used})
	o.UpdateDelivered()
	if err := a.Store.SaveSalesOrder(ctx, companyID, &o); err != nil {
		return doc, err
	}
	type state struct {
		Status sorder.Status `json:"status"`
	}
	what := map[string]string{sorder.ByDelivery: "dodací list", sorder.ByInvoice: "faktúra"}[doc.Kind]
	a.record(ctx, companyID, audit.CatInvoices, "fulfil", "sales_order", o.ID, "Objednávka "+o.Number+": "+what+" na dodanie", state{before}, state{o.Status})
	kind := link.Delivery
	if doc.Kind == sorder.ByInvoice {
		kind = link.Invoice
	}
	if l, err := link.New(link.Ref{Kind: link.SOrder, ID: o.ID}, link.Ref{Kind: kind, ID: doc.ID}); err == nil {
		if err := a.Store.SaveDocLink(ctx, companyID, &l); err != nil && !errors.Is(err, store.ErrDuplicate) {
			log.Printf("prepojenie objednávky %s: %v", o.Number, err)
		}
	}
	return doc, nil
}

// SalesOrderFromQuote zapíše objednávku z vystavenej alebo prijatej
// cenovej ponuky (odberateľ, položky, ceny) a prepojí ich.
func (a *App) SalesOrderFromQuote(ctx context.Context, companyID, quoteID int64) (sorder.Order, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return sorder.Order{}, err
	}
	q, err := a.Store.GetQuote(ctx, companyID, quoteID)
	if err != nil {
		return sorder.Order{}, err
	}
	if q.Status == quote.StatusDraft || q.Status == quote.StatusRejected {
		return sorder.Order{}, sorderErr([]sorder.FieldError{{Field: "status", Message: "Objednávka sa robí z vystavenej alebo prijatej ponuky"}})
	}
	if q.Currency != "" && q.Currency != "EUR" {
		return sorder.Order{}, sorderErr([]sorder.FieldError{{Field: "currency", Message: "Objednávka je iba v eurách"}})
	}
	o := sorder.Order{Customer: q.Customer, ContactID: q.ContactID, Date: a.Today(), PriceMode: q.PriceMode, Note: "Podľa cenovej ponuky " + q.Number + "."}
	for _, l := range q.Lines {
		o.Lines = append(o.Lines, sorder.Line{Line: l})
	}
	o, err = a.SaveSalesOrder(ctx, companyID, o)
	if err != nil {
		return o, err
	}
	if l, err := link.New(link.Ref{Kind: link.SOrder, ID: o.ID}, link.Ref{Kind: link.Quote, ID: q.ID}); err == nil {
		if err := a.Store.SaveDocLink(ctx, companyID, &l); err != nil && !errors.Is(err, store.ErrDuplicate) {
			log.Printf("prepojenie ponuky %s s objednávkou: %v", q.Number, err)
		}
	}
	return o, nil
}
