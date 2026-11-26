package app

import (
	"strings"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/catalog"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/porder"
	"github.com/fiskus-online/fiskus/backend/internal/purchase"
	"github.com/fiskus-online/fiskus/backend/internal/stock"
	"github.com/fiskus-online/fiskus/backend/internal/store"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

// Príjemka bez pripojenej prijatej faktúry nechá na 131 zostatok —
// uzávierka na to upozorní (nezastaví ju); po pripojení faktúry je 131 na nule.
func TestYearCloseStockCheck(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	c.Bookkeeping = store.BookkeepingDouble
	if _, err := a.UpdateCompany(ctx, c); err != nil {
		t.Fatal(err)
	}
	goods, _ := a.SaveItem(ctx, c.ID, catalog.Item{Code: "T1", Name: "Istič", Unit: "ks", Price: m.MustParse("20"), PriceMode: vat.PriceNet, Rate: vat.Rate23})
	if _, err := a.SaveStockConfig(ctx, c.ID, StockConfig{Warehouses: []Warehouse{{Code: "HL", Name: "Hlavný"}},
		Items: []StockItem{{ItemID: goods.ID, Kind: stock.Goods}}}); err != nil {
		t.Fatal(err)
	}
	in := validPurchase()
	in.Expense = purchase.ExpenseStock
	in.Rates = []purchase.RateRow{{Rate: vat.Rate23, Base: m.MustParse("100")}}
	p, _, err := a.SavePurchase(ctx, c.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := a.SaveStockDoc(ctx, c.ID, stock.Doc{Kind: stock.Receipt, Date: invoice.MustDate("2026-10-08"), Warehouse: "HL",
		Lines: []stock.Line{{ItemID: goods.ID, Quantity: m.MustParse("10"), UnitCost: m.MustParse("10")}}})
	if err != nil {
		t.Fatal(err)
	}
	a.Today = func() invoice.Date { return invoice.MustDate("2027-02-10") }
	check := func() YearCheck {
		info, err := a.YearCloseFor(ctx, c.ID, 2026)
		if err != nil {
			t.Fatal(err)
		}
		for _, x := range info.Checks {
			if x.ID == "stock" {
				if x.Blocking {
					t.Fatal("kontrola zásob nemá blokovať uzávierku")
				}
				return x
			}
		}
		t.Fatalf("kontrola zásob chýba: %+v", info.Checks)
		return YearCheck{}
	}
	po, err := a.SavePurchaseOrder(ctx, c.ID, porder.Order{Date: invoice.MustDate("2026-10-01"), Supplier: "Elektro veľkoobchod",
		Lines: []porder.Line{{ItemID: goods.ID, Name: "Istič", Unit: "ks", Quantity: m.MustParse("10"), UnitPrice: m.MustParse("10")}}})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		kind     string // väzba príjemky
		id       int64
		purchase int64 // faktúra k príjemke z objednávky
		ok       bool
		text     []string
	}{
		{"príjemka bez faktúry", "", 0, 0, false, []string{"131: -100,00 €", "1 príjemka bez pripojenej prijatej faktúry"}},
		{"príjemka s faktúrou", "purchase", p.ID, 0, true, []string{"bez zostatku"}},
		{"príjemka z objednávky bez faktúry", "porder", po.ID, 0, false, []string{"131: -100,00 €", "1 príjemka bez pripojenej prijatej faktúry"}},
		{"príjemka z objednávky s faktúrou", "porder", po.ID, p.ID, true, []string{"bez zostatku"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc.LinkKind, rc.LinkID, rc.PurchaseID = tt.kind, tt.id, tt.purchase
			saved, err := a.SaveStockDoc(ctx, c.ID, rc)
			if err != nil {
				t.Fatal(err)
			}
			want := tt.purchase
			if tt.kind == "purchase" {
				want = p.ID
			}
			if saved.Purchase() != want {
				t.Fatalf("faktúra príjemky: %+v", saved)
			}
			if tt.purchase != 0 && saved.PurchaseNumber != p.Number {
				t.Fatalf("číslo faktúry: %+v", saved)
			}
			x := check()
			if x.OK != tt.ok {
				t.Fatalf("%+v", x)
			}
			// bez faktúry: 131 kredit -100 € z príjemky; faktúra (501) 131 nedotkne
			if !tt.ok && (!strings.Contains(x.Text, tt.text[0]) || !strings.Contains(x.Text, tt.text[1])) {
				t.Fatalf("text: %s", x.Text)
			}
			if tt.ok && !strings.Contains(x.Text, tt.text[0]) {
				t.Fatalf("text: %s", x.Text)
			}
			// to isté upozornenie je pri súvahe počas roka
			st, err := a.LedgerStatementsFor(ctx, c.ID, 2026)
			if err != nil || (st.StockWarning == "") != tt.ok {
				t.Fatalf("upozornenie pri súvahe: %q %v", st.StockWarning, err)
			}
		})
	}
}

// Faktúra k príjemke z objednávky: neexistujúca sa odmietne, pri inej väzbe sa vymaže.
func TestStockReceiptPurchase(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	goods, _ := a.SaveItem(ctx, c.ID, catalog.Item{Code: "T1", Name: "Istič", Unit: "ks", Price: m.MustParse("20"), PriceMode: vat.PriceNet, Rate: vat.Rate23})
	if _, err := a.SaveStockConfig(ctx, c.ID, StockConfig{Warehouses: []Warehouse{{Code: "HL", Name: "Hlavný"}},
		Items: []StockItem{{ItemID: goods.ID, Kind: stock.Goods}}}); err != nil {
		t.Fatal(err)
	}
	po, err := a.SavePurchaseOrder(ctx, c.ID, porder.Order{Date: invoice.MustDate("2026-10-01"), Supplier: "Elektro veľkoobchod",
		Lines: []porder.Line{{ItemID: goods.ID, Name: "Istič", Unit: "ks", Quantity: m.MustParse("1"), UnitPrice: m.MustParse("10")}}})
	if err != nil {
		t.Fatal(err)
	}
	doc := stock.Doc{Kind: stock.Receipt, Date: invoice.MustDate("2026-10-08"), Warehouse: "HL",
		Lines: []stock.Line{{ItemID: goods.ID, Quantity: m.MustParse("1"), UnitCost: m.MustParse("10")}}}
	tests := []struct {
		name   string
		kind   string
		id     int64
		pur    int64
		field  string
		wantID int64
	}{
		{"neexistujúca faktúra", "porder", po.ID, 999, "purchaseId", 0},
		{"bez väzby faktúra zmizne", "", 0, 5, "", 0},
		{"z objednávky bez faktúry", "porder", po.ID, 0, "", 0},
	}
	for _, tt := range tests {
		d := doc
		d.LinkKind, d.LinkID, d.PurchaseID = tt.kind, tt.id, tt.pur
		got, err := a.SaveStockDoc(ctx, c.ID, d)
		if tt.field != "" {
			if !fieldErr(err, tt.field) {
				t.Errorf("%s: %v", tt.name, err)
			}
			continue
		}
		if err != nil || got.PurchaseID != tt.wantID || got.Purchase() != tt.wantID {
			t.Errorf("%s: %+v %v", tt.name, got, err)
		}
	}
}
