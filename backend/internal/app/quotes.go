package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/link"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/quote"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// quoteSeries je kľúč číselného radu ponúk (nezávislý od formátu).
const quoteSeries = "CP"

// Quotes vráti cenové ponuky firmy (year 0 = všetky).
func (a *App) Quotes(ctx context.Context, companyID int64, year int) ([]quote.Quote, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return nil, err
	}
	list, err := a.Store.ListQuotes(ctx, companyID, year)
	if err != nil {
		return nil, err
	}
	for i := range list {
		a.linkInvoice(ctx, companyID, &list[i])
	}
	return list, nil
}

// linkInvoice doplní aktuálne číslo faktúry vytvorenej z ponuky (koncept
// faktúry číslo ešte nemá, dostane ho až pri vystavení).
func (a *App) linkInvoice(ctx context.Context, companyID int64, q *quote.Quote) {
	if q.Share != nil {
		q.Share.URL = a.QuoteURL(*q)
	}
	a.refreshParts(ctx, companyID, q)
	if q.InvoiceID == 0 {
		return
	}
	if inv, err := a.Store.GetInvoice(ctx, companyID, q.InvoiceID); err == nil {
		q.InvoiceNumber = inv.Number
	}
}

// refreshParts vyradí neplatné čiastkové faktúry (zmazaný koncept, storno),
// doplní ich čísla a zostatok riadkov.
func (a *App) refreshParts(ctx context.Context, companyID int64, q *quote.Quote) {
	if len(q.Billed) == 0 {
		return
	}
	numbers := map[int64]string{}
	if q.ReleaseParts(func(id int64) bool {
		inv, err := a.Store.GetInvoice(ctx, companyID, id)
		if errors.Is(err, store.ErrNotFound) || (err == nil && inv.Status == invoice.StatusCancelled) {
			return false
		}
		numbers[id] = "koncept"
		if err == nil && inv.Number != "" {
			numbers[id] = inv.Number
		}
		return true
	}) {
		if err := a.Store.SaveQuote(ctx, companyID, q); err != nil {
			log.Printf("ponuka %s: uvoľnenie čiastkových faktúr: %v", q.Number, err)
		}
	}
	for i := range q.Billed {
		q.Billed[i].InvoiceNumber = numbers[q.Billed[i].InvoiceID]
	}
	if len(q.Billed) > 0 {
		q.Remaining = q.Left()
	}
}

// Quote vráti jednu ponuku.
func (a *App) Quote(ctx context.Context, companyID, id int64) (quote.Quote, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return quote.Quote{}, err
	}
	q, err := a.Store.GetQuote(ctx, companyID, id)
	if err == nil {
		a.linkInvoice(ctx, companyID, &q)
	}
	return q, err
}

// prepareQuote doplní údaje odvodené od firmy a prepočíta sumy.
func (a *App) prepareQuote(ctx context.Context, companyID int64, q *quote.Quote) error {
	c, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return err
	}
	q.NonVATPayer = !c.VATPayer
	if q.NonVATPayer {
		q.ReverseCharge = false
	}
	q.Note = strings.TrimSpace(q.Note)
	return q.Recalculate()
}

// PreviewQuote prepočíta ponuku bez uloženia (živý výpočet v editore).
func (a *App) PreviewQuote(ctx context.Context, companyID int64, q quote.Quote) (quote.Quote, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return q, err
	}
	err := a.prepareQuote(ctx, companyID, &q)
	return q, err
}

// SaveQuote uloží nový alebo upravený koncept ponuky. Koncept sa uloží aj
// neúplný — kontroluje sa pri vystavení.
func (a *App) SaveQuote(ctx context.Context, companyID int64, q quote.Quote) (quote.Quote, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return q, err
	}
	var before *quote.Quote
	if q.ID != 0 {
		old, err := a.Store.GetQuote(ctx, companyID, q.ID)
		if err != nil {
			return q, err
		}
		if !old.Editable() {
			if !old.Amendable() {
				return q, quote.ErrLocked
			}
			return a.amendQuote(ctx, companyID, old, q)
		}
		before = &old
	}
	q.Status, q.Number, q.InvoiceID, q.InvoiceNumber, q.Share = quote.StatusDraft, "", 0, "", nil
	if q.IssueDate.IsZero() {
		q.IssueDate = a.Today()
	}
	if q.ValidUntil.IsZero() {
		q.ValidUntil = q.IssueDate.AddDays(quote.DefaultValidDays)
	}
	if err := a.prepareQuote(ctx, companyID, &q); err != nil {
		return q, err
	}
	if err := a.Store.SaveQuote(ctx, companyID, &q); err != nil {
		return q, err
	}
	if before == nil {
		a.record(ctx, companyID, audit.CatQuotes, "create", "quote", q.ID, "Nový koncept ponuky pre "+q.Customer.Name, nil, q)
	} else {
		a.record(ctx, companyID, audit.CatQuotes, "update", "quote", q.ID, "Úprava konceptu ponuky pre "+q.Customer.Name, before, q)
	}
	return q, nil
}

