package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/delivery"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/link"
	"github.com/fiskus-online/fiskus/backend/internal/quote"
	"github.com/fiskus-online/fiskus/backend/internal/stock"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// Dodacie listy: koncept → vydaný (číslo DL20260001, výdaj zo skladu) →
// vyfakturovaný, alebo stornovaný. Faktúra vzniká z jedného alebo viacerých
// vydaných dodacích listov toho istého odberateľa; jej riadky už nie sú
// viazané na sklad (tovar vydal dodací list). Zmazaním konceptu alebo
// stornom faktúry sa dodací list vráti do stavu vydaný.

const (
	deliverySeries    = "DL"
	deliveryStockLink = "delivery"
)

func deliveryErr(errs []delivery.FieldError) error {
	if len(errs) == 0 {
		return nil
	}
	return &invoice.ValidationError{Fields: errs}
}

// releaseDelivery: vyfakturovaný dodací list, ktorého faktúra už neplatí,
// sa vráti na vydaný; doplní číslo faktúry. Vráti true pri zmene.
func (a *App) releaseDelivery(ctx context.Context, companyID int64, n *delivery.Note) bool {
	if n.InvoiceID == 0 {
		return false
	}
	inv, err := a.Store.GetInvoice(ctx, companyID, n.InvoiceID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && inv.Status == invoice.StatusCancelled) {
		n.InvoiceID, n.InvoiceNumber = 0, ""
		if n.Status == delivery.Invoiced {
			n.Status = delivery.Issued
		}
		return true
	}
	num := "koncept"
	if err == nil && inv.Number != "" {
		num = inv.Number
	}
	if num != n.InvoiceNumber {
		n.InvoiceNumber = num
		return true
	}
	return false
}

func (a *App) refreshDelivery(ctx context.Context, companyID int64, n *delivery.Note) error {
	if a.releaseDelivery(ctx, companyID, n) {
		return a.Store.SaveDelivery(ctx, companyID, n)
	}
	return nil
}

// Deliveries vráti dodacie listy, najnovšie prvé.
func (a *App) Deliveries(ctx context.Context, companyID int64) ([]delivery.Note, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return nil, err
	}
	list, err := a.Store.ListDeliveries(ctx, companyID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if err := a.refreshDelivery(ctx, companyID, &list[i]); err != nil {
			return nil, err
		}
	}
	return list, nil
}

// Delivery vráti dodací list.
func (a *App) Delivery(ctx context.Context, companyID, id int64) (delivery.Note, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return delivery.Note{}, err
	}
	n, err := a.Store.GetDelivery(ctx, companyID, id)
	if err != nil {
		return n, err
	}
	return n, a.refreshDelivery(ctx, companyID, &n)
}

// prepareDelivery doplní odberateľa z adresára, platiteľa DPH a prepočíta sumy.
func (a *App) prepareDelivery(ctx context.Context, companyID int64, n *delivery.Note) error {
	c, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return err
	}
	n.NonVATPayer = !c.VATPayer
	if n.ContactID != 0 && strings.TrimSpace(n.Customer.Name) == "" {
		ct, err := a.Store.GetContact(ctx, companyID, n.ContactID)
		if err != nil {
			return deliveryErr([]delivery.FieldError{{Field: "contactId", Message: "Odberateľ sa v adresári nenašiel"}})
		}
		n.Customer = PartyFromContact(ct)
	}
	if n.Customer.Country == "" {
		n.Customer.Country = "SK"
	}
	if err := n.Recalculate(); err != nil {
		return err
	}
	return deliveryErr(n.Validate())
}

