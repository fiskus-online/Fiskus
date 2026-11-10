package job

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
)

func TestVehicles(t *testing.T) {
	d := invoice.MustDate
	jobs := []Job{
		{ID: 1, Kind: Auto, Status: Done, Start: d("2025-04-10"), ContactID: 3, Customer: "Novák",
			Vehicle: &Vehicle{Plate: "BB482KE", VIN: "TMBJJ7NE8K0123456", Make: "Škoda", Model: "Octavia", Year: 2019, Mileage: 120000, Inspection: d("2026-05-01")}},
		{ID: 2, Kind: Auto, Status: Active, Start: d("2026-11-10"), ContactID: 3, Customer: "Novák",
			Vehicle: &Vehicle{Plate: "BB482KE", Mileage: 154300, NextService: d("2027-11-01")}},
		{ID: 3, Kind: Auto, Status: Lost, Created: d("2026-06-01"), Vehicle: &Vehicle{VIN: "WVWZZZ1KZAW000001", Make: "VW"}},
		{ID: 4, Name: "Všeobecná"},
		{ID: 5, Kind: Auto, Status: Done, Start: d("2026-01-05"), Customer: "Kráľ", Vehicle: &Vehicle{Plate: "ZV111AA", Make: "Ford"}},
	}
	got := Vehicles(jobs)
	if len(got) != 3 {
		t.Fatalf("vozidlá %+v", got)
	}
	tests := []struct {
		i       int
		key     string
		visits  int
		mileage int
		last    string
		ids     int
	}{
		{0, "BB482KE", 2, 154300, "2026-11-10", 2},
		{1, "WVWZZZ1KZAW000001", 0, 0, "2026-06-01", 1},
		{2, "ZV111AA", 1, 0, "2026-01-05", 1},
	}
	for _, tt := range tests {
		c := got[tt.i]
		if c.Key != tt.key || c.Visits != tt.visits || c.Mileage != tt.mileage || c.Last.String() != tt.last || len(c.JobIDs) != tt.ids {
			t.Fatalf("%d: %+v", tt.i, c)
		}
	}
	// údaje z najnovšej zákazky, kde sú vyplnené (VIN a značka zo staršej)
	c := got[0]
	if c.LastJobID != 2 || c.VIN != "TMBJJ7NE8K0123456" || c.Make != "Škoda" || c.Year != 2019 || c.Customer != "Novák" ||
		c.Inspection.String() != "2026-05-01" || c.NextService.String() != "2027-11-01" || c.JobIDs[0] != 2 {
		t.Fatalf("BB482KE %+v", c)
	}
	if Vehicles(nil) == nil {
		t.Fatal("prázdny zoznam má byť [], nie null")
	}
}
