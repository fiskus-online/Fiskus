package app

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/forms"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/link"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/quote"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// Krížové prepojenia dokladov: ručné (link.Link v úložisku) a tie, ktoré
// vyplývajú zo samotných dokladov (faktúra z ponuky, dobropis k faktúre,
// zálohová faktúra a vyúčtovanie). Ručné sa dajú zrušiť, vyplývajúce nie.

// LinkedDoc je súvisiaci doklad na zobrazenie.
type LinkedDoc struct {
	link.Ref
	// LinkID je ručné prepojenie (0 = vyplýva z dokladu, nedá sa zrušiť).
	LinkID   int64        `json:"linkId,omitempty"`
	Relation string       `json:"relation,omitempty"` // „faktúra z ponuky“, „dobropis“…
	Type     string       `json:"type"`               // druh: Faktúra, Cenová ponuka, Dodací list…
	Number   string       `json:"number"`
	Partner  string       `json:"partner"`
	Date     invoice.Date `json:"date"`
	Total    money.Dec    `json:"total"`
	Currency string       `json:"currency"`
	Status   string       `json:"status"`
	FormKey  string       `json:"formKey,omitempty"` // tlačivo: druh (na otvorenie)
}

// zákazky číta a mení každý, kto číta a mení faktúry
func linkReadPerm(k link.Kind) auth.Perm {
	if k == link.Purchase || k == link.POrder {
		return auth.PermPurchasesRead
	}
	return auth.PermInvoicesRead
}

func linkWritePerm(k link.Kind) auth.Perm {
	if k == link.Purchase || k == link.POrder {
		return auth.PermPurchasesWrite
	}
	return auth.PermInvoicesWrite
}

var invoiceTypes = map[invoice.Kind]string{invoice.KindInvoice: "Faktúra", invoice.KindCreditNote: "Dobropis",
	invoice.KindProforma: "Zálohová faktúra", invoice.KindAdvanceReceipt: "Doklad k platbe"}

// docInfo načíta údaje dokladu (ErrNotFound, ak nie je).
func (a *App) docInfo(ctx context.Context, companyID int64, r link.Ref) (LinkedDoc, error) {
	d := LinkedDoc{Ref: r}
	switch r.Kind {
	case link.Invoice:
		inv, err := a.Store.GetInvoice(ctx, companyID, r.ID)
		if err != nil {
			return d, err
		}
		d.Type, d.Number, d.Partner, d.Date = firstNonBlank(invoiceTypes[inv.Kind], "Faktúra"), inv.Number, inv.Customer.Name, inv.IssueDate
		d.Total, d.Currency, d.Status = inv.Summary.Total, inv.Currency, string(inv.Status)
	case link.Quote:
		q, err := a.Store.GetQuote(ctx, companyID, r.ID)
		if err != nil {
			return d, err
		}
		d.Type, d.Number, d.Partner, d.Date = "Cenová ponuka", q.Number, q.Customer.Name, q.IssueDate
		d.Total, d.Currency, d.Status = q.Summary.Total, q.Currency, string(q.Status)
	case link.Purchase:
		p, err := a.Store.GetPurchase(ctx, companyID, r.ID)
		if err != nil {
			return d, err
		}
		d.Type, d.Number, d.Partner, d.Date = "Prijatá faktúra", firstNonBlank(p.SupplierNumber, p.Number), p.Supplier.Name, p.IssueDate
		d.Total, d.Currency, d.Status = p.Summary.Total, p.Currency, string(p.Status)
		if d.Currency == "" {
			d.Currency = "EUR" // prázdna mena prijatej faktúry = euro
		}
	case link.Form:
		fd, err := a.Store.GetFormDoc(ctx, companyID, r.ID)
		if err != nil {
			return d, err
		}
		if fd.Template {
			return d, store.ErrNotFound // vzor nie je doklad
		}
		f, err := a.formByKey(ctx, companyID, fd.FormKey)
		if err != nil {
			f = forms.Form{Name: fd.Title}
		}
		d.Type, d.FormKey = firstNonBlank(f.Name, fd.Title, "Tlačivo"), fd.FormKey
		for _, fl := range f.Fields {
			v := strings.TrimSpace(fd.Values[fl.Key])
			switch fl.Role {
			case "number":
				d.Number = v
			case "partner":
				d.Partner, _, _ = strings.Cut(v, "\n")
			case "date":
				d.Date, _ = invoice.ParseDate(v)
			}
		}
		if c, err := forms.Compute(f, fd.Values, fd.Items); err == nil || !c.Total.IsZero() {
			d.Total = c.Total
			if d.Total.IsZero() {
				d.Total = c.ItemsTotal
			}
		}
		d.Currency = "EUR"
		if d.Date.IsZero() {
			d.Date = fd.Updated
		}
		if fd.CashID != 0 {
			d.Status = "posted"
		}
	case link.Job:
		j, err := a.Store.GetJob(ctx, companyID, r.ID)
		if err != nil {
			return d, err
		}
		d.Type, d.Number, d.Partner, d.Date = "Zákazka", j.Number, firstNonBlank(j.Customer, j.Name), firstDate(j.Start, j.Created)
		if j.Customer != "" {
			d.Partner = j.Customer + " — " + j.Name
		}
		d.Total, d.Currency, d.Status = j.Budget, "EUR", string(j.Status)
	case link.POrder:
		o, err := a.Store.GetPurchaseOrder(ctx, companyID, r.ID)
		if err != nil {
			return d, err
		}
		d.Type, d.Number, d.Partner, d.Date = "Objednávka", o.Number, o.Supplier, o.Date
		d.Total, d.Currency, d.Status = o.Total, "EUR", string(o.Status)
	default:
		return d, link.ErrUnknown
	}
	return d, nil
}

