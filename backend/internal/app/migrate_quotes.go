package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/quote"
	"github.com/fiskus-online/fiskus/backend/internal/store"
	"github.com/fiskus-online/fiskus/backend/internal/tabimport"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

// Prenos cenových ponúk z iného programu: ponuka si ponechá svoje číslo a
// stav. Vyfakturovaná ponuka sa spojí s faktúrou, ak ju vo Fiškuse nájde
// (rovnaký odberateľ a suma, faktúra nie staršia ako ponuka).

// migQuote je cenová ponuka zo zdroja.
type migQuote struct {
	Number string
	Ref    string // riadok v súbore (pre hlásenie)
	Status quote.Status
	Q      quote.Quote
	Err    error
}

// quoteStatusFrom prevedie stav zo zdroja; neznámy stav = vystavená.
func quoteStatusFrom(s string) quote.Status {
	k := strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.HasPrefix(k, "vyfakturovan"), strings.HasPrefix(k, "fakturovan"), k == "invoiced":
		return quote.StatusInvoiced
	case strings.HasPrefix(k, "prijat"), strings.HasPrefix(k, "akceptovan"), strings.HasPrefix(k, "schvalen"), k == "accepted":
		return quote.StatusAccepted
	case strings.HasPrefix(k, "odmietnut"), strings.HasPrefix(k, "zamietnut"), strings.HasPrefix(k, "odmítnut"), strings.HasPrefix(k, "zamítnut"), k == "rejected":
		return quote.StatusRejected
	}
	return quote.StatusIssued
}

// tableQuote prevedie riadok tabuľky na ponuku: popis = predmet (prvý
// riadok) a úvodný text (ďalšie riadky), suma = jedna položka.
func tableQuote(d tabimport.Doc, p invoice.Party, err error) migQuote {
	mq := migQuote{Number: strings.TrimSpace(d.Number), Ref: fmt.Sprintf("riadok %d", d.Line), Status: quoteStatusFrom(d.Status), Err: err}
	if err != nil {
		return mq
	}
	if mq.Number == "" {
		mq.Err = errors.New("chýba číslo ponuky")
		return mq
	}
	subject, intro, _ := strings.Cut(strings.TrimSpace(strings.ReplaceAll(d.Subject, "\r\n", "\n")), "\n")
	subject = strings.TrimSpace(subject)
	if subject == "" {
		subject = "Cenová ponuka " + mq.Number
	}
	if r := []rune(subject); len(r) > 200 {
		subject = string(r[:200])
	}
	valid := d.Valid
	if valid.IsZero() {
		valid = d.Issue.AddDays(quote.DefaultValidDays)
	}
	mq.Q = quote.Quote{Customer: p, IssueDate: d.Issue, ValidUntil: valid, Currency: "EUR", PriceMode: vat.PriceNet,
		Subject: subject, Intro: strings.TrimSpace(intro)}
	for _, am := range d.Amounts {
		mq.Q.Lines = append(mq.Q.Lines, invoice.Line{Description: subject, Quantity: money.FromInt(1), Unit: "ks",
			UnitPrice: am.Base.RoundCents(), Rate: am.Rate})
	}
	return mq
}

// migQuotes zapíše cenové ponuky (podľa čísla; existujúce preskočí).
func (a *App) migQuotes(ctx context.Context, companyID int64, m migration, apply bool, rep *ImportReport) error {
	if len(m.Quotes) == 0 {
		return nil
	}
	existing, err := a.Store.ListQuotes(ctx, companyID, 0)
	if err != nil {
		return err
	}
	numbers := map[string]bool{}
	linked := map[int64]bool{}
	for _, q := range existing {
		if q.Number != "" {
			numbers[q.Number] = true
		}
		if q.InvoiceID != 0 {
			linked[q.InvoiceID] = true
		}
	}
	contacts, err := a.Store.ListContacts(ctx, companyID, "")
	if err != nil {
		return err
	}
	byICO := map[string]int64{}
	for _, c := range contacts {
		if ico := strings.ReplaceAll(c.ICO, " ", ""); ico != "" {
			byICO[ico] = c.ID
		}
	}
	invs, err := a.Store.ListInvoices(ctx, companyID, store.InvoiceFilter{})
	if err != nil {
		return err
	}
	// najstaršie ponuky prvé (párovanie s faktúrami v poradí)
	list := append([]migQuote(nil), m.Quotes...)
	sort.SliceStable(list, func(i, j int) bool { return list[i].Q.IssueDate.Before(list[j].Q.IssueDate) })
	for _, mq := range list {
		if mq.Err != nil {
			rep.Quotes.Skipped++
			rep.warn("Cenová ponuka %s: %v", firstNonBlank(mq.Number, mq.Ref), mq.Err)
			continue
		}
		if numbers[mq.Number] {
			rep.Quotes.Existing++
			continue
		}
		q := mq.Q
		q.ContactID = byICO[strings.ReplaceAll(q.Customer.ICO, " ", "")]
		if err := a.prepareQuote(ctx, companyID, &q); err == nil {
			err = q.Validate()
		}
		if err != nil {
			rep.Quotes.Skipped++
			rep.warn("Cenová ponuka %s: %s", mq.Number, errText(err))
			continue
		}
		q.Number, q.Status = mq.Number, mq.Status
		if q.Status == quote.StatusInvoiced {
			if inv := quoteInvoice(q, invs, linked); inv != nil {
				q.InvoiceID = inv.ID
				linked[inv.ID] = true
			}
		}
		numbers[q.Number] = true
		rep.Quotes.New++
		if !apply {
			continue
		}
		if err := a.Store.SaveQuote(ctx, companyID, &q); err != nil {
			rep.Quotes.New--
			rep.Quotes.Skipped++
			rep.warn("Cenová ponuka %s: %v", q.Number, err)
			continue
		}
		a.record(ctx, companyID, audit.CatQuotes, "import", "quote", q.ID, "Prenesená ponuka "+q.Number+" z "+m.Source, nil, nil)
	}
	return nil
}

// quoteInvoice nájde faktúru k vyfakturovanej ponuke: rovnaký odberateľ
// (IČO, inak názov) a suma, vystavená v deň ponuky alebo neskôr; najbližšia.
func quoteInvoice(q quote.Quote, invs []invoice.Invoice, linked map[int64]bool) *invoice.Invoice {
	var best *invoice.Invoice
	for i := range invs {
		inv := &invs[i]
		if linked[inv.ID] || inv.Kind != invoice.KindInvoice || inv.Number == "" || inv.IssueDate.Before(q.IssueDate) ||
			inv.Summary.Total.Cmp(q.Summary.Total) != 0 {
			continue
		}
		same := q.Customer.ICO != "" && strings.ReplaceAll(inv.Customer.ICO, " ", "") == strings.ReplaceAll(q.Customer.ICO, " ", "")
		if q.Customer.ICO == "" {
			same = store.SearchKey(inv.Customer.Name) == store.SearchKey(q.Customer.Name)
		}
		if same && (best == nil || inv.IssueDate.Before(best.IssueDate)) {
			best = inv
		}
	}
	return best
}

// errText: chyba kontroly ponuky ako text (prvé pole).
func errText(err error) string {
	var ve *invoice.ValidationError
	if errors.As(err, &ve) && len(ve.Fields) > 0 {
		return ve.Fields[0].Message
	}
	return err.Error()
}
