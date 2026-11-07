package app

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/bank"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/journal"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/purchase"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

func TestCustomsDecision(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	// faktúra dodávateľa z Číny bez DPH (900 €)
	imp := validPurchase()
	imp.SupplierNumber, imp.VariableSymbol = "CN-77", "77"
	imp.Supplier = invoice.Party{Name: "Shenzhen Parts Co.", Country: "CN"}
	imp.SupplyKind, imp.Expense = invoice.SupplyGoods, purchase.ExpenseStock
	imp.Rates = []purchase.RateRow{{Rate: vat.Rate0, Base: m.MustParse("900")}}
	imp, _, err := a.SavePurchase(ctx, c.ID, imp)
	if err != nil || imp.Summary.Total.String() != "900.00" {
		t.Fatalf("faktúra dovozu: %+v %v", imp.Summary, err)
	}
	// rozhodnutie colného úradu: základ 936 (900 + clo 36), DPH 23 % 215,28 + clo 36
	cu := validPurchase()
	cu.SupplierNumber, cu.VariableSymbol = "26SK000000123456A1", "123456"
	cu.Supplier = invoice.Party{Name: "Colný úrad Bratislava", Country: "SK"}
	cu.Expense = purchase.ExpenseStock
	cu.Rates = []purchase.RateRow{{Rate: vat.Rate23, Base: m.MustParse("936")}}
	cu.Customs = &purchase.Customs{Duty: m.MustParse("36"), ImportID: imp.ID}
	cu, _, err = a.SavePurchase(ctx, c.ID, cu)
	if err != nil || cu.Summary.Total.String() != "251.28" || cu.Customs.ImportNumber != imp.Number {
		t.Fatalf("rozhodnutie: %+v %+v %v", cu.Summary, cu.Customs, err)
	}

	// zlé väzby
	bad := cu
	bad.ID, bad.SupplierNumber = 0, "X1"
	bad.Customs = &purchase.Customs{ImportID: cu.ID}
	if _, _, err := a.SavePurchase(ctx, c.ID, bad); !fieldErr(err, "customs.importId") {
		t.Fatalf("väzba na rozhodnutie: %v", err)
	}
	bad.Customs = &purchase.Customs{Duty: m.MustParse("-1")}
	if _, _, err := a.SavePurchase(ctx, c.ID, bad); !fieldErr(err, "customs.duty") {
		t.Fatalf("záporné clo: %v", err)
	}
	withVAT := validPurchase()
	withVAT.SupplierNumber, withVAT.VariableSymbol = "CN-78", "78"
	withVAT.Supplier = invoice.Party{Name: "Shenzhen Parts Co.", Country: "CN"}
	withVAT.SupplyKind = invoice.SupplyGoods
	withVAT, _, _ = a.SavePurchase(ctx, c.ID, withVAT)
	bad.Customs = &purchase.Customs{ImportID: withVAT.ID}
	if _, _, err := a.SavePurchase(ctx, c.ID, bad); !fieldErr(err, "customs.importId") {
		t.Fatalf("faktúra dovozu s DPH: %v", err)
	}
	_ = a.Store.DeletePurchase(ctx, c.ID, withVAT.ID)

	// DPH: odpočet pri dovoze r. 19 a r. 23, bez upozornenia pri faktúre dodávateľa, nie v KV
	ret, _, err := a.VATReturn(ctx, c.ID, "2026-10", m.Zero)
	if err != nil {
		t.Fatal(err)
	}
	if ret.Rows["r19"].String() != "215.28" || ret.Rows["r23"].String() != "215.28" || len(ret.KV.B2) != 0 {
		t.Fatalf("priznanie: r19 %s r23 %s B2 %d", ret.Rows["r19"], ret.Rows["r23"], len(ret.KV.B2))
	}
	for _, w := range ret.Warnings {
		if w.ID == imp.ID || w.ID == cu.ID {
			t.Errorf("upozornenie: %+v", w)
		}
	}

	// podvojné: 501 clo, 343 DPH / 379; úhrada colnému úradu 379 / 221
	txs, _, err := a.Store.SaveBankTransactions(ctx, c.ID, []bank.Transaction{{Account: "SK3112000000198742637541", Date: invoice.MustDate("2026-10-20"),
		Amount: m.MustParse("251.28"), Currency: "EUR", Direction: bank.Out, VS: "123456", CounterName: "Colný úrad", Fingerprint: "c1", Status: bank.StatusOpen}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.MatchTransaction(ctx, c.ID, txs[0].ID, "purchase", cu.ID); err != nil {
		t.Fatal(err)
	}
	book, err := a.Ledger(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"purchase:" + itoa(int(cu.ID)):  "FP-CLO 501 MD 36.00, 343 MD 215.28, 379 D 251.28",
		"purchase:" + itoa(int(imp.ID)): "FP-MATERIAL 501 MD 900.00, 321 D 900.00",
		"bank:" + itoa(int(txs[0].ID)):  "B-COLNY 379 MD 251.28, 221 D 251.28",
	}
	for _, ps := range book.Postings {
		if w, ok := want[ps.Key]; ok {
			if got := ps.Template + " " + lineStr(ps.Lines); got != w {
				t.Errorf("%s: %s, chcem %s", ps.Key, got, w)
			}
			delete(want, ps.Key)
		}
	}
	if len(want) > 0 {
		t.Errorf("chýbajú: %v", want)
	}

	// peňažný denník: clo výdavok zásoby (e1), DPH zaplatená (g.dph)
	jb, _ := a.Journal(ctx, c.ID, 2026)
	for _, e := range jb.Entries {
		if e.Key == "bank:"+itoa(int(txs[0].ID)) {
			if len(e.Lines) != 2 || e.Lines[0].Column != journal.ExpStock || e.Lines[0].Amount.String() != "36.00" || e.Lines[1].Column != journal.ExpVAT || e.Lines[1].Amount.String() != "215.28" {
				t.Fatalf("denník: %+v", e.Lines)
			}
		}
	}
}