// SaveDelivery založí koncept (ID 0) alebo upraví koncept či vydaný,
// nevyfakturovaný dodací list (číslo ostane, sklad sa zosúladí).
func (a *App) SaveDelivery(ctx context.Context, companyID int64, n delivery.Note) (delivery.Note, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return n, err
	}
	var before *delivery.Note
	if n.ID != 0 {
		old, err := a.Store.GetDelivery(ctx, companyID, n.ID)
		if err != nil {
			return n, err
		}
		a.releaseDelivery(ctx, companyID, &old)
		if !old.Editable() {
			return n, deliveryErr([]delivery.FieldError{{Field: "status", Message: "Vyfakturovaný alebo stornovaný dodací list sa nemení"}})
		}
		before = &old
		n.Number, n.Status, n.InvoiceID, n.InvoiceNumber = old.Number, old.Status, old.InvoiceID, old.InvoiceNumber
	} else {
		n.Number, n.Status, n.InvoiceID, n.InvoiceNumber = "", delivery.Draft, 0, ""
		if n.Date.IsZero() {
			n.Date = a.Today()
		}
	}
	if err := a.prepareDelivery(ctx, companyID, &n); err != nil {
		return n, err
	}
	if err := a.Store.SaveDelivery(ctx, companyID, &n); err != nil {
		return n, err
	}
	action, what := "create", "Nový dodací list pre "
	if before != nil {
		action, what = "update", "Úprava dodacieho listu "+n.Number+" pre "
	}
	a.record(ctx, companyID, audit.CatInvoices, action, "delivery", n.ID, what+n.Customer.Name, before, n)
	if n.Status == delivery.Issued {
		if err := a.syncDeliveryStock(ctx, companyID, n); err != nil {
			return n, err
		}
	}
	return n, nil
}

// deliveryTransition zmení stav dodacieho listu a zapíše ho do auditu.
func (a *App) deliveryTransition(ctx context.Context, companyID, id int64, action, what string, fn func(*delivery.Note) error) (delivery.Note, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return delivery.Note{}, err
	}
	n, err := a.Store.GetDelivery(ctx, companyID, id)
	if err != nil {
		return n, err
	}
	a.releaseDelivery(ctx, companyID, &n)
	before := n.Status
	if err := fn(&n); err != nil {
		return n, err
	}
	if err := a.Store.SaveDelivery(ctx, companyID, &n); err != nil {
		return n, err
	}
	type state struct {
		Status delivery.Status `json:"status"`
	}
	a.record(ctx, companyID, audit.CatInvoices, action, "delivery", n.ID, what+" "+n.Number+" pre "+n.Customer.Name, state{before}, state{n.Status})
	return n, a.syncDeliveryStock(ctx, companyID, n)
}

// IssueDelivery vydá koncept: pridelí číslo a vydá tovar zo skladu.
func (a *App) IssueDelivery(ctx context.Context, companyID, id int64) (delivery.Note, error) {
	return a.deliveryTransition(ctx, companyID, id, "issue", "Vydaný dodací list", func(n *delivery.Note) error {
		if n.Status != delivery.Draft {
			return deliveryErr([]delivery.FieldError{{Field: "status", Message: "Vydať možno iba koncept"}})
		}
		if err := a.prepareDelivery(ctx, companyID, n); err != nil {
			return err
		}
		seq, err := a.Store.NextSequence(ctx, companyID, deliverySeries, n.Date.Year())
		if err != nil {
			return err
		}
		n.Number, n.Status = fmt.Sprintf("DL%04d%04d", n.Date.Year(), seq), delivery.Issued
		return nil
	})
}

// CancelDelivery stornuje vydaný, nevyfakturovaný dodací list (tovar sa vráti na sklad).
func (a *App) CancelDelivery(ctx context.Context, companyID, id int64) (delivery.Note, error) {
	return a.deliveryTransition(ctx, companyID, id, "cancel", "Stornovaný dodací list", func(n *delivery.Note) error {
		if n.Status != delivery.Issued {
			return deliveryErr([]delivery.FieldError{{Field: "status", Message: "Stornovať možno iba vydaný, nevyfakturovaný dodací list"}})
		}
		n.Status = delivery.Cancelled
		return nil
	})
}

// DeleteDelivery zmaže koncept dodacieho listu a jeho prepojenia.
func (a *App) DeleteDelivery(ctx context.Context, companyID, id int64) error {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return err
	}
	n, err := a.Store.GetDelivery(ctx, companyID, id)
	if err != nil {
		return err
	}
	if n.Status != delivery.Draft {
		return deliveryErr([]delivery.FieldError{{Field: "status", Message: "Vydaný dodací list sa nemaže, iba stornuje"}})
	}
	links, err := a.Store.ListDocLinks(ctx, companyID, link.Ref{Kind: link.Delivery, ID: id})
	if err != nil {
		return err
	}
	for _, l := range links {
		if err := a.Store.DeleteDocLink(ctx, companyID, l.ID); err != nil {
			return err
		}
	}
	if err := a.Store.DeleteDelivery(ctx, companyID, id); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatInvoices, "delete", "delivery", id, "Zmazaný koncept dodacieho listu pre "+n.Customer.Name, n, nil)
	return nil
}

