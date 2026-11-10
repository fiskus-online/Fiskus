package app

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/catalog"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/job"
	"github.com/fiskus-online/fiskus/backend/internal/link"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/stock"
	"github.com/fiskus-online/fiskus/backend/internal/store"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

func TestJobWorkAndMaterial(t *testing.T) {
	a, c := newApp(t, edition.Desktop) // platiteľ DPH
	m := money.MustParse
	d := invoice.MustDate
	ct, _ := a.SaveContact(ctx, c.ID, store.Contact{Name: "Agro Liptov družstvo", ICO: "00612341"})
	kabel, _ := a.SaveItem(ctx, c.ID, catalog.Item{Code: "CYKY", Name: "Kábel CYKY 3x2,5", Unit: "m", Price: m("1.60"), PriceMode: vat.PriceNet, Rate: vat.Rate23})
	if _, err := a.SaveStockConfig(ctx, c.ID, StockConfig{Warehouses: []Warehouse{{Code: "HL", Name: "Hlavný"}},
		Items: []StockItem{{ItemID: kabel.ID, Kind: stock.Material}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveStockDoc(ctx, c.ID, stock.Doc{Kind: stock.Receipt, Date: d("2026-10-01"), Warehouse: "HL",
		Lines: []stock.Line{{ItemID: kabel.ID, Quantity: m("100"), UnitCost: m("0.80")}}}); err != nil {
		t.Fatal(err)
	}

	v, err := a.SaveCrmJob(ctx, c.ID, job.Job{Name: "Rozvádzač sušička", ContactID: ct.ID, Status: job.Active, Rate: m("35"),
		Work: []job.WorkEntry{
			{Date: d("2026-10-20"), Person: "Palo", Description: "Montáž rozvádzača", Hours: m("6"), Rate: m("35")},
			{Date: d("2026-10-21"), Person: "Brigádnik", Description: "Kabeláž", Hours: m("4"), Rate: m("25"), CostRate: m("12")},
		},
		Material: []job.MaterialEntry{
			{Date: d("2026-10-20"), ItemID: kabel.ID, Quantity: m("25"), UnitPrice: m("1.60"), Name: "prepíše sa"},
			{Date: d("2026-10-21"), Name: "Istič B16", Unit: "ks", Quantity: m("3"), UnitPrice: m("9"), UnitCost: m("5.50")},
		}})
	if err != nil {
		t.Fatal(err)
	}
	if v.Work[0].ID == 0 || v.Material[0].Name != "Kábel CYKY 3x2,5" || v.Material[0].Unit != "m" || !v.Material[0].Stock || v.Material[1].Stock {
		t.Fatalf("záznamy %+v %+v", v.Work, v.Material)
	}
	// náklad: kábel 25 × 0,80 = 20 zo skladu, istič 3 × 5,50 = 16,50, brigádnik 4 × 12 = 48
	s := v.Summary
	if v.Material[0].Cost.String() != "20.00" || v.Material[1].Cost.String() != "16.50" || s.MaterialCost.String() != "36.50" ||
		s.WorkCost.String() != "48.00" || s.Costs.String() != "84.50" || s.Hours.String() != "10.00" || s.Unbilled.String() != "377.00" {
		t.Fatalf("ekonomika %+v, materiál %+v", s, v.Material)
	}
	sv, _ := a.Stock(ctx, c.ID)
	if len(sv.Cards) != 1 || sv.Cards[0].Quantity.String() != "75.00" {
		t.Fatalf("sklad po výdaji %+v", sv.Cards)
	}
	var issue stock.Doc
	for _, dd := range sv.Docs {
		if dd.Kind == stock.Issue {
			issue = dd
		}
	}
	if issue.LinkKind != "job" || issue.LinkNumber != v.Number || !issue.Auto {
		t.Fatalf("výdajka %+v", issue)
	}
	if err := a.DeleteStockDoc(ctx, c.ID, issue.ID); !fieldErr(err, "kind") {
		t.Fatalf("výdajka zákazky sa maže v zákazke: %v", err)
	}

	// úprava množstva upraví výdajku (žiadna nová)
	j := v.Job
	j.Material[0].Quantity = m("30")
	if v, err = a.SaveCrmJob(ctx, c.ID, j); err != nil {
		t.Fatal(err)
	}
	sv, _ = a.Stock(ctx, c.ID)
	if sv.Cards[0].Quantity.String() != "70.00" || len(sv.Docs) != 2 {
		t.Fatalf("sklad po úprave %+v, doklady %d", sv.Cards, len(sv.Docs))
	}

	// faktúra zo zákazky
	inv, err := a.InvoiceCrmJob(ctx, c.ID, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Status != invoice.StatusDraft || inv.ContactID != ct.ID || len(inv.Lines) != 4 || inv.DeliveryDate.String() != "2026-10-21" ||
		inv.Lines[0].Unit != "hod" || inv.Lines[0].Description != "Montáž rozvádzača (20. 10. 2026, Palo)" || inv.Lines[2].ItemID != 0 ||
		inv.Lines[2].Quantity.String() != "30.00" || inv.Summary.Base.String() != "385.00" {
		t.Fatalf("faktúra %+v / %+v", inv, inv.Lines)
	}
	v, _ = a.CrmJob(ctx, c.ID, v.ID)
	if !v.Work[0].Billed() || v.Work[0].InvoiceNumber != "koncept" || !v.Summary.Unbilled.IsZero() || len(v.Docs) != 1 {
		t.Fatalf("po fakturácii %+v %+v", v.Work[0], v.Summary)
	}
	if _, err := a.InvoiceCrmJob(ctx, c.ID, v.ID); !fieldErr(err, "work") {
		t.Fatalf("druhá faktúra bez záznamov: %v", err)
	}
	// vyfakturované sa nemení
	j = v.Job
	j.Work[0].Hours = m("8")
	if _, err := a.SaveCrmJob(ctx, c.ID, j); !fieldErr(err, "work[0].description") {
		t.Fatalf("zmena vyfakturovanej práce: %v", err)
	}
	if err := a.DeleteCrmJob(ctx, c.ID, v.ID); !fieldErr(err, "work") {
		t.Fatalf("zmazanie zákazky s faktúrou: %v", err)
	}
	// vystavenie doplní číslo; vystavená faktúra neodpíše sklad druhýkrát
	issued, err := a.Issue(ctx, c.ID, inv.ID)
	if err != nil {
		t.Fatal(err)
	}
	v, _ = a.CrmJob(ctx, c.ID, v.ID)
	sv, _ = a.Stock(ctx, c.ID)
	if v.Work[1].InvoiceNumber != issued.Number || sv.Cards[0].Quantity.String() != "70.00" || v.Summary.Invoiced.String() != "385.00" ||
		v.Summary.Profit.String() != "296.50" {
		t.Fatalf("po vystavení %+v %+v, sklad %s", v.Work[1], v.Summary, sv.Cards[0].Quantity)
	}
	// storno faktúry uvoľní záznamy na novú fakturáciu
	if _, err := a.Cancel(ctx, c.ID, issued.ID); err != nil {
		t.Fatal(err)
	}
	v, _ = a.CrmJob(ctx, c.ID, v.ID)
	if v.Work[0].Billed() || v.Summary.Unbilled.String() != "385.00" {
		t.Fatalf("po storne %+v %+v", v.Work[0], v.Summary)
	}
	// zmazanie zákazky vráti materiál na sklad
	if err := a.DeleteCrmJob(ctx, c.ID, v.ID); err != nil {
		t.Fatal(err)
	}
	sv, _ = a.Stock(ctx, c.ID)
	if sv.Cards[0].Quantity.String() != "100.00" {
		t.Fatalf("sklad po zmazaní zákazky %s", sv.Cards[0].Quantity)
	}
}

func TestJobInvoiceDraftNonPayer(t *testing.T) {
	j := job.Job{Name: "Servis", Number: "Z2026003", Work: []job.WorkEntry{
		{Date: invoice.MustDate("2026-11-02"), Description: "Oprava", Hours: money.MustParse("1.5"), Rate: money.MustParse("30"),
			Billing: job.Billing{InvoiceID: 3}},
		{Date: invoice.MustDate("2026-11-03"), Description: "Kontrola", Hours: money.MustParse("1"), Rate: money.MustParse("30")},
	}}
	inv := jobInvoiceDraft(j, vat.Rate0, invoice.MustDate("2026-11-10"))
	if len(inv.Lines) != 1 || inv.Lines[0].Description != "Kontrola (3. 11. 2026)" || inv.Lines[0].Rate != vat.Rate0 ||
		inv.Subject != "Servis" || inv.Note != "Zákazka Z2026003." || inv.DeliveryDate.String() != "2026-11-03" {
		t.Fatalf("koncept %+v", inv)
	}
}

func TestJobGoodsCost(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	m := money.MustParse
	d := invoice.MustDate
	rele, _ := a.SaveItem(ctx, c.ID, catalog.Item{Code: "REL", Name: "Relé 24 V", Unit: "ks", Price: m("12"), PriceMode: vat.PriceNet, Rate: vat.Rate23})
	if _, err := a.SaveStockConfig(ctx, c.ID, StockConfig{Warehouses: []Warehouse{{Code: "HL", Name: "Hlavný"}}, Items: []StockItem{{ItemID: rele.ID, Kind: stock.Goods}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveStockDoc(ctx, c.ID, stock.Doc{Kind: stock.Receipt, Date: d("2026-10-01"), Warehouse: "HL",
		Lines: []stock.Line{{ItemID: rele.ID, Quantity: m("10"), UnitCost: m("7.50")}}}); err != nil {
		t.Fatal(err)
	}
	v, _ := a.SaveCrmJob(ctx, c.ID, job.Job{Name: "Dodávka relé"})
	dr := validDraft()
	dr.Lines = []invoice.Line{{Description: rele.Name, Quantity: m("4"), Unit: "ks", UnitPrice: m("12"), Rate: vat.Rate23, ItemID: rele.ID}}
	dr, _ = a.SaveDraft(ctx, c.ID, dr)
	inv, err := a.Issue(ctx, c.ID, dr.ID)
	if err != nil {
		t.Fatal(err)
	}
	jref := link.Ref{Kind: link.Job, ID: v.ID}
	if _, err := a.LinkDocs(ctx, c.ID, jref, link.Ref{Kind: link.Invoice, ID: inv.ID}); err != nil {
		t.Fatal(err)
	}
	cn, _ := a.CreateCreditNote(ctx, c.ID, inv.ID)
	cn.Lines = []invoice.Line{{Description: rele.Name, Quantity: m("-1"), Unit: "ks", UnitPrice: m("12"), Rate: vat.Rate23, ItemID: rele.ID}}
	cn.Correction.Reason = "Vrátené relé"
	cn, _ = a.SaveDraft(ctx, c.ID, cn)
	cn, err = a.Issue(ctx, c.ID, cn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.LinkDocs(ctx, c.ID, jref, link.Ref{Kind: link.Invoice, ID: cn.ID}); err != nil {
		t.Fatal(err)
	}
	v, _ = a.CrmJob(ctx, c.ID, v.ID)
	// predané 4 − vrátené 1 = 3 × 7,50; fakturované 48 − 12 = 36
	if v.Summary.GoodsCost.String() != "22.50" || v.Summary.Costs.String() != "22.50" || v.Summary.Invoiced.String() != "36.00" || v.Summary.Profit.String() != "13.50" {
		t.Fatalf("tovar zo skladu %+v", v.Summary)
	}
}
