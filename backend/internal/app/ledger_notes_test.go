package app

import (
	"strings"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/ledger"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/stock"
)

func TestLedgerNotes(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	// mikro: bez prehľadu riadkov (poznámky sú pre vzor Úč POD)
	n, err := a.LedgerNotesFor(ctx, c.ID, 2026)
	if err != nil || n.Form != ledger.FormMicro || len(n.Rows) != 0 || n.Company.Name != c.Name || !strings.Contains(n.Basis, "31. 12. 2026") {
		t.Fatalf("%+v %v", n, err)
	}
	if strings.Contains(n.DefaultPolicies, "Zásoby") {
		t.Fatal("bez skladu sa zásoby v zásadách neuvádzajú")
	}
	if err := a.SetStatementForm(ctx, c.ID, 2026, ledger.FormSmall); err != nil {
		t.Fatal(err)
	}
	cfg, _ := a.loadStock(ctx, c.ID)
	cfg.Method, cfg.Items = stock.FIFO, []StockItem{{ItemID: 1, Kind: stock.Goods}}
	if err := a.saveStockConfig(ctx, c.ID, cfg); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		in    NotesInput
		field string
	}{
		{"záporné záväzky", NotesInput{Liabilities5y: m.MustParse("-1")}, "liabilities5y"},
		{"počet zamestnancov nie je číslo", NotesInput{Employees: "dvaja"}, "employees"},
		{"v poriadku", NotesInput{Activity: "Elektroinštalácie a revízie", Employees: "2,5", PrevApproved: invoice.MustDate("2026-03-20"), Liabilities5y: m.MustParse("12000")}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := a.SaveLedgerNotes(ctx, c.ID, 2026, tt.in)
			if tt.field != "" {
				if !fieldErr(err, tt.field) {
					t.Fatalf("chyba %v", err)
				}
				return
			}
			if err != nil || n.Input.Activity != tt.in.Activity || n.Form != ledger.FormSmall {
				t.Fatalf("%+v %v", n.Input, err)
			}
			// prehľad: hlavné riadky súvahy a výkazu, konečné sumy
			codes := map[string]bool{}
			for _, r := range n.Rows {
				codes[r.Statement+":"+r.Code] = true
				if r.Label == "" {
					t.Errorf("riadok %s %s bez názvu", r.Statement, r.Code)
				}
			}
			if !codes["aktíva:01"] || !codes["pasíva:79"] || !codes["výkaz:61"] || len(n.Rows) != 36 {
				t.Fatalf("riadky %d %v", len(n.Rows), codes)
			}
			if !strings.Contains(n.DefaultPolicies, "FIFO") {
				t.Fatalf("zásady: %s", n.DefaultPolicies)
			}
		})
	}
	// uložené poznámky sú pre rok; iný rok je prázdny
	if n, _ := a.LedgerNotesFor(ctx, c.ID, 2025); n.Input.Activity != "" {
		t.Fatalf("2025: %+v", n.Input)
	}
}