// syncDeliveryStock zosúladí výdajku dodacieho listu: vydaný alebo
// vyfakturovaný vydá skladové položky z cenníka, koncept a storno nič.
func (a *App) syncDeliveryStock(ctx context.Context, companyID int64, n delivery.Note) error {
	stockMu.Lock()
	defer stockMu.Unlock()
	cfg, err := a.loadStock(ctx, companyID)
	if err != nil {
		return err
	}
	docs, err := a.Store.ListStockDocs(ctx, companyID)
	if err != nil {
		return err
	}
	var existing *stock.Doc
	for i := range docs {
		if docs[i].Auto && docs[i].LinkKind == deliveryStockLink && docs[i].LinkID == n.ID {
			existing = &docs[i]
			break
		}
	}
	var lines []stock.Line
	if (n.Status == delivery.Issued || n.Status == delivery.Invoiced) && len(cfg.Warehouses) > 0 {
		for _, l := range n.Lines {
			if _, ok := cfg.item(l.ItemID); l.ItemID != 0 && ok && l.Quantity.Sign() > 0 {
				lines = append(lines, stock.Line{ItemID: l.ItemID, Quantity: l.Quantity})
			}
		}
	}
	if len(lines) == 0 {
		if existing != nil {
			if err := a.Store.DeleteStockDoc(ctx, companyID, existing.ID); err != nil {
				return err
			}
			a.record(ctx, companyID, audit.CatSettings, "delete", "stock_doc", existing.ID, "Zmazaný "+stockDocSummary(*existing)+" (dodací list "+n.Number+")", existing, nil)
		}
		return nil
	}
	d := stock.Doc{Kind: stock.Issue, Date: n.Date, Warehouse: cfg.Warehouses[0].Code, Partner: n.Customer.Name,
		LinkKind: deliveryStockLink, LinkID: n.ID, LinkNumber: n.Number, Auto: true, Lines: lines}
	action := "create"
	var before any
	if existing != nil {
		d.ID, d.Number, d.Warehouse, action, before = existing.ID, existing.Number, existing.Warehouse, "update", *existing
	}
	if err := a.checkStockDoc(ctx, companyID, cfg, &d); err != nil {
		return err
	}
	if d.ID == 0 {
		d.Number = stock.NextNumber(docs, d.Kind, d.Date.Year())
	}
	if err := a.Store.SaveStockDoc(ctx, companyID, &d); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatSettings, action, "stock_doc", d.ID, stockDocSummary(d)+" (dodací list "+n.Number+")", before, d)
	return nil
}

