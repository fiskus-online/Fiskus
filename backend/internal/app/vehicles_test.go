package app

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/calendar"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/job"
)

func TestVehicleCalendar(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	today := a.Today()
	j, err := a.SaveCrmJob(ctx, c.ID, job.Job{Name: "Brzdy", Status: job.Active, Kind: job.Auto, Start: today,
		Vehicle: &job.Vehicle{Plate: "bb 482 ke", Make: "Škoda", Model: "Octavia", Inspection: today.AddDays(20), NextService: today.AddDays(-60)}})
	if err != nil {
		t.Fatal(err)
	}
	vs, err := a.CrmVehicles(ctx, c.ID)
	if err != nil || len(vs) != 1 || vs[0].Plate != "BB482KE" || vs[0].LastJobID != j.ID {
		t.Fatalf("vozidlá %+v %v", vs, err)
	}
	es, err := a.vehicleEvents(ctx, c.ID, today.AddDays(-31), today.AddDays(90), today)
	if err != nil {
		t.Fatal(err)
	}
	// servis pred 60 dňami je mimo rozsahu, STK o 20 dní v ňom
	if len(es) != 1 || es[0].Kind != calendar.KindVehicle || es[0].Title != "STK/EK – BB482KE Škoda Octavia" || es[0].Overdue ||
		es[0].Date != today.AddDays(20) || es[0].Link == "" {
		t.Fatalf("termíny %+v", es)
	}
	cv, err := a.Calendar(ctx, c.ID, today, today.AddDays(60))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range cv.Events {
		found = found || e.Kind == calendar.KindVehicle
	}
	if !found {
		t.Fatal("STK nie je v kalendári")
	}
}