type implied struct {
	ref      link.Ref
	relation string
}

// implicit vráti prepojenia, ktoré vyplývajú zo samotných dokladov.
func (a *App) implicit(ctx context.Context, companyID int64, r link.Ref) ([]implied, error) {
	var out []implied
	add := func(k link.Kind, id int64, rel string) {
		if id > 0 {
			out = append(out, implied{link.Ref{Kind: k, ID: id}, rel})
		}
	}
	switch r.Kind {
	case link.Quote:
		q, err := a.Store.GetQuote(ctx, companyID, r.ID)
		if err != nil {
			return nil, err
		}
		add(link.Invoice, q.InvoiceID, "faktúra z ponuky")
	case link.Invoice:
		inv, err := a.Store.GetInvoice(ctx, companyID, r.ID)
		if err != nil {
			return nil, err
		}
		if inv.Correction != nil {
			add(link.Invoice, inv.Correction.OriginalID, "opravovaná faktúra")
		}
		if ad := inv.Advance; ad != nil {
			add(link.Invoice, ad.ProformaID, "zálohová faktúra")
			add(link.Invoice, ad.FinalID, "vyúčtovanie")
			add(link.Invoice, ad.ReceiptID, "doklad k platbe")
			for _, rc := range ad.Receipts {
				add(link.Invoice, rc.ID, "doklad k platbe")
			}
			for _, dd := range ad.Deductions {
				add(link.Invoice, dd.ProformaID, "odpočítaná záloha")
			}
		}
		// spätné väzby: dobropisy k faktúre, ponuky vyfakturované touto faktúrou
		invs, err := a.Store.ListInvoices(ctx, companyID, store.InvoiceFilter{})
		if err != nil {
			return nil, err
		}
		for _, x := range invs {
			if x.Correction != nil && x.Correction.OriginalID == inv.ID {
				add(link.Invoice, x.ID, "dobropis")
			}
		}
		qs, err := a.Store.ListQuotes(ctx, companyID, 0)
		if err != nil {
			return nil, err
		}
		for _, q := range qs {
			if q.InvoiceID == inv.ID {
				add(link.Quote, q.ID, "ponuka vyfakturovaná touto faktúrou")
			}
		}
	}
	return out, nil
}

