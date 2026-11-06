package app

import (
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/asset"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func TestAccDepreciation(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	var ve *invoice.ValidationError
	if _, err := a.AccDepreciationFor(ctx, c.ID, 2026); !errors.As(err, &ve) {
		t.Fatalf("jednoduché účtovníctvo: %v", err)
	}
	c.Bookkeeping = store.BookkeepingDouble
	if _, err := a.UpdateCompany(ctx, c); err != nil {
		t.Fatal(err)
	}
	m := money.MustParse
	stroj, err := a.SaveAsset(ctx, c.ID, asset.Asset{Name: "Stroj", Kind: asset.Tangible, Group: 1, Acquired: invoice.MustDate("2025-03-15"),
		Cost: m("4800"), AccMonths: 48})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveAsset(ctx, c.ID, asset.Asset{Name: "Program", Kind: asset.Intangible, Years: 4, Acquired: invoice.MustDate("2025-01-10"),
		Cost: m("3600"), Disposed: invoice.MustDate("2026-07-01"), DisposalKind: asset.DisposalSale}); err != nil {
		t.Fatal(err)
	}
	dp, err := a.AccDepreciationFor(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if len(dp.Items) != 2 || !dp.Total.Equal(m("1200")) {
		t.Fatalf("odpisy 2026: %+v", dp)
	}
	// program: ako daňové 900 v 2025, predaný v 2026 → ZC 2 700 do 541, vyradenie 073 / 013
	want := []struct {
		acc           string
		debit, credit string
	}{
		{"551", "1200", "0"}, {"082", "0", "1200"},
		{"541", "2700", "0"}, {"073", "0", "2700"},
		{"073", "3600", "0"}, {"013", "0", "3600"},
	}
	if len(dp.Lines) != len(want) {
		t.Fatalf("riadky: %+v", dp.Lines)
	}
	for i, w := range want {
		l := dp.Lines[i]
		if l.Account != w.acc || !l.Debit.Equal(m(w.debit)) || !l.Credit.Equal(m(w.credit)) {
			t.Errorf("riadok %d: %+v, chcem %+v", i, l, w)
		}
	}
	doc, err := a.PostAccDepreciation(ctx, c.ID, 2026)
	if err != nil || !doc.Dep || doc.Number == "" {
		t.Fatalf("zaúčtovanie: %+v %v", doc, err)
	}
	again, err := a.PostAccDepreciation(ctx, c.ID, 2026) // nahradí, nepridá
	if err != nil || again.Number != doc.Number {
		t.Fatalf("opakované zaúčtovanie: %+v %v", again, err)
	}
	book, err := a.Ledger(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, x := range book.Internal {
		if x.Dep {
			n++
		}
	}
	if n != 1 {
		t.Errorf("interných dokladov s odpismi: %d", n)
	}
	if dp, _ = a.AccDepreciationFor(ctx, c.ID, 2026); dp.Posted != doc.Number {
		t.Errorf("zaúčtované: %q", dp.Posted)
	}
	reg, _ := a.Assets(ctx, c.ID, 2026)
	for _, it := range reg.Items {
		if it.ID == stroj.ID && (it.DepAccount != "082" || !it.Acc.Residual.Equal(m("2700"))) {
			t.Errorf("register: %+v", it.Acc)
		}
	}
	if _, err := a.PostAccDepreciation(ctx, c.ID, 2031); !errors.As(err, &ve) {
		t.Errorf("rok bez odpisov: %v", err)
	}
}
