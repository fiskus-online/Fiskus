package app

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/catalog"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/link"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/porder"
	"github.com/fiskus-online/fiskus/backend/internal/stock"
	"github.com/fiskus-online/fiskus/backend/internal/store"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

func TestPurchaseOrders(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	m := money.MustParse
	d := invoice.MustDate
	sup, _ := a.SaveContact(ctx, c.ID, store.Contact{Name: "Elektro veľkoobchod BB s.r.o.", ICO: "36000001"})
	kabel, _ := a.SaveItem(ctx, c.ID, catalog.Item{Code: "P05", Name: "Kábel CYKY-J 5x2,5", Unit: "m", Price: m("2.19"), PriceMode: vat.PriceNet, Rate: vat.Rate23,
		Supplier: "elektro veľkoobchod BB s.r.o.", SupplierCode: "CYKY5X25", PurchasePrice: m("1.30")})
	istic, _ := a.SaveItem(ctx, c.ID, catalog.Item{Code: "B16", Name: "Istič B16 3P", Unit: "ks", Price: m("18.50"), PriceMode: vat.PriceNet, Rate: vat.Rate23,
		Supplier: "Schrack", PurchasePrice: m("11.20")})
	svorka, _ := a.SaveItem(ctx, c.ID, catalog.Item{Code: "WAGO", Name: "Svorka WAGO 221", Unit: "ks", Price: m("0.60"), PriceMode: vat.PriceNet, Rate: vat.Rate23})
	if _, err := a.SaveStockConfig(ctx, c.ID, StockConfig{Warehouses: []Warehouse{{Code: "HL", Name: "Hlavný"}, {Code: "ZV", Name: "Zvolen"}},
		Items: []StockItem{{ItemID: kabel.ID, Kind: stock.Material, Min: m("100")}, {ItemID: istic.ID, Kind: stock.Goods, Min: m("5")},
			{ItemID: svorka.ID, Kind: stock.Material}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveStockDoc(ctx, c.ID, stock.Doc{Kind: stock.Receipt, Date: d("2026-10-01"), Warehouse: "HL",
		Lines: []stock.Line{{ItemID: kabel.ID, Quantity: m("30"), UnitCost: m("1.30")}, {ItemID: istic.ID, Quantity: m("6"), UnitCost: m("11")}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveStockDoc(ctx, c.ID, stock.Doc{Kind: stock.Receipt, Date: d("2026-10-01"), Warehouse: "ZV",
		Lines: []stock.Line{{ItemID: kabel.ID, Quantity: m("12.5"), UnitCost: m("1.30")}}}); err != nil {
		t.Fatal(err)
	}

	// na objednanie: kábel 42,5 m z minima 100 → 58 m (nahor), istič je nad minimom
	rows, err := a.Reorder(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ItemID != kabel.ID || rows[0].Stock.String() != "42.50" || rows[0].Suggested.String() != "58.00" {
		t.Fatalf("na objednanie %+v", rows)
	}
	if _, err := a.OrdersFromReorder(ctx, c.ID, nil); !fieldErr(err, "picks") {
		t.Fatalf("prázdny výber: %v", err)
	}
	made, err := a.OrdersFromReorder(ctx, c.ID, []ReorderPick{{ItemID: kabel.ID, Quantity: m("100")}, {ItemID: istic.ID, Quantity: m("10")}, {ItemID: svorka.ID, Quantity: m("50")}})
	if err != nil {
		t.Fatal(err)
	}
	// dodávatelia: Bez dodávateľa, Elektro… (z adresára), Schrack
	if len(made) != 3 || made[1].ContactID != sup.ID || made[1].Supplier != sup.Name || made[1].Lines[0].SupplierCode != "CYKY5X25" ||
		made[1].Total.String() != "130.00" || made[0].Supplier != "Bez dodávateľa" || made[1].Number == "" || made[1].Status != porder.Draft {
		t.Fatalf("objednávky %+v", made)
	}
	if rows, _ := a.Reorder(ctx, c.ID); len(rows) != 0 {
		t.Fatalf("po objednaní nie je čo objednať: %+v", rows)
	}
	o := made[1]

	// úprava: prijaté množstvá z klienta neplatia
	o.Lines[0].Received = m("100")
	o.Lines[0].UnitPrice = m("1.25")
	o.Expected = d("2026-11-14")
	if o, err = a.SavePurchaseOrder(ctx, c.ID, o); err != nil {
		t.Fatal(err)
	}
	if !o.Lines[0].Received.IsZero() || o.Total.String() != "125.00" {
		t.Fatalf("úprava %+v", o)
	}
	if o, err = a.SendPurchaseOrder(ctx, c.ID, o.ID); err != nil || o.Status != porder.Sent || o.Sent.IsZero() {
		t.Fatalf("odoslanie %+v %v", o, err)
	}
	if _, err := a.SendPurchaseOrder(ctx, c.ID, o.ID); !fieldErr(err, "status") {
		t.Fatalf("druhé odoslanie: %v", err)
	}

	// čiastočný príjem do Zvolena
	got, err := a.ReceivePurchaseOrder(ctx, c.ID, o.ID, POrderReceipt{Date: d("2026-11-12"), Warehouse: "ZV", Quantity: []money.Dec{m("60")}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Order.Status != porder.Partial || got.Stock == "" || got.Order.Lines[0].Received.String() != "60.00" {
		t.Fatalf("čiastočný príjem %+v", got)
	}
	sd, _ := a.Store.GetStockDoc(ctx, c.ID, got.StockID)
	if sd.Kind != stock.Receipt || sd.Warehouse != "ZV" || sd.LinkKind != "porder" || sd.LinkNumber != o.Number || sd.Lines[0].UnitCost.String() != "1.25" {
		t.Fatalf("príjemka %+v", sd)
	}
	if it, _ := a.Store.GetItem(ctx, c.ID, kabel.ID); it.PurchasePrice.String() != "1.25" {
		t.Errorf("nákupná cena %s", it.PurchasePrice)
	}
	if _, err := a.ReceivePurchaseOrder(ctx, c.ID, o.ID, POrderReceipt{Quantity: []money.Dec{m("41")}}); !fieldErr(err, "lines[0].receive") {
		t.Fatalf("príjem nad objednané: %v", err)
	}
	// prijatý riadok nemožno zmazať, množstvo nie pod prijaté
	o = got.Order
	o.Lines[0].Quantity = m("50")
	if _, err := a.SavePurchaseOrder(ctx, c.ID, o); !fieldErr(err, "lines[0].quantity") {
		t.Fatalf("menej ako prijaté: %v", err)
	}
	o.Lines = nil
	if _, err := a.SavePurchaseOrder(ctx, c.ID, o); !fieldErr(err, "lines") {
		t.Fatalf("zmazanie prijatého riadku: %v", err)
	}
	if err := a.DeletePurchaseOrder(ctx, c.ID, o.ID); !fieldErr(err, "status") {
		t.Fatalf("zmazanie prijímanej objednávky: %v", err)
	}
	got, err = a.ReceivePurchaseOrder(ctx, c.ID, o.ID, POrderReceipt{Date: d("2026-11-13"), Quantity: []money.Dec{m("40")}})
	if err != nil || got.Order.Status != porder.Received {
		t.Fatalf("dodanie zvyšku %+v %v", got, err)
	}
	if _, err := a.SavePurchaseOrder(ctx, c.ID, got.Order); !fieldErr(err, "status") {
		t.Fatalf("úprava prijatej: %v", err)
	}
	sv, _ := a.Stock(ctx, c.ID)
	total := money.Zero
	for _, cd := range sv.Cards {
		if cd.ItemID == kabel.ID {
			total = total.Add(cd.Quantity)
		}
	}
	if total.String() != "142.50" {
		t.Fatalf("kábel na skladoch %s", total)
	}

	// položka mimo skladu: príjem bez príjemky; zrušenie a zmazanie
	free, _ := a.SavePurchaseOrder(ctx, c.ID, porder.Order{Supplier: "Hornbach", Lines: []porder.Line{{Name: "Hmoždinky", Quantity: m("2"), UnitPrice: m("3")}}})
	if r, err := a.ReceivePurchaseOrder(ctx, c.ID, free.ID, POrderReceipt{Quantity: []money.Dec{m("2")}}); err != nil || r.StockID != 0 || r.Order.Status != porder.Received {
		t.Fatalf("príjem mimo skladu %+v %v", r, err)
	}
	if x, err := a.CancelPurchaseOrder(ctx, c.ID, made[2].ID); err != nil || x.Status != porder.Cancelled {
		t.Fatalf("zrušenie %+v %v", x, err)
	}
	if _, err := a.CancelPurchaseOrder(ctx, c.ID, made[2].ID); !fieldErr(err, "status") {
		t.Fatalf("druhé zrušenie: %v", err)
	}
	if _, err := a.LinkDocs(ctx, c.ID, link.Ref{Kind: link.POrder, ID: made[0].ID}, link.Ref{Kind: link.POrder, ID: o.ID}); err != nil {
		t.Fatal(err)
	}
	if err := a.DeletePurchaseOrder(ctx, c.ID, made[0].ID); err != nil {
		t.Fatal(err)
	}
	if ls, _ := a.Store.ListDocLinks(ctx, c.ID, link.Ref{Kind: link.POrder, ID: o.ID}); len(ls) != 0 {
		t.Errorf("prepojenia zmazanej objednávky ostali: %+v", ls)
	}
	if docs, _ := a.DocLinks(ctx, c.ID, link.Ref{Kind: link.POrder, ID: o.ID}); len(docs) != 0 {
		t.Errorf("súvisiace %+v", docs)
	}
}

func TestCeilWhole(t *testing.T) {
	for in, want := range map[string]string{"57.5": "58.00", "58": "58.00", "0.01": "1.00"} {
		if got := ceilWhole(money.MustParse(in)).String(); got != want {
			t.Errorf("%s → %s, chcel som %s", in, got, want)
		}
	}
}
