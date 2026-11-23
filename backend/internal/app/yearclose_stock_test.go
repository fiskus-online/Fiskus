package app

import (
	"strings"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/catalog"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
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
	tests := []struct {
		name string
		link bool
		ok   bool
		text []string
	}{
		{"príjemka bez faktúry", false, false, []string{"131: -100,00 €", "1 príjemka bez pripojenej prijatej faktúry"}},
		{"príjemka s faktúrou", true, true, []string{"bez zostatku"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.link {
				rc.LinkKind, rc.LinkID = "purchase", p.ID
				if _, err := a.SaveStockDoc(ctx, c.ID, rc); err != nil {
					t.Fatal(err)
				}
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
