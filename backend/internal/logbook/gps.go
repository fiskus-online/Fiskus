package logbook

import (
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // časové pásmo Europe/Bratislava aj vo Windows a v prehliadači
)

// Kniha jázd z GPS: záznam trasy (GPX z mobilnej aplikácie, navigácie alebo
// GPS jednotky vo vozidle, prípadne CSV s časom a súradnicami) sa rozdelí
// na jazdy podľa zastávok. Vzdialenosť je súčet úsekov po povrchu Zeme;
// skoky GPS (nereálna rýchlosť) sa vynechajú. Miesta odchodu a príchodu
// sa pomenujú podľa uložených miest, ostatné ostanú ako súradnice.

// Point je bod trasy.
type Point struct {
	Time     time.Time
	Lat, Lon float64
}

// Place je pomenované miesto (sídlo, sklad, zákazník) na pomenovanie jázd.
type Place struct {
	Name   string  `json:"name"`
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	Radius int     `json:"radius"` // metre; 0 = 300 m
}

// GPSTrip je jazda nájdená v trase.
type GPSTrip struct {
	Start, End time.Time
	From, To   Point
	FromName   string
	ToName     string
	Meters     float64
	Points     int
}

var (
	// ErrGPSFormat — súbor nie je GPX ani CSV so súradnicami.
	ErrGPSFormat = errors.New("súbor nie je trasa GPX ani CSV s časom a súradnicami (stĺpce čas, šírka, dĺžka)")
	// ErrGPSEmpty — v trase nie sú body s časom.
	ErrGPSEmpty = errors.New("v trase nie sú body s časom")
)

// ParseTrack prečíta body trasy z GPX alebo CSV (zoradené podľa času).
func ParseTrack(data []byte) ([]Point, error) {
	t := bytes.TrimSpace(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
	var pts []Point
	var err error
	if bytes.HasPrefix(t, []byte("<")) {
		pts, err = parseGPX(t)
	} else {
		pts, err = parseCSV(t)
	}
	if err != nil {
		return nil, err
	}
	if len(pts) == 0 {
		return nil, ErrGPSEmpty
	}
	sort.SliceStable(pts, func(i, j int) bool { return pts[i].Time.Before(pts[j].Time) })
	return pts, nil
}

func parseGPX(data []byte) ([]Point, error) {
	var g struct {
		XMLName xml.Name
		Tracks  []struct {
			Segs []struct {
				Pts []struct {
					Lat  float64 `xml:"lat,attr"`
					Lon  float64 `xml:"lon,attr"`
					Time string  `xml:"time"`
				} `xml:"trkpt"`
			} `xml:"trkseg"`
		} `xml:"trk"`
	}
	if err := xml.Unmarshal(data, &g); err != nil || g.XMLName.Local != "gpx" {
		return nil, ErrGPSFormat
	}
	var out []Point
	for _, tr := range g.Tracks {
		for _, s := range tr.Segs {
			for _, p := range s.Pts {
				tm, err := time.Parse(time.RFC3339, strings.TrimSpace(p.Time))
				if err != nil || !validCoord(p.Lat, p.Lon) {
					continue
				}
				out = append(out, Point{Time: tm, Lat: p.Lat, Lon: p.Lon})
			}
		}
	}
	return out, nil
}

// parseCSV číta stĺpce podľa hlavičky: čas (time, timestamp, date, čas),
// šírka (lat, latitude, šírka) a dĺžka (lon, lng, longitude, dĺžka).
func parseCSV(data []byte) ([]Point, error) {
	sep := ','
	if first, _, _ := bytes.Cut(data, []byte("\n")); bytes.Count(first, []byte(";")) > bytes.Count(first, []byte(",")) {
		sep = ';'
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma, r.FieldsPerRecord, r.TrimLeadingSpace = sep, -1, true
	rows, err := r.ReadAll()
	if err != nil || len(rows) < 2 {
		return nil, ErrGPSFormat
	}
	col := func(names ...string) int {
		for i, h := range rows[0] {
			h = strings.ToLower(strings.TrimSpace(h))
			for _, n := range names {
				if h == n {
					return i
				}
			}
		}
		return -1
	}
	ti, la, lo := col("time", "timestamp", "date", "datetime", "čas", "cas"), col("lat", "latitude", "šírka", "sirka"), col("lon", "lng", "long", "longitude", "dĺžka", "dlzka")
	if ti < 0 || la < 0 || lo < 0 {
		return nil, ErrGPSFormat
	}
	var out []Point
	for _, row := range rows[1:] {
		if len(row) <= max(ti, la, lo) {
			continue
		}
		tm, ok := parseTime(row[ti])
		lat, e1 := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(row[la]), ",", "."), 64)
		lon, e2 := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(row[lo]), ",", "."), 64)
		if !ok || e1 != nil || e2 != nil || !validCoord(lat, lon) {
			continue
		}
		out = append(out, Point{Time: tm, Lat: lat, Lon: lon})
	}
	return out, nil
}

