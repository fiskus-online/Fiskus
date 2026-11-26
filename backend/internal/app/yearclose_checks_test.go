package app

import (
	"strings"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/asset"
	"github.com/fiskus-online/fiskus/backend/internal/cash"
	"github.com/fiskus-online/fiskus/backend/internal/catalog"
	"github.com/fiskus-online/fiskus/backend/internal/chart"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/fx"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/ledger"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/stock"
	"github.com/fiskus-online/fiskus/backend/internal/store"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

// Kontroly uzávierky, ktoré spájajú moduly: odpisy, kurzové rozdiely, sklad.
func TestYearCloseIntegrityChecks(t *testing.T) {
	type step struct {
		name     string
		do       func(t *testing.T, a *App, cid int64)
		id       string
		ok       bool
		contains string
	}
	tests := []struct {
		name  string
		setup func(t *testing.T, a *App, cid int64)
		steps []step
	}{
		{"účtovné odpisy", func(t *testing.T, a *App, cid int64) {
			if _, err := a.SaveAsset(ctx, cid, asset.Asset{Name: "Stroj", Kind: asset.Tangible, Group: 1, Acquired: invoice.MustDate("2025-03-15"),
				Cost: m.MustParse("4800"), AccMonths: 48}); err != nil {
				t.Fatal(err)
			}
		}, []step{
			{"nezaúčtované", nil, "depreciation", false, "nie sú zaúčtované"},
			{"zaúčtované", func(t *testing.T, a *App, cid int64) {
				if _, err := a.PostAccDepreciation(ctx, cid, 2026); err != nil {
					t.Fatal(err)
				}
			}, "depreciation", true, "zaúčtované"},
			{"zmena registra", func(t *testing.T, a *App, cid int64) {
				list, _ := a.Store.ListAssets(ctx, cid)
				x := list[0]
				x.AccMonths = 24
				if _, err := a.SaveAsset(ctx, cid, x); err != nil {
					t.Fatal(err)
				}
			}, "depreciation", false, "nezodpovedá"},
		}},
		{"kurzové rozdiely", func(t *testing.T, a *App, cid int64) {
			if err := a.Store.SaveRates(ctx, []fx.Rate{rate("USD", "2026-03-02", "1.1"), rate("USD", "2026-12-31", "1.25")}); err != nil {
				t.Fatal(err)
			}
			regs, err := a.SaveCashRegisters(ctx, cid, []cash.Register{{Name: "Pokladnica"}, {ID: -1, Name: "Pokladnica USD", Currency: "USD", Prefix: "US"}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := a.SaveAccount(ctx, cid, "", chart.Account{Number: "211.002", Name: "Pokladnica USD", Active: true}); err != nil {
				t.Fatal(err)
			}
			for _, r := range regs {
				if r.Currency != "USD" {
					continue
				}
				cd, err := a.CreateCash(ctx, cid, cash.Document{Kind: cash.In, RegisterID: r.ID, Date: invoice.MustDate("2026-03-03"),
					Amount: m.MustParse("1100"), Purpose: "Vklad USD"})
				if err != nil {
					t.Fatal(err)
				}
				if err := a.SetMoneyAccounts(ctx, cid, map[string]string{"cash:" + itoa(int(r.ID)): "211002"}); err != nil {
					t.Fatal(err)
				}
				if err := a.SetPosting(ctx, cid, 2026, "cash:"+itoa(int(cd.ID)), "", []ledger.Line{{Account: "211002", Debit: m.MustParse("1000")},
					{Account: "491", Credit: m.MustParse("1000")}}, "", ledger.Dims{}); err != nil {
					t.Fatal(err)
				}
			}
		}, []step{
			{"nezaúčtované", nil, "fx", false, "nie sú zaúčtované"},
			{"zaúčtované", func(t *testing.T, a *App, cid int64) {
				if _, err := a.PostFXRevaluation(ctx, cid, 2026); err != nil {
					t.Fatal(err)
				}
			}, "fx", true, "zaúčtované"},
		}},
		{"výdaj nad stav", func(t *testing.T, a *App, cid int64) {
			goods, _ := a.SaveItem(ctx, cid, catalog.Item{Code: "T1", Name: "Istič", Unit: "ks", Price: m.MustParse("20"), PriceMode: vat.PriceNet, Rate: vat.Rate23})
			if _, err := a.SaveStockConfig(ctx, cid, StockConfig{Warehouses: []Warehouse{{Code: "HL", Name: "Hlavný"}},
				Items: []StockItem{{ItemID: goods.ID, Kind: stock.Goods}}}); err != nil {
				t.Fatal(err)
			}
			for _, d := range []stock.Doc{
				{Kind: stock.Receipt, Date: invoice.MustDate("2026-10-08"), Warehouse: "HL", Lines: []stock.Line{{ItemID: goods.ID, Quantity: m.MustParse("2"), UnitCost: m.MustParse("10")}}},
				{Kind: stock.Issue, Date: invoice.MustDate("2026-10-09"), Warehouse: "HL", Lines: []stock.Line{{ItemID: goods.ID, Quantity: m.MustParse("5")}}},
			} {
				if _, err := a.SaveStockDoc(ctx, cid, d); err != nil {
					t.Fatal(err)
				}
			}
		}, []step{
			{"chyba ocenenia", nil, "stock-valuation", false, "1 dokladov"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, c := newApp(t, edition.Desktop)
			c.Bookkeeping = store.BookkeepingDouble
			if _, err := a.UpdateCompany(ctx, c); err != nil {
				t.Fatal(err)
			}
			tt.setup(t, a, c.ID)
			a.Today = func() invoice.Date { return invoice.MustDate("2027-02-10") }
			for _, s := range tt.steps {
				if s.do != nil {
					s.do(t, a, c.ID)
				}
				ck := checkByID(t, a, c.ID, 2026, s.id)
				if ck.OK != s.ok || !ck.Blocking || !strings.Contains(ck.Text, s.contains) {
					t.Errorf("%s: %+v", s.name, ck)
				}
				_, err := a.CloseYear(ctx, c.ID, 2026)
				if !s.ok && err == nil {
					t.Errorf("%s: rok sa uzavrel napriek chybe", s.name)
				}
				if s.ok && err != nil {
					t.Errorf("%s: rok sa neuzavrel: %v", s.name, err)
				}
				if err == nil {
					if _, err := a.ReopenYear(ctx, c.ID, 2026, "ďalší krok testu"); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}
