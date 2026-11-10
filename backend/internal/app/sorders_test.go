package app

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/catalog"
	"github.com/fiskus-online/fiskus/backend/internal/delivery"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/link"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/quote"
	"github.com/fiskus-online/fiskus/backend/internal/sorder"
	"github.com/fiskus-online/fiskus/backend/internal/stock"
	"github.com/fiskus-online/fiskus/backend/internal/store"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

func TestSalesOrders(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	m := money.MustParse
	d := invoice.MustDate
	ct, _ := a.SaveContact(ctx, c.ID, store.Contact{Name: "Agro Liptov družstvo", City: "Liptovský Mikuláš"})
	kabel, _ := a.SaveItem(ctx, c.ID, catalog.Item{Code: "P05", Name: "Kábel", Unit: "m", Price: m("2"), PriceMode: vat.PriceNet, Rate: vat.Rate23})
	if _, err := a.SaveStockConfig(ctx, c.ID, StockConfig{Warehouses: []Warehouse{{Code: "HL", Name: "Hlavný"}},
		Items: []StockItem{{ItemID: kabel.ID, Kind: stock.Goods, Min: m("20")}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveStockDoc(ctx, c.ID, stock.Doc{Kind: stock.Receipt, Date: d("2026-10-01"), Warehouse: "HL",
		Lines: []stock.Line{{ItemID: kabel.ID, Quantity: m("50"), UnitCost: m("1.35")}}}); err != nil {
		t.Fatal(err)
	}
	card := func() StockCard {
		sv, _ := a.Stock(ctx, c.ID)
		return sv.Cards[0]
	}
	lines := []sorder.Line{
		{Line: invoice.Line{Description: kabel.Name, Quantity: m("40"), Unit: "m", UnitPrice: m("2"), Rate: vat.Rate23, ItemID: kabel.ID}},
		{Line: invoice.Line{Description: "Montáž", Quantity: m("2"), Unit: "hod", UnitPrice: m("35"), Rate: vat.Rate23}},
	}

	if _, err := a.SaveSalesOrder(ctx, c.ID, sorder.Order{ContactID: ct.ID}); !fieldErr(err, "lines") {
		t.Fatalf("bez položiek: %v", err)
	}
	o, err := a.SaveSalesOrder(ctx, c.ID, sorder.Order{ContactID: ct.ID, Date: d("2026-11-05"), CustomerRef: "OBJ-77", Lines: lines,
		Status: sorder.Done, Fulfils: []sorder.Fulfil{{Kind: "invoice", ID: 999}}})
	if err != nil {
		t.Fatal(err)
	}
	if o.Number != "OP20260001" || o.Status != sorder.Open || len(o.Fulfils) != 0 || o.Customer.Name != ct.Name || o.Summary.Base.String() != "150.00" {
		t.Fatalf("nová objednávka %+v", o)
	}
	if o.Lines[0].Available == nil || o.Lines[0].Available.String() != "50.00" || o.Lines[1].Available != nil {
		t.Fatalf("disponibilné: %+v", o.Lines)
	}
	if cd := card(); cd.Reserved.String() != "40.00" || cd.Free.String() != "10.00" {
		t.Fatalf("rezervácia na sklade: %+v", cd)
	}
	// návrh na objednanie: minimum 20 − (50 − 40 rezervované) = 10
	if rows, _ := a.Reorder(ctx, c.ID); len(rows) != 1 || rows[0].Reserved.String() != "40.00" || rows[0].Suggested.String() != "10.00" {
		t.Fatalf("návrh na objednanie: %+v", rows)
	}
	// druhá objednávka vidí, čo zostalo
	o2, err := a.SaveSalesOrder(ctx, c.ID, sorder.Order{ContactID: ct.ID, Date: d("2026-11-06"), Lines: lines[:1]})
	if err != nil || o2.Number != "OP20260002" || o2.Lines[0].Available.String() != "10.00" || o2.Lines[0].Short.String() != "30.00" {
		t.Fatalf("druhá objednávka %+v %v", o2, err)
	}
	if err := a.DeleteSalesOrder(ctx, c.ID, o2.ID); err != nil {
		t.Fatal(err)
	}

	// časť dodacím listom
	k1 := o.Lines[0].Key
	if _, err := a.FulfilSalesOrder(ctx, c.ID, o.ID, SalesOrderFulfil{To: sorder.ByDelivery, Qty: map[int]money.Dec{k1: m("50")}}); !fieldErr(err, "lines[0].pick") {
		t.Fatalf("viac ako objednané: %v", err)
	}
	doc, err := a.FulfilSalesOrder(ctx, c.ID, o.ID, SalesOrderFulfil{To: sorder.ByDelivery, Qty: map[int]money.Dec{k1: m("15")}})
	if err != nil || doc.Kind != "delivery" {
		t.Fatalf("dodací list: %+v %v", doc, err)
	}
	n, _ := a.Delivery(ctx, c.ID, doc.ID)
	if n.Status != delivery.Draft || len(n.Lines) != 1 || n.Lines[0].Quantity.String() != "15.00" || n.Order != "OBJ-77" || n.Customer.Name != ct.Name {
		t.Fatalf("koncept dodacieho listu %+v", n)
	}
	o, _ = a.SalesOrder(ctx, c.ID, o.ID)
	if o.Status != sorder.Partial || o.Lines[0].Rest.String() != "25.00" || o.Fulfils[0].Number != "koncept" {
		t.Fatalf("po dodacom liste %+v", o)
	}
	if n, err = a.IssueDelivery(ctx, c.ID, n.ID); err != nil {
		t.Fatal(err)
	}
	if cd := card(); cd.Quantity.String() != "35.00" || cd.Reserved.String() != "25.00" {
		t.Fatalf("sklad po vydaní %+v", cd)
	}
	o, _ = a.SalesOrder(ctx, c.ID, o.ID)
	if o.Fulfils[0].Number != n.Number {
		t.Fatalf("číslo dodacieho listu %+v", o.Fulfils)
	}
	if err := a.DeleteSalesOrder(ctx, c.ID, o.ID); !fieldErr(err, "status") {
		t.Fatalf("zmazanie s dodaním: %v", err)
	}
	// dodaná položka sa nedá odstrániť ani znížiť pod dodané
	edit := o
	edit.Lines = edit.Lines[1:]
	if _, err := a.SaveSalesOrder(ctx, c.ID, edit); !fieldErr(err, "lines") {
		t.Fatalf("odstránenie dodanej: %v", err)
	}
	// storno dodacieho listu vráti množstvo na dodanie
	if _, err := a.CancelDelivery(ctx, c.ID, n.ID); err != nil {
		t.Fatal(err)
	}
	o, _ = a.SalesOrder(ctx, c.ID, o.ID)
	if o.Status != sorder.Open || len(o.Fulfils) != 0 || card().Reserved.String() != "40.00" {
		t.Fatalf("po storne dodacieho listu %+v", o)
	}

	// zvyšok faktúrou: skladová položka ostane na faktúre (vydá sa ňou)
	doc, err = a.FulfilSalesOrder(ctx, c.ID, o.ID, SalesOrderFulfil{To: sorder.ByInvoice})
	if err != nil || doc.Kind != "invoice" {
		t.Fatalf("faktúra: %+v %v", doc, err)
	}
	inv, _ := a.Store.GetInvoice(ctx, c.ID, doc.ID)
	if inv.Status != invoice.StatusDraft || len(inv.Lines) != 2 || inv.Lines[0].ItemID != kabel.ID || inv.Subject != "Objednávka OBJ-77" {
		t.Fatalf("koncept faktúry %+v", inv)
	}
	o, _ = a.SalesOrder(ctx, c.ID, o.ID)
	if o.Status != sorder.Done || card().Reserved.Sign() != 0 {
		t.Fatalf("vybavená %+v", o)
	}
	if _, err := a.SaveSalesOrder(ctx, c.ID, o); !fieldErr(err, "status") {
		t.Fatalf("úprava vybavenej: %v", err)
	}
	if docs, _ := a.DocLinks(ctx, c.ID, link.Ref{Kind: link.SOrder, ID: o.ID}); len(docs) != 2 {
		t.Fatalf("prepojenia: %+v", docs)
	}
	// zmazaný koncept faktúry vráti objednávku do stavu prijatá
	if err := a.Store.DeleteInvoice(ctx, c.ID, inv.ID); err != nil {
		t.Fatal(err)
	}
	if o, _ = a.SalesOrder(ctx, c.ID, o.ID); o.Status != sorder.Open {
		t.Fatalf("po zmazaní faktúry %s", o.Status)
	}

	// zrušenie a obnovenie
	if o, err = a.CancelSalesOrder(ctx, c.ID, o.ID); err != nil || o.Status != sorder.Cancelled || card().Reserved.Sign() != 0 {
		t.Fatalf("zrušenie %+v %v", o, err)
	}
	if _, err := a.FulfilSalesOrder(ctx, c.ID, o.ID, SalesOrderFulfil{To: sorder.ByInvoice}); !fieldErr(err, "status") {
		t.Fatalf("dodanie zrušenej: %v", err)
	}
	if o, err = a.ReopenSalesOrder(ctx, c.ID, o.ID); err != nil || o.Status != sorder.Open {
		t.Fatalf("obnovenie %+v %v", o, err)
	}

	// z cenovej ponuky
	q, err := a.SaveQuote(ctx, c.ID, quote.Quote{ContactID: ct.ID, Customer: invoice.Party{Name: ct.Name, Country: "SK"}, IssueDate: d("2026-11-01"), ValidUntil: d("2026-11-30"),
		Lines: []invoice.Line{{Description: "Svietidlo", Quantity: m("4"), Unit: "ks", UnitPrice: m("30"), Rate: vat.Rate23}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SalesOrderFromQuote(ctx, c.ID, q.ID); !fieldErr(err, "status") {
		t.Fatalf("z konceptu ponuky: %v", err)
	}
	if q, err = a.IssueQuote(ctx, c.ID, q.ID); err != nil {
		t.Fatal(err)
	}
	fq, err := a.SalesOrderFromQuote(ctx, c.ID, q.ID)
	if err != nil || len(fq.Lines) != 1 || fq.Summary.Base.String() != "120.00" || fq.Customer.Name != ct.Name {
		t.Fatalf("z ponuky %+v %v", fq, err)
	}
}
