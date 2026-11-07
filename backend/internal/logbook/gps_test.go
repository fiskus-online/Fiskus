package logbook

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

var (
	home = Point{Lat: 48.7363, Lon: 19.1462} // Banská Bystrica
	shop = Point{Lat: 48.5767, Lon: 19.1263} // Zvolen (≈ 17,8 km vzdušnou čiarou)
)

// drive vytvorí body jazdy po priamke s bodom každých 30 s.
func drive(from, to Point, start time.Time, minutes int) []Point {
	var out []Point
	n := minutes * 2
	for i := 0; i <= n; i++ {
		f := float64(i) / float64(n)
		out = append(out, Point{Time: start.Add(time.Duration(i) * 30 * time.Second), Lat: from.Lat + (to.Lat-from.Lat)*f, Lon: from.Lon + (to.Lon-from.Lon)*f})
	}
	return out
}

// stand sú body státia s chvením GPS (do 20 m).
func stand(at Point, start time.Time, minutes int) []Point {
	var out []Point
	for i := 1; i <= minutes; i++ {
		j := 0.00015 * math.Sin(float64(i))
		out = append(out, Point{Time: start.Add(time.Duration(i) * time.Minute), Lat: at.Lat + j, Lon: at.Lon - j})
	}
	return out
}

func TestSplitTrips(t *testing.T) {
	t0 := time.Date(2026, 11, 3, 6, 30, 0, 0, time.UTC) // 7:30 miestneho času
	places := []Place{{Name: "Sídlo, Banská Bystrica", Lat: home.Lat, Lon: home.Lon}, {Name: "Sklad Zvolen", Lat: shop.Lat, Lon: shop.Lon, Radius: 200}}
	there := drive(home, shop, t0, 20)
	wait := stand(shop, there[len(there)-1].Time, 40)
	back := drive(shop, home, wait[len(wait)-1].Time.Add(time.Minute), 22)
	jump := []Point{{Time: back[5].Time.Add(10 * time.Second), Lat: 49.5, Lon: 20.5}} // skok GPS

	tests := []struct {
		name  string
		pts   []Point
		trips []string // od – do, km, odchod–príchod (miestny čas)
	}{
		{"tam a späť so zastávkou", concat(stand(home, t0.Add(-10*time.Minute), 9), there, wait, back),
			[]string{"Sídlo, Banská Bystrica → Sklad Zvolen 18 km 07:30–07:50", "Sklad Zvolen → Sídlo, Banská Bystrica 18 km 08:31–08:53"}},
		{"skok GPS sa nepočíta", concat(there, wait, back[:6], jump, back[6:]),
			[]string{"Sídlo, Banská Bystrica → Sklad Zvolen 18 km 07:30–07:50", "Sklad Zvolen → Sídlo, Banská Bystrica 18 km 08:31–08:53"}},
		{"výpadok záznamu rozdelí jazdu", concat(there, back[len(back)-3:]),
			[]string{"Sídlo, Banská Bystrica → Sklad Zvolen 18 km 07:30–07:50", "48.72905, 19.14530 → Sídlo, Banská Bystrica 1 km 08:52–08:53"}},
		{"neznáme miesto ako súradnice", drive(home, Point{Lat: 48.80, Lon: 19.30}, t0, 10),
			[]string{"Sídlo, Banská Bystrica → 48.80000, 19.30000 13 km 07:30–07:40"}},
		{"presun na parkovisku sa vynechá", drive(home, Point{Lat: home.Lat + 0.001, Lon: home.Lon}, t0, 2), nil},
		{"prázdne", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, tr := range SplitTrips(tt.pts, places) {
				got = append(got, fmt.Sprintf("%s → %s %d km %s–%s", tr.FromName, tr.ToName, int(math.Round(tr.Meters/1000)),
					tr.Start.In(Local()).Format("15:04"), tr.End.In(Local()).Format("15:04")))
			}
			if strings.Join(got, " | ") != strings.Join(tt.trips, " | ") {
				t.Errorf("jazdy:\n %v\nchcem:\n %v", got, tt.trips)
			}
		})
	}
}

func concat(parts ...[]Point) []Point {
	var out []Point
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func TestParseTrack(t *testing.T) {
	gpx := `<?xml version="1.0"?><gpx version="1.1" xmlns="http://www.topografix.com/GPX/1/1"><trk><trkseg>
<trkpt lat="48.7363" lon="19.1462"><time>2026-11-03T06:30:30Z</time></trkpt>
<trkpt lat="48.7300" lon="19.1450"><time>2026-11-03T06:30:00Z</time></trkpt>
<trkpt lat="0" lon="0"><time>2026-11-03T06:31:00Z</time></trkpt>
<trkpt lat="48.7200" lon="19.1440"></trkpt>
</trkseg></trk></gpx>`
	tests := []struct {
		name  string
		data  string
		n     int
		first string // čas prvého bodu (UTC)
		err   error
	}{
		{"GPX zoradí podľa času, vynechá bez času a 0,0", gpx, 2, "06:30:00", nil},
		{"CSV s čiarkou", "time,lat,lon\n2026-11-03T06:30:00Z,48.7363,19.1462\n2026-11-03T06:31:00Z,48.73,19.145\n", 2, "06:30:00", nil},
		{"CSV so stredníkom a miestnym časom", "čas;šírka;dĺžka\n3.11.2026 7:30;48,7363;19,1462\n", 1, "06:30:00", nil},
		{"CSV s Unix časom v ms", "timestamp,latitude,longitude\n1793687400000,48.7363,19.1462\n", 1, time.UnixMilli(1793687400000).UTC().Format("15:04:05"), nil},
		{"bez stĺpcov", "a,b\n1,2\n", 0, "", ErrGPSFormat},
		{"iné XML", "<kml></kml>", 0, "", ErrGPSFormat},
		{"GPX bez bodov", `<gpx></gpx>`, 0, "", ErrGPSEmpty},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pts, err := ParseTrack([]byte(tt.data))
			if err != tt.err {
				t.Fatalf("chyba %v, chcem %v", err, tt.err)
			}
			if len(pts) != tt.n || (tt.n > 0 && pts[0].Time.UTC().Format("15:04:05") != tt.first) {
				t.Fatalf("body %+v", pts)
			}
		})
	}
}
