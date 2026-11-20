package app

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/asset"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/logbook"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
)

func TestVehicleVATAdjustmentInReturn(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	a.Today = func() invoice.Date { return invoice.MustDate("2027-01-10") }
	car, err := a.SaveAsset(ctx, c.ID, asset.Asset{Name: "Škoda Octavia", Group: 1, Acquired: invoice.MustDate("2026-02-01"), Cost: m.MustParse("30000"),
		Disposed: invoice.MustDate("2026-11-15"), DisposalKind: asset.DisposalSale})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveVehicle(ctx, c.ID, logbook.Vehicle{Name: "Škoda Octavia", Plate: "BB123AB", Consumption: m.MustParse("5"), Category: "M1",
		AssetID: car.ID, AcquiredVAT: m.MustParse("6900")}); err != nil {
		t.Fatal(err)
	}
	list, err := a.VehicleVATAdjustments(ctx, c.ID, 0)
	if err != nil || len(list) != 1 || list[0].Year != 2026 || !list[0].Tax.Equal(m.MustParse("3450")) || list[0].Years != 5 {
		t.Fatalf("úpravy: %+v %v", list, err)
	}
	// posledné obdobie roka: dodatočný odpočet v r. 19
	r, _, err := a.VATReturn(ctx, c.ID, "2026-12", m.Zero)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Rows["r19"].Equal(m.MustParse("3450")) || len(r.Adjustments) != 1 || !r.Deduction.Equal(m.MustParse("3450")) {
		t.Fatalf("priznanie december: %+v %+v", r.Rows, r.Adjustments)
	}
	// iné obdobie bez úpravy
	r, _, err = a.VATReturn(ctx, c.ID, "2026-11", m.Zero)
	if err != nil || !r.Rows["r19"].IsZero() || len(r.Adjustments) != 0 {
		t.Fatalf("priznanie november: %+v %v", r.Rows, err)
	}
}