// IssueQuote vystaví koncept a pridelí mu číslo z radu ponúk.
func (a *App) IssueQuote(ctx context.Context, companyID, id int64) (quote.Quote, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return quote.Quote{}, err
	}
	q, err := a.Store.GetQuote(ctx, companyID, id)
	if err != nil {
		return q, err
	}
	if !q.Editable() {
		return q, quote.ErrBadTransition
	}
	// kontrola pred pridelením čísla, aby v rade nevznikali diery
	if err := a.prepareQuote(ctx, companyID, &q); err != nil {
		return q, err
	}
	if err := q.Validate(); err != nil {
		return q, err
	}
	company, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return q, err
	}
	year, month := q.IssueDate.Year(), q.IssueDate.Month()
	seq, err := a.Store.NextSequence(ctx, companyID, quoteSeries, year)
	if err != nil {
		return q, err
	}
	format := company.NumberFormats[QuoteFormatKey]
	if format == "" {
		format = ExtraFormats[QuoteFormatKey]
	}
	number := invoice.FormatNumberWith(format, year, month, seq)
	// číslo už môže mať ponuka prenesená z iného programu → ďalšie v rade
	if all, err := a.Store.ListQuotes(ctx, companyID, 0); err == nil {
		taken := map[string]bool{}
		for _, x := range all {
			taken[x.Number] = x.Number != ""
		}
		for i := 0; i < 1000 && taken[number]; i++ {
			if seq, err = a.Store.NextSequence(ctx, companyID, quoteSeries, year); err != nil {
				return q, err
			}
			number = invoice.FormatNumberWith(format, year, month, seq)
		}
	}
	if err := q.Issue(number); err != nil {
		return q, err
	}
	if err := a.Store.SaveQuote(ctx, companyID, &q); err != nil {
		return q, err
	}
	a.record(ctx, companyID, audit.CatQuotes, "issue", "quote", q.ID,
		"Vystavená ponuka "+q.Number+" pre "+q.Customer.Name+", "+q.Summary.Total.Display()+" €", nil, q)
	if a.PublicURL != "" {
		// na serveri s verejnou adresou dostane ponuka hneď odkaz pre zákazníka (do e-mailu)
		if err := a.share(ctx, companyID, &q); err != nil {
			return q, err
		}
		a.linkInvoice(ctx, companyID, &q)
	}
	return q, nil
}

// quoteTransition zmení stav ponuky a zapíše to do auditu.
func (a *App) quoteTransition(ctx context.Context, companyID, id int64, action, what string, fn func(*quote.Quote) error) (quote.Quote, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return quote.Quote{}, err
	}
	q, err := a.Store.GetQuote(ctx, companyID, id)
	if err != nil {
		return q, err
	}
	before := q.Status
	if err := fn(&q); err != nil {
		return q, err
	}
	if err := a.Store.SaveQuote(ctx, companyID, &q); err != nil {
		return q, err
	}
	type state struct {
		Status quote.Status `json:"status"`
	}
	a.record(ctx, companyID, audit.CatQuotes, action, "quote", q.ID, what+q.Number+" ("+q.Customer.Name+")",
		state{before}, state{q.Status})
	return q, nil
}

// AcceptQuote označí ponuku ako prijatú zákazníkom.
func (a *App) AcceptQuote(ctx context.Context, companyID, id int64) (quote.Quote, error) {
	return a.quoteTransition(ctx, companyID, id, "accept", "Prijatá ponuka ", (*quote.Quote).Accept)
}

// RejectQuote označí ponuku ako odmietnutú.
func (a *App) RejectQuote(ctx context.Context, companyID, id int64) (quote.Quote, error) {
	return a.quoteTransition(ctx, companyID, id, "reject", "Odmietnutá ponuka ", (*quote.Quote).Reject)
}