// InvoiceDeliveries vytvorí koncept faktúry z vydaných dodacích listov
// jedného odberateľa, prepojí ich s ňou a označí ako vyfakturované.
func (a *App) InvoiceDeliveries(ctx context.Context, companyID int64, ids []int64) (invoice.Invoice, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return invoice.Invoice{}, err
	}
	if len(ids) == 0 {
		return invoice.Invoice{}, deliveryErr([]delivery.FieldError{{Field: "ids", Message: "Vyberte dodacie listy"}})
	}
	var notes []delivery.Note
	seen := map[int64]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		n, err := a.Store.GetDelivery(ctx, companyID, id)
		if err != nil {
			return invoice.Invoice{}, err
		}
		a.releaseDelivery(ctx, companyID, &n)
		if !n.Invoiceable() {
			return invoice.Invoice{}, deliveryErr([]delivery.FieldError{{Field: "ids", Message: "Dodací list " + firstNonBlank(n.Number, "(koncept)") + " nie je vydaný alebo je už vyfakturovaný"}})
		}
		if len(notes) > 0 {
			f := notes[0]
			if n.ContactID != f.ContactID || !strings.EqualFold(strings.TrimSpace(n.Customer.Name), strings.TrimSpace(f.Customer.Name)) {
				return invoice.Invoice{}, deliveryErr([]delivery.FieldError{{Field: "ids", Message: "Na jednu faktúru možno dať iba dodacie listy toho istého odberateľa"}})
			}
			if n.PriceMode != f.PriceMode {
				return invoice.Invoice{}, deliveryErr([]delivery.FieldError{{Field: "ids", Message: "Dodacie listy majú rôzne ceny (s DPH a bez DPH)"}})
			}
		}
		notes = append(notes, n)
	}
	sort.Slice(notes, func(i, k int) bool { return notes[i].Number < notes[k].Number })
	first := notes[0]
	inv := invoice.Invoice{Kind: invoice.KindInvoice, Customer: first.Customer, ContactID: first.ContactID, IssueDate: a.Today(),
		PriceMode: first.PriceMode, Currency: "EUR"}
	var nums []string
	for _, n := range notes {
		nums = append(nums, n.Number)
		if n.Date.After(inv.DeliveryDate) {
			inv.DeliveryDate = n.Date
		}
		for _, l := range n.Lines {
			l.ItemID = 0 // tovar vydal dodací list
			inv.Lines = append(inv.Lines, l)
		}
		if inv.Subject == "" && n.Order != "" {
			inv.Subject = "Objednávka " + n.Order
		}
	}
	if len(nums) == 1 {
		inv.Note = "Podľa dodacieho listu " + nums[0] + "."
	} else {
		inv.Note = "Podľa dodacích listov " + strings.Join(nums, ", ") + "."
	}
	saved, err := a.saveDraft(ctx, companyID, a.withDocExtras(ctx, companyID, inv))
	if err != nil {
		return saved, err
	}
	a.record(ctx, companyID, audit.CatInvoices, "create", "invoice", saved.ID, "Koncept faktúry z dodacích listov "+strings.Join(nums, ", ")+" pre "+saved.Customer.Name, nil, saved)
	for _, n := range notes {
		before := n.Status
		n.Status, n.InvoiceID, n.InvoiceNumber = delivery.Invoiced, saved.ID, "koncept"
		if err := a.Store.SaveDelivery(ctx, companyID, &n); err != nil {
			return saved, err
		}
		type state struct {
			Status    delivery.Status `json:"status"`
			InvoiceID int64           `json:"invoiceId,omitempty"`
		}
		a.record(ctx, companyID, audit.CatInvoices, "invoice", "delivery", n.ID, "Vyfakturovaný dodací list "+n.Number, state{Status: before}, state{n.Status, saved.ID})
		l, err := link.New(link.Ref{Kind: link.Delivery, ID: n.ID}, link.Ref{Kind: link.Invoice, ID: saved.ID})
		if err != nil {
			return saved, err
		}
		if err := a.Store.SaveDocLink(ctx, companyID, &l); err != nil && !errors.Is(err, store.ErrDuplicate) {
			return saved, err
		}
	}
	return saved, nil
}

// DeliveryFromQuote vytvorí koncept dodacieho listu z cenovej ponuky
// (odberateľ, položky a ceny) a prepojí ich.
func (a *App) DeliveryFromQuote(ctx context.Context, companyID, quoteID int64) (delivery.Note, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return delivery.Note{}, err
	}
	q, err := a.Store.GetQuote(ctx, companyID, quoteID)
	if err != nil {
		return delivery.Note{}, err
	}
	if q.Status == quote.StatusDraft || q.Status == quote.StatusRejected {
		return delivery.Note{}, deliveryErr([]delivery.FieldError{{Field: "status", Message: "Dodací list sa robí z vystavenej alebo prijatej ponuky"}})
	}
	if q.Currency != "" && q.Currency != "EUR" {
		return delivery.Note{}, deliveryErr([]delivery.FieldError{{Field: "currency", Message: "Dodací list je iba v eurách"}})
	}
	n := delivery.Note{Customer: q.Customer, ContactID: q.ContactID, Date: a.Today(), PriceMode: q.PriceMode,
		Lines: append([]invoice.Line(nil), q.Lines...), Note: "Podľa cenovej ponuky " + q.Number + "."}
	n, err = a.SaveDelivery(ctx, companyID, n)
	if err != nil {
		return n, err
	}
	if l, err := link.New(link.Ref{Kind: link.Delivery, ID: n.ID}, link.Ref{Kind: link.Quote, ID: q.ID}); err == nil {
		if err := a.Store.SaveDocLink(ctx, companyID, &l); err != nil && !errors.Is(err, store.ErrDuplicate) {
			log.Printf("prepojenie ponuky %s s dodacím listom: %v", q.Number, err)
		}
	}
	return n, nil
}
