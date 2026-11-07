package api

import (
	"fmt"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
)

func TestLogbookAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d", e.cid)
	var v struct{ ID int64 }
	e.do("POST", base+"/vehicles", `{"name":"Octavia","plate":"BB123AB"}`, 422, nil) // bez spotreby
	e.do("POST", base+"/vehicles", `{"name":"Octavia","plate":"BB123AB","consumption":"5","ownership":"private"}`, 201, &v)
	var tr struct{ ID, Km int64 }
	e.do("POST", base+"/trips", fmt.Sprintf(`{"vehicleId":%d,"date":"2026-10-01","from":"BB","to":"ZV","purpose":"Montáž","odoStart":100,"odoEnd":125}`, v.ID), 201, &tr)
	if tr.Km != 25 {
		t.Fatalf("jazda: %+v", tr)
	}
	e.do("PUT", fmt.Sprintf("%s/trips/%d", base, tr.ID), fmt.Sprintf(`{"vehicleId":%d,"date":"2026-10-01","from":"BB","to":"ZV","purpose":"Montáž","odoStart":100,"odoEnd":130}`, v.ID), 200, &tr)
	e.do("POST", base+"/fuel", fmt.Sprintf(`{"vehicleId":%d,"date":"2026-10-01","quantity":"10","amount":"16.00"}`, v.ID), 201, nil)
	var trips []struct{ Km int64 }
	e.do("GET", fmt.Sprintf("%s/vehicles/%d/trips?year=2026", base, v.ID), nil, 200, &trips)
	if len(trips) != 1 || trips[0].Km != 30 {
		t.Fatalf("jazdy: %+v", trips)
	}
	var rep struct {
		BaseAllowance string
		FuelExpense   string
		Posting       string
	}
	e.do("GET", fmt.Sprintf("%s/vehicles/%d/report?year=2026", base, v.ID), nil, 200, &rep)
	// 30 × 0,313 = 9,39; 30 × 5/100 × 1,6 = 2,40
	if rep.BaseAllowance != "9.39" || rep.FuelExpense != "2.40" || rep.Posting != "11.79" {
		t.Fatalf("vyúčtovanie: %+v", rep)
	}
	e.do("POST", fmt.Sprintf("%s/vehicles/%d/report/2026", base, v.ID), nil, 200, nil)
	e.do("GET", base+"/fuel-docs?year=2026", nil, 200, nil)
	e.do("DELETE", fmt.Sprintf("%s/vehicles/%d", base, v.ID), nil, 422, nil)
	e.do("DELETE", fmt.Sprintf("%s/trips/%d", base, tr.ID), nil, 204, nil)
	e.do("GET", fmt.Sprintf("%s/vehicles/%d/trips", base, 999), nil, 404, nil)
}

func TestGPSAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d", e.cid)
	var v struct{ ID int64 }
	e.do("POST", base+"/vehicles", `{"name":"Octavia","plate":"BB123AB","consumption":"5","odometer":5000,"odometerDate":"2026-01-01"}`, 201, &v)
	e.do("PUT", base+"/logbook/places", `[{"name":"Sídlo","lat":48.7363,"lon":19.1462}]`, 200, nil)
	var places []struct{ Name string }
	e.do("GET", base+"/logbook/places", nil, 200, &places)
	if len(places) != 1 || places[0].Name != "Sídlo" {
		t.Fatalf("miesta: %+v", places)
	}
	csv := "time,lat,lon\n2026-11-03T06:30:00Z,48.7363,19.1462\n2026-11-03T06:40:00Z,48.70,19.14\n2026-11-03T06:50:00Z,48.5767,19.1263\n"
	var p struct {
		Points int
		Trips  []map[string]any
	}
	e.do("POST", fmt.Sprintf("%s/vehicles/%d/gps", base, v.ID), csv, 200, &p)
	if p.Points != 3 || len(p.Trips) != 1 || p.Trips[0]["from"] != "Sídlo" || p.Trips[0]["odoStart"] != 5000.0 {
		t.Fatalf("náhľad: %+v", p)
	}
	e.do("POST", fmt.Sprintf("%s/vehicles/%d/gps", base, v.ID), "a,b\n", 422, nil)
	p.Trips[0]["purpose"] = "Nákup materiálu"
	var saved []struct{ ID, OdoEnd int64 }
	e.do("POST", fmt.Sprintf("%s/vehicles/%d/gps/trips", base, v.ID), p.Trips, 201, &saved)
	if len(saved) != 1 || saved[0].ID == 0 || saved[0].OdoEnd != 5018 {
		t.Fatalf("uložené: %+v", saved)
	}
}
