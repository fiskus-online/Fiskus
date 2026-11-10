package app

import (
	"errors"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/forms"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/link"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/quote"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func kinds(ds []LinkedDoc) map[link.Ref]LinkedDoc {
	out := map[link.Ref]LinkedDoc{}
	for _, d := range ds {
		out[d.Ref] = d
	}
	return out
}

func TestDocLinks(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	draft, _ := a.SaveDraft(ctx, c.ID, validDraft())
	inv, err := a.Issue(ctx, c.ID, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	cn, err := a.CreateCreditNote(ctx, c.ID, inv.ID)
	if err != nil {
		t.Fatal(err)
	}
	qd, _ := a.SaveQuote(ctx, c.ID, quote.Quote{Customer: inv.Customer, IssueDate: invoice.MustDate("2026-10-01"), ValidUntil: invoice.MustDate("2026-10-31"),
		Currency: "EUR", Lines: inv.Lines})
	q, _ := a.IssueQuote(ctx, c.ID, qd.ID)
	dl, err := a.SaveFormDoc(ctx, c.ID, forms.Doc{FormKey: "dodaci-list", Values: forms.Values{"number": "DL-7", "date": "2026-10-02",
		"issuer": "Moja firma", "partner": "Odberateľ s.r.o.\nHlavná 1"}, Items: []forms.Item{{Desc: "Kábel", Qty: money.FromInt(10), Unit: "m", Price: money.FromInt(2)}}})
	if err != nil {
		t.Fatal(err)
	}
	invRef, qRef, dlRef := link.Ref{Kind: link.Invoice, ID: inv.ID}, link.Ref{Kind: link.Quote, ID: q.ID}, link.Ref{Kind: link.Form, ID: dl.ID}

	// ručné prepojenia
	got, err := a.LinkDocs(ctx, c.ID, invRef, dlRef)
	if err != nil {
		t.Fatal(err)
	}
	m := kinds(got)
	if d := m[dlRef]; d.LinkID == 0 || d.Type != "Dodací list" || d.Number != "DL-7" || d.Partner != "Odberateľ s.r.o." || d.Date.String() != "2026-10-02" {
		t.Fatalf("dodací list: %+v", d)
	}
	if d := m[link.Ref{Kind: link.Invoice, ID: cn.ID}]; d.Relation != "dobropis" || d.LinkID != 0 || d.Type != "Dobropis" {
		t.Fatalf("dobropis vyplýva z dokladu: %+v", d)
	}
	if _, err := a.LinkDocs(ctx, c.ID, dlRef, invRef); !fieldErr(err, "doc") {
		t.Fatalf("rovnaká dvojica opačne: %v", err)
	}
	if _, err := a.LinkDocs(ctx, c.ID, invRef, invRef); !fieldErr(err, "doc") {
		t.Fatalf("sám so sebou: %v", err)
	}
	if _, err := a.LinkDocs(ctx, c.ID, invRef, link.Ref{Kind: link.Quote, ID: 999}); !fieldErr(err, "doc") {
		t.Fatalf("neexistujúci: %v", err)
	}
	// z druhej strany je to isté prepojenie
	back, _ := a.DocLinks(ctx, c.ID, dlRef)
	if len(back) != 1 || back[0].Ref != invRef || back[0].Number != inv.Number {
		t.Fatalf("z dodacieho listu: %+v", back)
	}

	// ponuka vyfakturovaná existujúcou faktúrou → väzba vyplýva z ponuky
	if _, err := a.QuoteInvoicedBy(ctx, c.ID, q.ID, draft.ID+999); !fieldErr(err, "invoiceId") {
		t.Fatalf("neexistujúca faktúra: %v", err)
	}
	if _, err := a.QuoteInvoicedBy(ctx, c.ID, q.ID, cn.ID); !fieldErr(err, "invoiceId") {
		t.Fatalf("dobropis: %v", err)
	}
	q, err = a.QuoteInvoicedBy(ctx, c.ID, q.ID, inv.ID)
	if err != nil || q.Status != quote.StatusInvoiced || q.InvoiceID != inv.ID || q.InvoiceNumber != inv.Number {
		t.Fatalf("vyfakturovaná: %+v %v", q, err)
	}
	if _, err := a.QuoteInvoicedBy(ctx, c.ID, q.ID, inv.ID); !errors.Is(err, quote.ErrBadTransition) {
		t.Fatalf("druhýkrát: %v", err)
	}
	m = kinds(must(a.DocLinks(ctx, c.ID, invRef)))
	if m[qRef].Relation != "ponuka vyfakturovaná touto faktúrou" || len(m) != 3 {
		t.Fatalf("z faktúry: %+v", m)
	}
	// ručné prepojenie, ktoré už vyplýva z dokladu, sa neukáže dvakrát
	if got, err := a.LinkDocs(ctx, c.ID, qRef, invRef); err != nil || len(got) != 1 || got[0].LinkID != 0 {
		t.Fatalf("duplicitné s vyplývajúcim: %+v %v", got, err)
	}

	// zrušenie: iba ručné a iba z vlastného dokladu
	dlLink := kinds(must(a.DocLinks(ctx, c.ID, invRef)))[dlRef].LinkID
	if _, err := a.UnlinkDocs(ctx, c.ID, qRef, dlLink); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cudzie prepojenie: %v", err)
	}
	if got, err := a.UnlinkDocs(ctx, c.ID, dlRef, dlLink); err != nil || len(got) != 0 {
		t.Fatalf("zrušenie: %+v %v", got, err)
	}

	// zálohová faktúra a vyúčtovanie
	pd := validDraft()
	pd.Kind = invoice.KindProforma
	pdr, _ := a.SaveDraft(ctx, c.ID, pd)
	pro, err := a.Issue(ctx, c.ID, pdr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.LinkDocs(ctx, c.ID, link.Ref{Kind: link.Invoice, ID: pro.ID}, dlRef); err != nil {
		t.Fatal(err)
	}
}

func TestDocLinksPerms(t *testing.T) {
	o := newOffice(t)
	actx := as(o.admin)
	draft, err := o.a.SaveDraft(actx, o.alfa.ID, validDraft())
	if err != nil {
		t.Fatal(err)
	}
	inv, _ := o.a.Issue(actx, o.alfa.ID, draft.ID)
	p, _, err := o.a.SavePurchase(actx, o.alfa.ID, validPurchase())
	if err != nil {
		t.Fatal(err)
	}
	invRef, pRef := link.Ref{Kind: link.Invoice, ID: inv.ID}, link.Ref{Kind: link.Purchase, ID: p.ID}
	if _, err := o.a.LinkDocs(actx, o.alfa.ID, invRef, pRef); err != nil {
		t.Fatal(err)
	}
	mb, err := o.a.AddMember(actx, o.alfa.ID, NewMember{Email: "f@alfa.sk", Name: "F", Password: "heslo-1234567", Role: auth.RoleInvoicer})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := o.a.Store.GetUser(ctx, mb.User.ID)
	fctx := as(actorOf(u, auth.Actor{Channel: auth.ChannelWeb}))
	// fakturant nevidí prijaté faktúry: prepojenie sa mu neukáže
	if got, err := o.a.DocLinks(fctx, o.alfa.ID, invRef); err != nil || len(got) != 0 {
		t.Fatalf("fakturant: %+v %v", got, err)
	}
	if _, err := o.a.DocLinks(fctx, o.alfa.ID, pRef); !errors.Is(err, ErrForbidden) {
		t.Fatalf("fakturant číta prijatú faktúru: %v", err)
	}
	if _, err := o.a.LinkDocs(fctx, o.alfa.ID, invRef, pRef); !errors.Is(err, ErrForbidden) {
		t.Fatalf("fakturant prepojí prijatú faktúru: %v", err)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
