package stock

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
)

func TestPurchaseHistory(t *testing.T) {
	d, m := invoice.MustDate, money.MustParse
	rc := func(id int64, date, partner string, lines ...Line) Doc {
		return Doc{ID: id, Kind: Receipt, Date: d(date), Partner: partner, Number: "PR" + date, Lines: lines}
	}
	docs := []Doc{
		rc(1, "2026-03-01", "Elektro Hron", Line{ItemID: 7, Quantity: m("100"), UnitCost: m("1.20")}),
		rc(2, "2026-06-01", "Elektro Hron ", Line{ItemID: 7, Quantity: m("50"), UnitCost: m("1.50")}, Line{ItemID: 8, Quantity: m("1"), UnitCost: m("9")}),
		rc(3, "2026-05-01", "Sonepar", Line{ItemID: 7, Quantity: m("40"), UnitCost: m("1.35")}),
		rc(4, "2026-07-01", "", Line{ItemID: 7, Quantity: m("10"), UnitCost: m("0")}), // bez ceny
		{ID: 5, Kind: Receipt, Return: true, Date: d("2026-08-01"), Lines: []Line{{ItemID: 7, Quantity: m("2"), UnitCost: m("1.3")}}},
		{ID: 6, Kind: Issue, Date: d("2026-08-02"), Lines: []Line{{ItemID: 7, Quantity: m("20"), UnitCost: m("1.3")}}},
	}
	entries, sums := PurchaseHistory(docs, 7)
	if len(entries) != 3 || entries[0].DocID != 2 || entries[2].DocID != 1 {
		t.Fatalf("nákupy %+v", entries)
	}
	tests := []struct {
		i                   int
		supplier            string
		last, min, max, avg string
		count               int
	}{
		{0, "Sonepar", "1.35", "1.35", "1.35", "1.35", 1},
		// Elektro Hron: (100 × 1,20 + 50 × 1,50) / 150 = 1,30; posledná 1,50 (medzera v názve nevadí)
		{1, "Elektro Hron", "1.50", "1.20", "1.50", "1.30", 2},
	}
	if len(sums) != 2 {
		t.Fatalf("dodávatelia %+v", sums)
	}
	for _, tt := range tests {
		s := sums[tt.i]
		if s.Supplier != tt.supplier || s.Last.String() != tt.last || s.Min.String() != tt.min || s.Max.String() != tt.max ||
			s.Average.String() != tt.avg || s.Count != tt.count {
			t.Fatalf("%d: %+v", tt.i, s)
		}
	}
	if e, s := PurchaseHistory(nil, 7); e == nil || s == nil || len(e) != 0 {
		t.Fatal("prázdne zoznamy majú byť [], nie null")
	}
}
