package app

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/catalog"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/ledger"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/purchase"
	"github.com/fiskus-online/fiskus/backend/internal/stock"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

// Nákup zásob: predkontácia podľa príjemky pripojenej k prijatej faktúre
// (tovar 131, materiál na sklad 111, bez príjemky spotreba 501), aby
// obstaranie a príjemka sedeli (131 a 111 bez zostatku).
func TestStockPurchaseTemplate(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	goods, _ := a.SaveItem(ctx, c.ID, catalog.Item{Code: "T1", Name: "Istič", Unit: "ks", Price: m.MustParse("20"), PriceMode: vat.PriceNet, Rate: vat.Rate23})
	mat, _ := a.SaveItem(ctx, c.ID, catalog.Item{Code: "M1", Name: "Svorky", Unit: "ks", Price: m.MustParse("1"), PriceMode: vat.PriceNet, Rate: vat.Rate23})
	if _, err := a.SaveStockConfig(ctx, c.ID, StockConfig{Warehouses: []Warehouse{{Code: "HL", Name: "Hlavný"}},
		Items: []StockItem{{ItemID: goods.ID, Kind: stock.Goods}, {ItemID: mat.ID, Kind: stock.Material}}}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		receipt  []int64 // položky príjemky (každá 10 ks po 10 €)
		tpl      string
		lines    string
		problem  bool
		zeroAcct string // účet obstarania bez zostatku
	}{
		{"bez príjemky — materiál do spotreby", nil, "FP-MATERIAL", "501 MD 100.00, 343 MD 23.00, 321 D 123.00", false, ""},
		{"tovar na sklad", []int64{goods.ID}, "FP-TOVAR", "131 MD 100.00, 343 MD 23.00, 321 D 123.00", false, "131"},
		{"materiál na sklad", []int64{mat.ID}, "FP-MATERIAL-SKLAD", "111 MD 100.00, 343 MD 23.00, 321 D 123.00", false, "111"},
		{"tovar aj materiál — ručne", []int64{goods.ID, mat.ID}, "", "", true, ""},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validPurchase()
			in.SupplierNumber = "Z-" + itoa(i)
			in.Expense = purchase.ExpenseStock
			in.Rates = []purchase.RateRow{{Rate: vat.Rate23, Base: m.MustParse("100")}}
			p, _, err := a.SavePurchase(ctx, c.ID, in)
			if err != nil {
				t.Fatal(err)
			}
			if tt.receipt != nil {
				var ls []stock.Line
				for _, id := range tt.receipt {
					q := "10"
					if len(tt.receipt) > 1 {
						q = "5"
					}
					ls = append(ls, stock.Line{ItemID: id, Quantity: m.MustParse(q), UnitCost: m.MustParse("10")})
				}
				if _, err := a.SaveStockDoc(ctx, c.ID, stock.Doc{Kind: stock.Receipt, Date: invoice.MustDate("2026-10-08"), Warehouse: "HL",
					LinkKind: "purchase", LinkID: p.ID, Lines: ls}); err != nil {
					t.Fatal(err)
				}
			}
			book, err := a.Ledger(ctx, c.ID, 2026)
			if err != nil {
				t.Fatal(err)
			}
			var ps ledger.Posting
			for _, x := range book.Postings {
				if x.Key == "purchase:"+itoa(int(p.ID)) {
					ps = x
				}
			}
			if ps.Template != tt.tpl || (ps.Problem != "") != tt.problem || (!tt.problem && lineStr(ps.Lines) != tt.lines) {
				t.Fatalf("%s [%s] %q", ps.Template, lineStr(ps.Lines), ps.Problem)
			}
			if tt.zeroAcct != "" {
				for _, acc := range book.Accounts {
					if acc.Account == tt.zeroAcct && !acc.Closing.IsZero() {
						t.Fatalf("%s má zostatok %s", tt.zeroAcct, acc.Closing)
					}
				}
			}
			// ďalší prípad začína bez príjemiek tohto nákupu
			docs, _ := a.Store.ListStockDocs(ctx, c.ID)
			for _, d := range docs {
				_ = a.Store.DeleteStockDoc(ctx, c.ID, d.ID)
			}
		})
	}
}