var csvTimes = []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04", "2.1.2006 15:04:05", "2.1.2006 15:04", "02.01.2006 15:04:05", "02.01.2006 15:04"}

// parseTime: ISO s pásmom, miestny čas (Slovensko) alebo Unix čas v sekundách/milisekundách.
func parseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n > 1e12 {
			return time.UnixMilli(n), true
		}
		return time.Unix(n, 0), true
	}
	for _, f := range csvTimes {
		if f == time.RFC3339 {
			if t, err := time.Parse(f, s); err == nil {
				return t, true
			}
			continue
		}
		if t, err := time.ParseInLocation(f, s, Local()); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func validCoord(lat, lon float64) bool {
	return lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180 && !(lat == 0 && lon == 0)
}

// Local je časové pásmo jázd (Slovensko).
func Local() *time.Location {
	if l, err := time.LoadLocation("Europe/Bratislava"); err == nil {
		return l
	}
	return time.UTC
}

// Distance je vzdialenosť dvoch bodov v metroch (haversine).
func Distance(a, b Point) float64 {
	const r = 6371000.0
	rad := math.Pi / 180
	dLat, dLon := (b.Lat-a.Lat)*rad, (b.Lon-a.Lon)*rad
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(a.Lat*rad)*math.Cos(b.Lat*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Min(1, math.Sqrt(h)))
}

const (
	stopGap  = 10 * time.Minute // státie dlhšie ako toto = koniec jazdy
	stopNear = 150.0            // pohyb do 150 m počas státia sa nepočíta
	maxSpeed = 250.0            // km/h — rýchlejší „skok“ je chyba GPS
	minTrip  = 300.0            // kratšia jazda (m) sa vynechá (presun na parkovisku)
)

// SplitTrips rozdelí trasu na jazdy: jazda končí, keď vozidlo stojí
// (v okruhu 150 m) dlhšie ako 10 minút alebo v zázname chýba úsek dlhší
// ako 10 minút.
func SplitTrips(in []Point, places []Place) []GPSTrip {
	pts := append([]Point(nil), in...)
	var out []GPSTrip
	var cur *GPSTrip
	anchor := 0               // posledný bod, kde sa vozidlo pohlo o viac ako 150 m
	atAnchor := 0.0           // prejdené metre jazdy v bode anchor
	still := 0                // posledný bod státia pred rozjazdom (čas odchodu)
	finish := func(end int) { // jazda končí v bode end (zastávka)
		if cur != nil {
			cur.Meters = atAnchor
			if cur.Meters >= minTrip {
				cur.End, cur.To = pts[end].Time, pts[end]
				cur.FromName, cur.ToName = PlaceName(cur.From, places), PlaceName(cur.To, places)
				out = append(out, *cur)
			}
		}
		cur = nil
	}
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		dt := b.Time.Sub(a.Time)
		if dt > stopGap { // výpadok záznamu = zastávka
			finish(anchor)
			anchor, atAnchor, still = i, 0, i
			continue
		}
		d := Distance(a, b)
		if dt > 0 && d/dt.Hours()/1000 > maxSpeed {
			pts[i] = a // skok GPS: bod sa nahradí predchádzajúcim
			continue
		}
		if cur != nil {
			cur.Meters += d
		}
		if Distance(pts[anchor], b) < stopNear {
			if cur != nil && b.Time.Sub(pts[anchor].Time) > stopGap {
				finish(anchor) // stojí — jazda skončila v bode anchor
				atAnchor = 0
			}
			if cur == nil {
				still = i
			}
			continue
		}
		if cur == nil { // vozidlo sa pohlo z miesta anchor
			cur = &GPSTrip{Start: pts[max(still, anchor)].Time, From: pts[anchor], Meters: Distance(pts[anchor], b)}
		}
		cur.Points++
		anchor, atAnchor = i, cur.Meters
	}
	finish(anchor)
	return out
}

// PlaceName pomenuje bod podľa najbližšieho uloženého miesta v jeho okruhu,
// inak vráti súradnice.
func PlaceName(p Point, places []Place) string {
	best, name := math.MaxFloat64, ""
	for _, pl := range places {
		r := float64(pl.Radius)
		if r <= 0 {
			r = 300
		}
		if d := Distance(p, Point{Lat: pl.Lat, Lon: pl.Lon}); d <= r && d < best {
			best, name = d, pl.Name
		}
	}
	if name != "" {
		return name
	}
	return fmt.Sprintf("%.5f, %.5f", p.Lat, p.Lon)
}
