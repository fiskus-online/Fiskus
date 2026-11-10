package app

import (
	"errors"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/job"
	"github.com/fiskus-online/fiskus/backend/internal/link"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/quote"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func TestCrmJobs(t *testing.T) {
	a, c := newApp(t, edition.Desktop) // platiteľ DPH: sumy bez DPH
	ct, _ := a.SaveContact(ctx, c.ID, store.Contact{Name: "FARMOL SLOVAKIA, s.r.o.", ICO: "50473484"})

	if _, err := a.SaveCrmJob(ctx, c.ID, job.Job{Name: " "}); !fieldErr(err, "name") {
		t.Fatalf("bez názvu: %v", err)
	}
	if _, err := a.SaveCrmJob(ctx, c.ID, job.Job{Name: "x", ContactID: 999}); !fieldErr(err, "contactId") {
		t.Fatalf("neznámy kontakt: %v", err)
	}
	v, err := a.SaveCrmJob(ctx, c.ID, job.Job{Name: "Rekonštrukcia RS etiketovačky", ContactID: ct.ID, Status: job.Active,
		Due: invoice.MustDate("2026-10-20"), Number: "X", Closed: invoice.MustDate("2020-01-01")})
	if err != nil {
		t.Fatal(err)
	}
	if v.Number != "Z2026001" || v.Customer != ct.Name || !v.Closed.IsZero() || v.Created.String() != "2026-10-29" || !v.Late {
		t.Fatalf("nová zákazka: %+v", v)
	}
	v2, _ := a.SaveCrmJob(ctx, c.ID, job.Job{Name: "Servis"})
	if v2.Number != "Z2026002" || v2.Status != job.Lead {
		t.Fatalf("druhá: %+v", v2)
	}
	jref := link.Ref{Kind: link.Job, ID: v.ID}

	// doklady: ponuka 1000 + DPH, faktúra 400 + DPH (uhradená 246), dobropis −100 + DPH, náklady 81,30 + DPH
	party := invoice.Party{Name: ct.Name, ICO: ct.ICO}
	lines := func(price string) []invoice.Line {
		l := validDraft().Lines[0]
		l.Quantity, l.UnitPrice = money.FromInt(1), money.MustParse(price)
		return []invoice.Line{l}
	}
	qd, _ := a.SaveQuote(ctx, c.ID, quote.Quote{Customer: party, IssueDate: invoice.MustDate("2026-10-01"), ValidUntil: invoice.MustDate("2026-10-31"), Currency: "EUR", Lines: lines("1000")})
	q, _ := a.IssueQuote(ctx, c.ID, qd.ID)
	d := validDraft()
	d.Customer, d.Lines = party, lines("400")
	dr, _ := a.SaveDraft(ctx, c.ID, d)
	inv, err := a.Issue(ctx, c.ID, dr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddPayment(ctx, c.ID, inv.ID, invoice.Payment{Date: invoice.MustDate("2026-10-25"), Amount: money.MustParse("246")}, false); err != nil {
		t.Fatal(err)
	}
	cnd, _ := a.CreateCreditNote(ctx, c.ID, inv.ID)
	cnd.Lines = lines("-100")
	cnd.Correction.Reason = "Zľava"
	cnd, _ = a.SaveDraft(ctx, c.ID, cnd)
	cn, err := a.Issue(ctx, c.ID, cnd.ID)
	if err != nil {
		t.Fatal(err)
	}
	p, _, _ := a.SavePurchase(ctx, c.ID, validPurchase())
	draftInv, _ := a.SaveDraft(ctx, c.ID, validDraft()) // koncept sa nepočíta
	for _, r := range []link.Ref{{Kind: link.Quote, ID: q.ID}, {Kind: link.Invoice, ID: inv.ID}, {Kind: link.Invoice, ID: cn.ID},
		{Kind: link.Purchase, ID: p.ID}, {Kind: link.Invoice, ID: draftInv.ID}} {
		if _, err := a.LinkDocs(ctx, c.ID, jref, r); err != nil {
			t.Fatal(err)
		}
	}
	got, err := a.CrmJob(ctx, c.ID, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	s := got.Summary
	if !s.Net || s.Offered.String() != "1000.00" || s.Invoiced.String() != "300.00" || s.Gross.String() != "369.00" ||
		s.Paid.String() != "246.00" || s.Unpaid.String() != "123.00" || s.Costs.String() != "81.30" || s.Profit.String() != "218.70" ||
		s.Margin == nil || *s.Margin != 73 || s.Docs != 5 || len(got.Docs) != 5 {
		t.Fatalf("ekonomika: %+v (%d dokladov)", s, len(got.Docs))
	}
	// z faktúry je vidno zákazku
	if ds, _ := a.DocLinks(ctx, c.ID, link.Ref{Kind: link.Invoice, ID: inv.ID}); len(ds) < 1 || kinds(ds)[jref].Type != "Zákazka" {
		t.Fatalf("zákazka z faktúry: %+v", ds)
	}

	// dokončenie zapíše dátum, číslo a vznik sa nemenia
	upd := got.Job
	upd.Status, upd.Number, upd.Name = job.Done, "Z1", "Rekonštrukcia RS"
	got, err = a.SaveCrmJob(ctx, c.ID, upd)
	if err != nil || got.Number != "Z2026001" || got.Closed.String() != "2026-10-29" || got.Late {
		t.Fatalf("dokončená: %+v %v", got, err)
	}
	list, _ := a.CrmJobs(ctx, c.ID)
	if len(list) != 2 || list[1].Summary.Invoiced.String() != "300.00" || list[0].Summary.Margin != nil {
		t.Fatalf("zoznam: %+v", list)
	}

	// zmazanie: prepojenia preč, doklady ostanú
	if err := a.DeleteCrmJob(ctx, c.ID, v.ID); err != nil {
		t.Fatal(err)
	}
	if links, _ := a.Store.ListDocLinks(ctx, c.ID, link.Ref{Kind: link.Invoice, ID: inv.ID}); len(links) != 0 {
		t.Fatalf("prepojenia po zmazaní: %+v", links)
	}
	if _, err := a.Store.GetInvoice(ctx, c.ID, inv.ID); err != nil {
		t.Fatal("faktúra zmizla")
	}
	if _, err := a.CrmJob(ctx, c.ID, v.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("po zmazaní: %v", err)
	}
}

func TestCrmJobsNonPayer(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	np, _ := a.CreateCompany(ctx, store.Company{Name: "Živnostník", ICO: "43352952"})
	v, _ := a.SaveCrmJob(ctx, np.ID, job.Job{Name: "Servis"})
	d := validDraft()
	dr, _ := a.SaveDraft(ctx, np.ID, d)
	inv, err := a.Issue(ctx, np.ID, dr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.LinkDocs(ctx, np.ID, link.Ref{Kind: link.Job, ID: v.ID}, link.Ref{Kind: link.Invoice, ID: inv.ID}); err != nil {
		t.Fatal(err)
	}
	got, _ := a.CrmJob(ctx, np.ID, v.ID)
	if got.Summary.Net || got.Summary.Invoiced.String() != "400.00" || got.Summary.Gross.String() != "400.00" || *got.Summary.Margin != 100 {
		t.Fatalf("neplatiteľ: %+v", got.Summary)
	}
}