// CopyQuote vytvorí z ponuky nový koncept s dnešným dátumom (nová verzia ponuky).
func (a *App) CopyQuote(ctx context.Context, companyID, id int64) (quote.Quote, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return quote.Quote{}, err
	}
	q, err := a.Store.GetQuote(ctx, companyID, id)
	if err != nil {
		return q, err
	}
	days := quote.DefaultValidDays
	if !q.IssueDate.IsZero() && !q.ValidUntil.IsZero() {
		if d := q.IssueDate.DaysUntil(q.ValidUntil); d > 0 {
			days = d
		}
	}
	c := q.Copy(a.Today(), days)
	if err := a.prepareQuote(ctx, companyID, &c); err != nil {
		return c, err
	}
	if err := a.Store.SaveQuote(ctx, companyID, &c); err != nil {
		return c, err
	}
	src := q.Number
	if src == "" {
		src = "konceptu"
	}
	a.record(ctx, companyID, audit.CatQuotes, "create", "quote", c.ID, "Kópia ponuky "+src+" pre "+c.Customer.Name, nil, c)
	return c, nil
}

// InvoiceQuote vytvorí z vystavenej alebo prijatej ponuky koncept faktúry.
// Ponuka sa označí ako vyfakturovaná a odkáže na faktúru.
func (a *App) InvoiceQuote(ctx context.Context, companyID, id int64) (invoice.Invoice, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return invoice.Invoice{}, err
	}
	q, err := a.Store.GetQuote(ctx, companyID, id)
	if err != nil {
		return invoice.Invoice{}, err
	}
	a.refreshParts(ctx, companyID, &q)
	if len(q.Billed) > 0 { // po čiastkových faktúrach sa fakturuje zostatok
		return a.InvoiceQuotePart(ctx, companyID, id, q.Left())
	}
	if !q.Invoiceable() {
		return invoice.Invoice{}, quote.ErrBadTransition
	}
	draft := q.ToInvoice(a.Today(), 0)
	draft.DueDate = invoice.Date{} // splatnosť podľa kontaktu alebo firmy
	inv, err := a.saveDraft(ctx, companyID, a.withDocExtras(ctx, companyID, draft))
	if err != nil {
		return inv, err
	}
	a.record(ctx, companyID, audit.CatInvoices, "create", "invoice", inv.ID,
		"Koncept faktúry z ponuky "+q.Number+" pre "+inv.Customer.Name, nil, inv)
	before := q.Status
	if err := q.MarkInvoiced(inv.ID, ""); err != nil {
		return inv, err
	}
	if err := a.Store.SaveQuote(ctx, companyID, &q); err != nil {
		return inv, err
	}
	type state struct {
		Status    quote.Status `json:"status"`
		InvoiceID int64        `json:"invoiceId,omitempty"`
	}
	a.record(ctx, companyID, audit.CatQuotes, "invoice", "quote", q.ID, "Vyfakturovaná ponuka "+q.Number,
		state{Status: before}, state{q.Status, inv.ID})
	return inv, nil
}

