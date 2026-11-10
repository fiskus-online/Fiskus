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
	"github.com/fiskus-online/fiskus/backend/internal/stock"
	"github.com/fiskus-online/fiskus/backend/internal/store"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

func TestDeliveries(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	m := money.MustParse
	d := invoice.MustDate
	ct, _ := a.SaveContact(ctx, c.ID, store.Contact{Name: "Agro Liptov družstvo", ICO: "00612341", City: "Liptovský Mikuláš"})
	other, _ := a.SaveContact(ctx, c.ID, store.Contact{Name: "Mlyn Brezno s.r.o."})
	kabel, _ := a.SaveItem(ctx, c.ID, catalog.Item{Code: "P05", Name: "Kábel CYKY-J 5x2,5", Unit: "m", Price: m("2.19"), PriceMode: vat.PriceNet, Rate: vat.Rate23})
	if _, err := a.SaveStockConfig(ctx, c.ID, StockConfig{Warehouses: []Warehouse{{Code: "HL", Name: "Hlavný"}}, Items: []StockItem{{ItemID: kabel.ID, Kind: stock.Goods}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveStockDoc(ctx, c.ID, stock.Doc{Kind: stock.Receipt, Date: d("2026-10-01"), Warehouse: "HL",
		Lines: []stock.Line{{ItemID: kabel.ID, Quantity: m("100"), UnitCost: m("1.35")}}}); err != nil {
		t.Fatal(err)
	}
	qty := func() money.Dec {
		sv, _ := a.Stock(ctx, c.ID)
		return sv.Cards[0].Quantity
	}
	lines := []invoice.Line{
		{Description: kabel.Name, Quantity: m("40"), Unit: "m", UnitPrice: m("2.19"), Rate: vat.Rate23, ItemID: kabel.ID},
		{Description: "Montáž", Quantity: m("2"), Unit: "hod", UnitPrice: m("35"), Rate: vat.Rate23},
	}

	if _, err := a.SaveDelivery(ctx, c.ID, delivery.Note{ContactID: ct.ID}); !fieldErr(err, "lines") {
		t.Fatalf("bez položiek: %v", err)
	}
	n, err := a.SaveDelivery(ctx, c.ID, delivery.Note{ContactID: ct.ID, Date: d("2026-11-05"), Place: "Sušička", Lines: lines, Number: "X", Status: delivery.Invoiced})
	if err != nil {
		t.Fatal(err)
	}
	if n.Number != "" || n.Status != delivery.Draft || n.Customer.Name != ct.Name || n.Summary.Base.String() != "157.60" || qty().String() != "100.00" {
		t.Fatalf("koncept %+v", n)
	}
	n, err = a.IssueDelivery(ctx, c.ID, n.ID)
	if err != nil || n.Number != "DL20260001" || n.Status != delivery.Issued || qty().String() != "60.00" {
		t.Fatalf("vydanie %+v %v, sklad %s", n, err, qty())
	}
	if err := a.DeleteDelivery(ctx, c.ID, n.ID); !fieldErr(err, "status") {
		t.Fatalf("zmazanie vydaného: %v", err)
	}
	// úprava vydaného: číslo ostane, výdajka sa upraví
	n.Lines[0].Quantity = m("45")
	if n, err = a.SaveDelivery(ctx, c.ID, n); err != nil || n.Number != "DL20260001" || qty().String() != "55.00" {
		t.Fatalf("úprava vydaného %+v %v, sklad %s", n, err, qty())
	}

	// druhý dodací list z ponuky, tretí pre iného odberateľa
	qd, _ := a.SaveQuote(ctx, c.ID, quote.Quote{Customer: n.Customer, ContactID: ct.ID, IssueDate: d("2026-11-01"), ValidUntil: d("2026-11-30"), Currency: "EUR",
		Lines: []invoice.Line{{Description: "Revízia rozvádzača", Quantity: m("1"), Unit: "ks", UnitPrice: m("120"), Rate: vat.Rate23}}})
	if _, err := a.DeliveryFromQuote(ctx, c.ID, qd.ID); !fieldErr(err, "status") {
		t.Fatalf("z konceptu ponuky: %v", err)
	}
	q, _ := a.IssueQuote(ctx, c.ID, qd.ID)
	n2, err := a.DeliveryFromQuote(ctx, c.ID, q.ID)
	if err != nil || len(n2.Lines) != 1 || n2.Note != "Podľa cenovej ponuky "+q.Number+"." {
		t.Fatalf("z ponuky %+v %v", n2, err)
	}
	if docs, _ := a.DocLinks(ctx, c.ID, link.Ref{Kind: link.Delivery, ID: n2.ID}); len(docs) != 1 || docs[0].Kind != link.Quote {
		t.Fatalf("prepojenie s ponukou %+v", docs)
	}
	if _, err := a.InvoiceDeliveries(ctx, c.ID, []int64{n.ID, n2.ID}); !fieldErr(err, "ids") {
		t.Fatalf("koncept na faktúru: %v", err)
	}
	n2, _ = a.IssueDelivery(ctx, c.ID, n2.ID)
	n3, _ := a.SaveDelivery(ctx, c.ID, delivery.Note{ContactID: other.ID, Date: d("2026-11-06"), Lines: lines[1:]})
	n3, _ = a.IssueDelivery(ctx, c.ID, n3.ID)
	if _, err := a.InvoiceDeliveries(ctx, c.ID, []int64{n.ID, n3.ID}); !fieldErr(err, "ids") {
		t.Fatalf("rôzni odberatelia: %v", err)
	}

	// zberná faktúra
	inv, err := a.InvoiceDeliveries(ctx, c.ID, []int64{n.ID, n2.ID, n.ID})
	if err != nil {
		t.Fatal(err)
	}
	if inv.Status != invoice.StatusDraft || len(inv.Lines) != 3 || inv.Lines[0].ItemID != 0 || inv.ContactID != ct.ID ||
		inv.Note != "Podľa dodacích listov DL20260001, DL20260002." || inv.DeliveryDate.String() != "2026-11-05" || inv.Summary.Base.String() != "288.55" {
		t.Fatalf("faktúra %+v", inv)
	}
	n, _ = a.Delivery(ctx, c.ID, n.ID)
	if n.Status != delivery.Invoiced || n.InvoiceID != inv.ID || n.InvoiceNumber != "koncept" {
		t.Fatalf("vyfakturovaný %+v", n)
	}
	if _, err := a.SaveDelivery(ctx, c.ID, n); !fieldErr(err, "status") {
		t.Fatalf("úprava vyfakturovaného: %v", err)
	}
	if _, err := a.CancelDelivery(ctx, c.ID, n.ID); !fieldErr(err, "status") {
		t.Fatalf("storno vyfakturovaného: %v", err)
	}
	issued, err := a.Issue(ctx, c.ID, inv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if qty().String() != "55.00" {
		t.Fatalf("faktúra nesmie vydať tovar druhýkrát: %s", qty())
	}
	if n, _ = a.Delivery(ctx, c.ID, n.ID); n.InvoiceNumber != issued.Number {
		t.Fatalf("číslo faktúry %+v", n)
	}
	// storno faktúry vráti dodacie listy na vydané; storno dodacieho listu vráti tovar
	if _, err := a.Cancel(ctx, c.ID, issued.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ = a.Delivery(ctx, c.ID, n.ID); n.Status != delivery.Issued || n.InvoiceID != 0 {
		t.Fatalf("po storne faktúry %+v", n)
	}
	if n, err = a.CancelDelivery(ctx, c.ID, n.ID); err != nil || n.Status != delivery.Cancelled || qty().String() != "100.00" {
		t.Fatalf("storno dodacieho listu %+v %v, sklad %s", n, err, qty())
	}
	// koncept sa dá zmazať
	n4, _ := a.SaveDelivery(ctx, c.ID, delivery.Note{ContactID: ct.ID, Lines: lines[1:]})
	if err := a.DeleteDelivery(ctx, c.ID, n4.ID); err != nil {
		t.Fatal(err)
	}
}
