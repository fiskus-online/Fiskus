package logbook

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
)

func TestVATAdjustments(t *testing.T) {
	d := invoice.MustDate
	acq := Acquisition{FirstUse: d("2026-03-10"), VAT: money.MustParse("9200"), Rate: 23}
	car := Vehicle{ID: 1, Name: "Škoda", Plate: "BB123AA", Category: "M1"}
	type adj struct {
		year  int
		tax   string
		years int
	}
	cases := []struct {
		name string
		v    Vehicle
		a    Acquisition
		want []adj
	}{
		{"bez zmeny: nič", car, acq, nil},
		{"predaj v 3. roku: DV × 0,5 × 3 / 5", car, Acquisition{FirstUse: acq.FirstUse, VAT: acq.VAT, Rate: 23, Sold: d("2028-07-01")}, []adj{{2028, "2760.00", 3}}},
		{"predaj v roku nadobudnutia: celé obdobie", car, Acquisition{FirstUse: acq.FirstUse, VAT: acq.VAT, Rate: 23, Sold: d("2026-12-01")}, []adj{{2026, "4600.00", 5}}},
		{"predaj po skončení obdobia: nič", car, Acquisition{FirstUse: acq.FirstUse, VAT: acq.VAT, Rate: 23, Sold: d("2031-01-05")}, nil},
		{"výlučne na podnikanie od 2028: každý rok 1/10 DV", Vehicle{ID: 1, Name: "Škoda", Category: "M1", FullVAT: true, FullVATFrom: 2028}, acq,
			[]adj{{2028, "920.00", 1}, {2029, "920.00", 1}, {2030, "920.00", 1}}},
		{"výlučne od 2028 a predaj 2029", Vehicle{ID: 1, Name: "Škoda", Category: "M1", FullVAT: true, FullVATFrom: 2028},
			Acquisition{FirstUse: acq.FirstUse, VAT: acq.VAT, Rate: 23, Sold: d("2029-05-01")}, []adj{{2028, "920.00", 1}, {2029, "1840.00", 2}}},
		{"plný odpočet od začiatku: nič", Vehicle{ID: 1, Name: "Škoda", Category: "M1", FullVAT: true}, Acquisition{FirstUse: acq.FirstUse, VAT: acq.VAT, Rate: 23, Sold: d("2028-07-01")}, nil},
		{"úžitkové N1: nič", Vehicle{ID: 1, Name: "Dodávka", Category: "N1"}, Acquisition{FirstUse: acq.FirstUse, VAT: acq.VAT, Rate: 23, Sold: d("2028-07-01")}, nil},
		{"kúpené pred 2026: nič", car, Acquisition{FirstUse: d("2025-12-20"), VAT: acq.VAT, Rate: 23, Sold: d("2027-07-01")}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.v.VATAdjustments(c.a)
			if len(got) != len(c.want) {
				t.Fatalf("%+v", got)
			}
			for i, w := range c.want {
				if got[i].Year != w.year || !got[i].Tax.Equal(money.MustParse(w.tax)) || got[i].Years != w.years {
					t.Errorf("%d: %+v, chcem %+v", i, got[i], w)
				}
			}
		})
	}
}

func TestVATPercentFrom(t *testing.T) {
	v := Vehicle{Category: "M1", FullVAT: true, FullVATFrom: 2028}
	cases := []struct {
		date string
		want int
	}{{"2026-05-01", 50}, {"2027-12-31", 50}, {"2028-01-01", 100}}
	for _, c := range cases {
		if got := v.VATPercent(invoice.MustDate(c.date)); got != c.want {
			t.Errorf("%s: %d", c.date, got)
		}
	}
}
