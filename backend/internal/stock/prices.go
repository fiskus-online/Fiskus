package stock

import (
	"sort"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
)

// PriceEntry je nákup položky: príjemka s obstarávacou cenou.
type PriceEntry struct {
	Date       invoice.Date `json:"date"`
	Supplier   string       `json:"supplier"`
	Quantity   money.Dec    `json:"quantity"`
	UnitCost   money.Dec    `json:"unitCost"`
	DocID      int64        `json:"docId"`
	DocNumber  string       `json:"docNumber"`
	LinkKind   string       `json:"linkKind,omitempty"` // porder | purchase
	LinkID     int64        `json:"linkId,omitempty"`
	LinkNumber string       `json:"linkNumber,omitempty"`
}

// SupplierPrice je súhrn nákupov položky od jedného dodávateľa.
type SupplierPrice struct {
	Supplier string       `json:"supplier"`
	Last     money.Dec    `json:"last"` // posledná cena
	LastDate invoice.Date `json:"lastDate"`
	Min      money.Dec    `json:"min"`
	Max      money.Dec    `json:"max"`
	Quantity money.Dec    `json:"quantity"` // nakúpené spolu
	Count    int          `json:"count"`
	// Average je vážený priemer ceny (hodnota / množstvo).
	Average money.Dec `json:"average"`
}

// PurchaseHistory vráti nákupy položky z príjemiek s cenou (bez vrátení
// z dobropisov), najnovšie prvé, a súhrn podľa dodávateľa (najlacnejšia
// posledná cena prvá).
func PurchaseHistory(docs []Doc, itemID int64) ([]PriceEntry, []SupplierPrice) {
	entries := []PriceEntry{}
	for _, d := range docs {
		if d.Kind != Receipt || d.Return {
			continue
		}
		for _, l := range d.Lines {
			if l.ItemID != itemID || l.Quantity.Sign() <= 0 || l.UnitCost.Sign() <= 0 {
				continue
			}
			entries = append(entries, PriceEntry{Date: d.Date, Supplier: strings.TrimSpace(d.Partner), Quantity: l.Quantity, UnitCost: l.UnitCost,
				DocID: d.ID, DocNumber: d.Number, LinkKind: d.LinkKind, LinkID: d.LinkID, LinkNumber: d.LinkNumber})
		}
	}
	sort.SliceStable(entries, func(i, k int) bool {
		if entries[i].Date != entries[k].Date {
			return entries[k].Date.Before(entries[i].Date)
		}
		return entries[i].DocID > entries[k].DocID
	})
	idx := map[string]int{}
	values := map[string]money.Dec{}
	sums := []SupplierPrice{}
	for _, e := range entries { // najnovšie prvé: prvý výskyt je posledná cena
		key := strings.ToLower(e.Supplier)
		i, ok := idx[key]
		if !ok {
			idx[key] = len(sums)
			i = len(sums)
			sums = append(sums, SupplierPrice{Supplier: e.Supplier, Last: e.UnitCost, LastDate: e.Date, Min: e.UnitCost, Max: e.UnitCost})
		}
		s := &sums[i]
		if e.UnitCost.Cmp(s.Min) < 0 {
			s.Min = e.UnitCost
		}
		if e.UnitCost.Cmp(s.Max) > 0 {
			s.Max = e.UnitCost
		}
		s.Quantity = s.Quantity.Add(e.Quantity)
		s.Count++
		values[key] = values[key].Add(e.Quantity.Mul(e.UnitCost))
	}
	for i := range sums {
		s := &sums[i]
		if s.Quantity.Sign() > 0 {
			s.Average = values[strings.ToLower(s.Supplier)].DivCents(s.Quantity)
		}
	}
	sort.SliceStable(sums, func(i, k int) bool { return sums[i].Last.Cmp(sums[k].Last) < 0 })
	return entries, sums
}