// InvoiceQuotePart vytvorí čiastkovú faktúru k vystavenej alebo prijatej
// ponuke: množstvá po riadkoch ponuky (0 = riadok nefakturovať). Ponuka si
// zapamätá vyfakturované množstvá; po poslednej časti je vyfakturovaná.
func (a *App) InvoiceQuotePart(ctx context.Context, companyID, id int64, qty []money.Dec) (invoice.Invoice, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return invoice.Invoice{}, err
	}
	q, err := a.Store.GetQuote(ctx, companyID, id)
	if err != nil {
		return invoice.Invoice{}, err
	}
	a.refreshParts(ctx, companyID, &q)
	check := q
	check.Billed = append([]quote.Billing(nil), q.Billed...)
	if err := check.BillPart(-1, qty); err != nil {
		var pe quote.ErrPart
		if errors.As(err, &pe) {
			return invoice.Invoice{}, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "quantity", Message: pe.Error()}}}
		}
		return invoice.Invoice{}, err
	}
	final := check.Status == quote.StatusInvoiced
	draft := q.ToInvoice(a.Today(), 0)
	draft.DueDate = invoice.Date{}
	lines := draft.Lines[:0]
	for i, l := range draft.Lines {
		if i < len(qty) && !qty[i].IsZero() {
			l.Quantity = qty[i]
			lines = append(lines, l)
		}
	}
	draft.Lines = lines
	part := len(q.Billed) + 1
	// „Podľa cenovej ponuky CP… (a objednávky …).“ → „Čiastková faktúra č. 2 podľa cenovej ponuky CP…“
	label := fmt.Sprintf("Čiastková faktúra č. %d podľa cenovej ponuky %s", part, q.Number)
	if final {
		label = fmt.Sprintf("Záverečná faktúra (časť %d) podľa cenovej ponuky %s", part, q.Number)
	}
	if ref := "Podľa cenovej ponuky " + q.Number; strings.Contains(draft.Note, ref) {
		draft.Note = strings.Replace(draft.Note, ref, label, 1)
	} else {
		draft.Note = strings.TrimSpace(label + ".\n" + draft.Note)
	}
	inv, err := a.saveDraft(ctx, companyID, a.withDocExtras(ctx, companyID, draft))
	if err != nil {
		return inv, err
	}
	a.record(ctx, companyID, audit.CatInvoices, "create", "invoice", inv.ID, fmt.Sprintf("Koncept čiastkovej faktúry č. %d z ponuky %s pre %s", part, q.Number, inv.Customer.Name), nil, inv)
	before := q.Status
	if err := q.BillPart(inv.ID, qty); err != nil {
		return inv, err
	}
	q.Remaining = nil
	if err := a.Store.SaveQuote(ctx, companyID, &q); err != nil {
		return inv, err
	}
	type state struct {
		Status quote.Status `json:"status"`
		Parts  int          `json:"parts"`
	}
	a.record(ctx, companyID, audit.CatQuotes, "invoice", "quote", q.ID, fmt.Sprintf("Čiastková faktúra č. %d k ponuke %s", part, q.Number),
		state{before, part - 1}, state{q.Status, part})
	if l, err := link.New(link.Ref{Kind: link.Quote, ID: q.ID}, link.Ref{Kind: link.Invoice, ID: inv.ID}); err == nil {
		if err := a.Store.SaveDocLink(ctx, companyID, &l); err != nil && !errors.Is(err, store.ErrDuplicate) {
			return inv, err
		}
	}
	return inv, nil
}

// RenumberQuote zmení číslo vystavenej ponuky (napríklad po prenose
// z iného programu). Ponuka nie je daňový doklad; číslo musí byť
// jedinečné a zmena sa zapíše do auditu.
func (a *App) RenumberQuote(ctx context.Context, companyID, id int64, number string) (quote.Quote, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return quote.Quote{}, err
	}
	q, err := a.Store.GetQuote(ctx, companyID, id)
	if err != nil {
		return q, err
	}
	number = strings.TrimSpace(number)
	if q.Status == quote.StatusDraft {
		return q, vErr("number", "Koncept nemá číslo — číslo dostane pri vystavení")
	}
	if number == "" || len([]rune(number)) > 40 {
		return q, vErr("number", "Zadajte číslo ponuky (najviac 40 znakov)")
	}
	if number == q.Number {
		return q, nil
	}
	all, err := a.Store.ListQuotes(ctx, companyID, 0)
	if err != nil {
		return q, err
	}
	for _, x := range all {
		if x.ID != q.ID && x.Number == number {
			return q, vErr("number", "Číslo "+number+" už má iná ponuka")
		}
	}
	before := q
	q.Number = number
	if err := a.Store.SaveQuote(ctx, companyID, &q); err != nil {
		return q, err
	}
	a.record(ctx, companyID, audit.CatQuotes, "renumber", "quote", q.ID,
		"Zmena čísla ponuky "+before.Number+" → "+q.Number+" ("+q.Customer.Name+")", before, q)
	return q, nil
}

// amendQuote uloží úpravu vystavenej ponuky: číslo, stav, faktúra a odkaz
// pre zákazníka ostanú; ponuka musí byť úplná ako pri vystavení.
func (a *App) amendQuote(ctx context.Context, companyID int64, old, q quote.Quote) (quote.Quote, error) {
	q.ID, q.Status, q.Number, q.InvoiceID, q.InvoiceNumber, q.Share = old.ID, old.Status, old.Number, old.InvoiceID, old.InvoiceNumber, old.Share
	if q.IssueDate.IsZero() {
		q.IssueDate = old.IssueDate
	}
	if err := a.prepareQuote(ctx, companyID, &q); err != nil {
		return q, err
	}
	if err := q.Validate(); err != nil {
		return q, err
	}
	if err := a.Store.SaveQuote(ctx, companyID, &q); err != nil {
		return q, err
	}
	a.record(ctx, companyID, audit.CatQuotes, "amend", "quote", q.ID,
		"Úprava vystavenej ponuky "+q.Number+" pre "+q.Customer.Name+", "+q.Summary.Total.Display()+" €", old, q)
	return q, nil
}
