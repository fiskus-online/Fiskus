package app

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/quote"
)

// export „Zoznam cenových ponúk“ (nadpis nad tabuľkou, popis na viac riadkov)
const quotesCSV = "Zoznam cenových ponúk\n" +
	"Exportované: 10. 11. 2026 9:50:55\n" +
	"\n" +
	"Číslo cenovej ponuky;Popis;Názov/Meno;IČO;IČ DPH;DIČ;Vystavenie;Platnosť do;Spolu;Mena;Stav\n" +
	"CP20260011;Servis riadiaceho systému;FARMOL SLOVAKIA, s.r.o.;50473484;SK2120345150;2120345150;27.10.2026;10.11.2026;220;EUR;Vyfakturovaná\n" +
	"CP20260009;\"Riadiaci systém SLES.\nRozsah dodávky:\n- programové vybavenie\";FARMOL SLOVAKIA, s.r.o.;50473484;SK2120345150;2120345150;23.10.2026;6.11.2026;6740;EUR;Vytvorená\n" +
	"CP20260002;Výmena meniča;Knauf Insulation, s.r.o.;31628109;SK2020478757;2020478757;10.8.2026;24.8.2026;1660;EUR;Odmietnutá\n" +
	"CP20260012;Bez dátumu;Firma;;;;;;10;EUR;Vytvorená\n"

func TestImportQuotes(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	// faktúra k vyfakturovanej ponuke (rovnaký odberateľ a suma, neskôr)
	inv := validDraft()
	inv.Customer = invoice.Party{Name: "FARMOL SLOVAKIA, s.r.o.", ICO: "50473484", Street: "G. Schoellera 3", City: "Levice", Zip: "934 01", Country: "SK"}
	inv.IssueDate, inv.DeliveryDate, inv.DueDate = invoice.MustDate("2026-10-28"), invoice.MustDate("2026-10-28"), invoice.MustDate("2026-11-11")
	inv.Lines = []invoice.Line{{Description: "Servis", Quantity: money.FromInt(1), Unit: "ks", UnitPrice: money.MustParse("178.86"), Rate: 23}}
	saved, err := a.SaveDraft(ctx, c.ID, inv)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := a.Issue(ctx, c.ID, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if issued.Summary.Total.String() != "220.00" {
		t.Fatalf("suma faktúry %s", issued.Summary.Total)
	}

	preview, err := a.ImportTable(ctx, c.ID, []byte(quotesCSV), MigrateOptions{}, false)
	if err != nil || preview.Kind != "quotes" || preview.Quotes != (ImportCount{New: 3, Skipped: 1}) || preview.Contacts.New != 2 {
		t.Fatalf("náhľad: %+v %v", preview, err)
	}
	if qs, _ := a.Store.ListQuotes(ctx, c.ID, 0); len(qs) != 0 {
		t.Fatal("náhľad zapísal")
	}
	res, err := a.ImportTable(ctx, c.ID, []byte(quotesCSV), MigrateOptions{}, true)
	if err != nil || res.Quotes.New != 3 {
		t.Fatalf("prenos: %+v %v", res, err)
	}
	qs, _ := a.Store.ListQuotes(ctx, c.ID, 0)
	got := map[string]quote.Quote{}
	for _, q := range qs {
		got[q.Number] = q
	}
	q11, q9, q2 := got["CP20260011"], got["CP20260009"], got["CP20260002"]
	if q11.Status != quote.StatusInvoiced || q11.InvoiceID != issued.ID || q11.Summary.Total.String() != "220.00" {
		t.Fatalf("vyfakturovaná: %+v", q11)
	}
	if q9.Status != quote.StatusIssued || q9.Subject != "Riadiaci systém SLES." || q9.Intro != "Rozsah dodávky:\n- programové vybavenie" ||
		q9.ValidUntil.String() != "2026-11-06" || q9.ContactID == 0 || q9.InvoiceID != 0 {
		t.Fatalf("vytvorená: %+v", q9)
	}
	if q2.Status != quote.StatusRejected || q2.Customer.ICDPH != "SK2020478757" {
		t.Fatalf("odmietnutá: %+v", q2)
	}
	contacts, _ := a.Store.ListContacts(ctx, c.ID, "")
	if len(contacts) != 2 {
		t.Fatalf("adresár: %d", len(contacts))
	}

	// opakovaný prenos nič nezdvojí
	again, err := a.ImportTable(ctx, c.ID, []byte(quotesCSV), MigrateOptions{}, true)
	if err != nil || again.Quotes.Existing != 3 || again.Quotes.New != 0 {
		t.Fatalf("znova: %+v %v", again, err)
	}

	// nová ponuka v 2026 preskočí čísla prenesených (CP20260002 je obsadené)
	for range 2 {
		draft, err := a.SaveQuote(ctx, c.ID, quote.Quote{Customer: inv.Customer, IssueDate: invoice.MustDate("2026-10-29"),
			ValidUntil: invoice.MustDate("2026-11-29"), Currency: "EUR", Lines: inv.Lines})
		if err != nil {
			t.Fatal(err)
		}
		out, err := a.IssueQuote(ctx, c.ID, draft.ID)
		if err != nil {
			t.Fatal(err)
		}
		if out.Number == "CP20260002" || out.Number == "CP20260009" || out.Number == "CP20260011" {
			t.Fatalf("obsadené číslo %s", out.Number)
		}
	}
	all, _ := a.Store.ListQuotes(ctx, c.ID, 0)
	seen := map[string]bool{}
	for _, q := range all {
		if seen[q.Number] {
			t.Fatalf("dve ponuky %s", q.Number)
		}
		seen[q.Number] = true
	}
	if !seen["CP20260001"] || !seen["CP20260003"] {
		t.Fatalf("čísla: %v", seen)
	}
}

func TestQuoteStatusFrom(t *testing.T) {
	for in, want := range map[string]quote.Status{
		"Vyfakturovaná": quote.StatusInvoiced, "Vytvorená": quote.StatusIssued, "Odoslaná": quote.StatusIssued,
		"Prijatá": quote.StatusAccepted, "Akceptovaná": quote.StatusAccepted, "Odmietnutá": quote.StatusRejected,
		"Zamítnutá": quote.StatusRejected, "": quote.StatusIssued,
	} {
		if got := quoteStatusFrom(in); got != want {
			t.Errorf("%q → %s", in, got)
		}
	}
}