// DocLinks vráti doklady súvisiace s dokladom r.
func (a *App) DocLinks(ctx context.Context, companyID int64, r link.Ref) ([]LinkedDoc, error) {
	if !r.Kind.Known() {
		return nil, link.ErrUnknown
	}
	if err := a.Require(ctx, companyID, linkReadPerm(r.Kind)); err != nil {
		return nil, err
	}
	if _, err := a.docInfo(ctx, companyID, r); err != nil {
		return nil, err
	}
	out := []LinkedDoc{}
	seen := map[link.Ref]bool{r: true}
	visible := func(k link.Kind) bool { return a.Require(ctx, companyID, linkReadPerm(k)) == nil }
	imp, err := a.implicit(ctx, companyID, r)
	if err != nil {
		return nil, err
	}
	for _, im := range imp {
		if seen[im.ref] || !visible(im.ref.Kind) {
			continue
		}
		d, err := a.docInfo(ctx, companyID, im.ref)
		if err != nil {
			continue // zmazaný koncept
		}
		seen[im.ref] = true
		d.Relation = im.relation
		out = append(out, d)
	}
	links, err := a.Store.ListDocLinks(ctx, companyID, r)
	if err != nil {
		return nil, err
	}
	for _, l := range links {
		o := l.Other(r)
		if seen[o] || !visible(o.Kind) {
			continue
		}
		d, err := a.docInfo(ctx, companyID, o)
		if err != nil {
			continue
		}
		seen[o] = true
		d.LinkID = l.ID
		out = append(out, d)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out, nil
}

// LinkDocs prepojí dva doklady a vráti súvisiace doklady prvého.
func (a *App) LinkDocs(ctx context.Context, companyID int64, from, to link.Ref) ([]LinkedDoc, error) {
	l, err := link.New(from, to)
	if err != nil {
		return nil, vErr("doc", err.Error())
	}
	if err := a.Require(ctx, companyID, linkWritePerm(from.Kind)); err != nil {
		return nil, err
	}
	if err := a.Require(ctx, companyID, linkReadPerm(to.Kind)); err != nil {
		return nil, err
	}
	fd, err := a.docInfo(ctx, companyID, from)
	if err != nil {
		return nil, err
	}
	td, err := a.docInfo(ctx, companyID, to)
	if errors.Is(err, store.ErrNotFound) {
		return nil, vErr("doc", "Doklad sa nenašiel")
	} else if err != nil {
		return nil, err
	}
	if err := a.Store.SaveDocLink(ctx, companyID, &l); errors.Is(err, store.ErrDuplicate) {
		return nil, vErr("doc", "Doklady už sú prepojené")
	} else if err != nil {
		return nil, err
	}
	a.record(ctx, companyID, audit.CatInvoices, "link", string(from.Kind), from.ID,
		"Prepojenie: "+docLabel(fd)+" ↔ "+docLabel(td), nil, l)
	return a.DocLinks(ctx, companyID, from)
}

// UnlinkDocs zruší ručné prepojenie dokladu r.
func (a *App) UnlinkDocs(ctx context.Context, companyID int64, r link.Ref, linkID int64) ([]LinkedDoc, error) {
	if err := a.Require(ctx, companyID, linkWritePerm(r.Kind)); err != nil {
		return nil, err
	}
	links, err := a.Store.ListDocLinks(ctx, companyID, r)
	if err != nil {
		return nil, err
	}
	for _, l := range links {
		if l.ID != linkID {
			continue
		}
		if err := a.Store.DeleteDocLink(ctx, companyID, l.ID); err != nil {
			return nil, err
		}
		fd, _ := a.docInfo(ctx, companyID, r)
		td, _ := a.docInfo(ctx, companyID, l.Other(r))
		a.record(ctx, companyID, audit.CatInvoices, "unlink", string(r.Kind), r.ID,
			"Zrušené prepojenie: "+docLabel(fd)+" ↔ "+docLabel(td), l, nil)
		return a.DocLinks(ctx, companyID, r)
	}
	return nil, store.ErrNotFound
}

func docLabel(d LinkedDoc) string {
	s := d.Type
	if d.Number != "" {
		s += " " + d.Number
	}
	if d.Partner != "" {
		s += " (" + d.Partner + ")"
	}
	return s
}

// QuoteInvoicedBy označí ponuku ako vyfakturovanú existujúcou faktúrou
// (napr. faktúra vznikla mimo ponuky alebo po prenose z iného programu).
func (a *App) QuoteInvoicedBy(ctx context.Context, companyID, quoteID, invoiceID int64) (quote.Quote, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return quote.Quote{}, err
	}
	q, err := a.Store.GetQuote(ctx, companyID, quoteID)
	if err != nil {
		return q, err
	}
	inv, err := a.Store.GetInvoice(ctx, companyID, invoiceID)
	if errors.Is(err, store.ErrNotFound) {
		return q, vErr("invoiceId", "Faktúra sa nenašla")
	} else if err != nil {
		return q, err
	}
	if inv.Kind != invoice.KindInvoice && inv.Kind != invoice.KindProforma {
		return q, vErr("invoiceId", "Vyberte faktúru alebo zálohovú faktúru")
	}
	if inv.Status == invoice.StatusDraft || inv.Number == "" {
		return q, vErr("invoiceId", "Faktúra ešte nie je vystavená")
	}
	before := q
	if err := q.MarkInvoiced(inv.ID, inv.Number); err != nil {
		return q, err
	}
	if err := a.Store.SaveQuote(ctx, companyID, &q); err != nil {
		return q, err
	}
	a.record(ctx, companyID, audit.CatQuotes, "invoice", "quote", q.ID,
		"Ponuka "+q.Number+" vyfakturovaná faktúrou "+inv.Number, before, q)
	return q, nil
}
