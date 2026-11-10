package app

import (
	"strings"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/link"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/quote"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

func TestQuoteParts(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	m := money.MustParse
	d := invoice.MustDate
	qd, err := a.SaveQuote(ctx, c.ID, quote.Quote{Customer: invoice.Party{Name: "Agro Liptov družstvo", Country: "SK"}, IssueDate: d("2026-10-01"),
		ValidUntil: d("2026-10-31"), Currency: "EUR", Lines: []invoice.Line{
			{Description: "Rozvádzač RM1", Quantity: m("1"), Unit: "ks", UnitPrice: m("4200"), Rate: vat.Rate23},
			{Description: "Montáž", Quantity: m("40"), Unit: "hod", UnitPrice: m("35"), Rate: vat.Rate23},
		}})
	if err != nil {
		t.Fatal(err)
	}
	q, _ := a.IssueQuote(ctx, c.ID, qd.ID)
	ms := func(xs ...string) []money.Dec {
		out := make([]money.Dec, len(xs))
		for i, x := range xs {
			out[i] = m(x)
		}
		return out
	}
	if _, err := a.InvoiceQuotePart(ctx, c.ID, q.ID, ms("0", "41")); !fieldErr(err, "quantity") {
		t.Fatalf("viac ako ponuka: %v", err)
	}
	// 1. časť: polovica
	inv1, err := a.InvoiceQuotePart(ctx, c.ID, q.ID, ms("0.5", "20"))
	if err != nil {
		t.Fatal(err)
	}
	if len(inv1.Lines) != 2 || inv1.Lines[0].Quantity.String() != "0.50" || inv1.Summary.Base.String() != "2800.00" ||
		!strings.HasPrefix(inv1.Note, "Čiastková faktúra č. 1 podľa cenovej ponuky "+q.Number) {
		t.Fatalf("1. časť %+v", inv1)
	}
	q, _ = a.Quote(ctx, c.ID, q.ID)
	if q.Status != quote.StatusIssued || len(q.Billed) != 1 || q.Remaining[1].String() != "20.00" || q.Amendable() || q.Billed[0].InvoiceNumber != "koncept" {
		t.Fatalf("po 1. časti %+v", q)
	}
	if docs, _ := a.DocLinks(ctx, c.ID, link.Ref{Kind: link.Quote, ID: q.ID}); len(docs) != 1 || docs[0].ID != inv1.ID {
		t.Fatalf("prepojenie %+v", docs)
	}
	// 2. časť: iba montáž
	inv2, err := a.InvoiceQuotePart(ctx, c.ID, q.ID, ms("0", "8"))
	if err != nil || len(inv2.Lines) != 1 || inv2.Lines[0].Description != "Montáž" {
		t.Fatalf("2. časť %+v %v", inv2, err)
	}
	// zmazaný koncept 2. časti sa uvoľní
	if err := a.DeleteInvoice(ctx, c.ID, inv2.ID); err != nil {
		t.Fatal(err)
	}
	q, _ = a.Quote(ctx, c.ID, q.ID)
	if len(q.Billed) != 1 || q.Remaining[1].String() != "20.00" {
		t.Fatalf("po zmazaní konceptu %+v", q)
	}
	// Vytvoriť faktúru = zvyšok (záverečná)
	inv3, err := a.InvoiceQuote(ctx, c.ID, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if inv3.Summary.Base.String() != "2800.00" || !strings.HasPrefix(inv3.Note, "Záverečná faktúra (časť 2)") {
		t.Fatalf("záverečná %+v", inv3)
	}
	q, _ = a.Quote(ctx, c.ID, q.ID)
	if q.Status != quote.StatusInvoiced || q.InvoiceID != inv3.ID {
		t.Fatalf("vyfakturovaná %+v", q)
	}
	if _, err := a.InvoiceQuotePart(ctx, c.ID, q.ID, ms("0", "1")); err == nil {
		t.Fatal("vyfakturovaná ponuka ďalšiu časť nemá")
	}
	// storno 1. časti vráti ponuku medzi prijaté so zostatkom
	is1, err := a.Issue(ctx, c.ID, inv1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Cancel(ctx, c.ID, is1.ID); err != nil {
		t.Fatal(err)
	}
	q, _ = a.Quote(ctx, c.ID, q.ID)
	if q.Status != quote.StatusAccepted || len(q.Billed) != 1 || q.Remaining[0].String() != "0.50" {
		t.Fatalf("po storne %+v", q)
	}
}
