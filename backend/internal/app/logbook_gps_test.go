package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/logbook"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
)

// gpsCSV je jazda Banská Bystrica → Zvolen (≈ 18 km) a po 40 min späť.
func gpsCSV() []byte {
	var b strings.Builder
	b.WriteString("time,lat,lon\n")
	line := func(min int, f float64, back bool) {
		if back {
			f = 1 - f
		}
		fmt.Fprintf(&b, "2026-11-03T%02d:%02d:00Z,%.6f,%.6f\n", 6+min/60, min%60, 48.7363+(48.5767-48.7363)*f, 19.1462+(19.1263-19.1462)*f)
	}
	for i := 0; i <= 20; i++ { // 6:30–6:50 UTC tam
		line(30+i, float64(i)/20, false)
	}
	for i := 1; i <= 40; i++ { // státie
		line(50+i, 1, false)
	}
	for i := 1; i <= 20; i++ { // späť 7:31–7:50 UTC
		line(90+i, float64(i)/20, true)
	}
	return []byte(b.String())
}

func fieldErrPrefix(err error, prefix string) bool {
	var ve *invoice.ValidationError
	if !errors.As(err, &ve) {
		return false
	}
	for _, f := range ve.Fields {
		if strings.HasPrefix(f.Field, prefix) {
			return true
		}
	}
	return false
}

func TestGPSTrips(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	v, err := a.SaveVehicle(ctx, c.ID, logbook.Vehicle{Name: "Škoda Octavia", Plate: "BB123AB", Consumption: m.MustParse("5"),
		Driver: "Ján Novák", Odometer: 1000, OdometerDate: invoice.MustDate("2026-01-01")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveGPSPlaces(ctx, c.ID, []logbook.Place{{Name: " Sídlo ", Lat: 48.7363, Lon: 19.1462}, {Name: "Sklad Zvolen", Lat: 48.5767, Lon: 19.1263}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveGPSPlaces(ctx, c.ID, []logbook.Place{{Name: "", Lat: 48, Lon: 19}}); !fieldErr(err, "places[0].name") {
		t.Fatalf("miesto bez názvu: %v", err)
	}
	if _, err := a.PreviewGPSTrips(ctx, c.ID, v.ID, []byte("nič")); !fieldErr(err, "file") {
		t.Fatalf("zlý súbor: %v", err)
	}

	p, err := a.PreviewGPSTrips(ctx, c.ID, v.ID, gpsCSV())
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		date, dep, arr, from, to string
		km, odo                  int64
		dup                      bool
	}
	got := func(p GPSPreview) []row {
		var out []row
		for _, d := range p.Trips {
			out = append(out, row{d.Date.String(), d.Depart, d.Arrive, d.From, d.To, d.Km, d.OdoStart, d.Duplicate})
		}
		return out
	}
	want := []row{{"2026-11-03", "07:30", "07:50", "Sídlo", "Sklad Zvolen", 18, 1000, false}, {"2026-11-03", "08:30", "08:50", "Sklad Zvolen", "Sídlo", 18, 1018, false}}
	if g := got(p); len(g) != 2 || g[0] != want[0] || g[1] != want[1] || p.Points != 81 {
		t.Fatalf("náhľad %d bodov:\n %+v\nchcem %+v", p.Points, g, want)
	}

	// bez účelu sa neuloží nič
	if _, err := a.SaveGPSTrips(ctx, c.ID, v.ID, []logbook.Trip{p.Trips[0].Trip, p.Trips[1].Trip}); err == nil || !fieldErrPrefix(err, "trips[0].") {
		t.Fatalf("bez účelu: %v", err)
	}
	if list, _ := a.Trips(ctx, c.ID, v.ID, 2026); len(list) != 0 {
		t.Fatalf("uložené napriek chybe: %d", len(list))
	}
	first := p.Trips[0].Trip
	first.Purpose = "Odvoz materiálu"
	if _, err := a.SaveGPSTrips(ctx, c.ID, v.ID, []logbook.Trip{first}); err != nil {
		t.Fatal(err)
	}

	// opakované načítanie: prvá jazda je už v knihe, druhá nadväzuje
	p, err = a.PreviewGPSTrips(ctx, c.ID, v.ID, gpsCSV())
	if err != nil {
		t.Fatal(err)
	}
	g := got(p)
	if !g[0].dup || g[0].odo != 0 || g[1].dup || g[1].odo != 1018 {
		t.Fatalf("opakovaný náhľad: %+v", g)
	}
}
