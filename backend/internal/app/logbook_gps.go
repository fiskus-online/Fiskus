package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/logbook"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// GPSDraft je jazda navrhnutá zo záznamu GPS, ktorú používateľ pred
// uložením skontroluje (účel, druh, km).
type GPSDraft struct {
	logbook.Trip
	FromPoint []float64 `json:"fromPoint"`
	ToPoint   []float64 `json:"toPoint"`
	Meters    int64     `json:"meters"`
	// Duplicate je jazda, ktorá už v knihe je (rovnaký deň a čas odchodu).
	Duplicate bool `json:"duplicate"`
}

// GPSPreview je výsledok načítania záznamu GPS.
type GPSPreview struct {
	Points int        `json:"points"`
	Trips  []GPSDraft `json:"trips"`
}

func placesKey(companyID int64) string { return fmt.Sprintf("logbook.places.%d", companyID) }

// GPSPlaces vráti pomenované miesta firmy (sídlo, sklad, zákazníci …).
func (a *App) GPSPlaces(ctx context.Context, companyID int64) ([]logbook.Place, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerRead); err != nil {
		return nil, err
	}
	return a.gpsPlaces(ctx, companyID)
}

func (a *App) gpsPlaces(ctx context.Context, companyID int64) ([]logbook.Place, error) {
	out := []logbook.Place{}
	raw, err := a.Store.GetSetting(ctx, placesKey(companyID))
	if errors.Is(err, store.ErrNotFound) || (err == nil && raw == "") {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	return out, json.Unmarshal([]byte(raw), &out)
}

// SaveGPSPlaces uloží zoznam pomenovaných miest.
func (a *App) SaveGPSPlaces(ctx context.Context, companyID int64, places []logbook.Place) ([]logbook.Place, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return nil, err
	}
	out := make([]logbook.Place, 0, len(places))
	for i, p := range places {
		p.Name = strings.TrimSpace(p.Name)
		if p.Name == "" {
			return nil, vErr(fmt.Sprintf("places[%d].name", i), "Zadajte názov miesta")
		}
		if p.Lat < -90 || p.Lat > 90 || p.Lon < -180 || p.Lon > 180 || (p.Lat == 0 && p.Lon == 0) {
			return nil, vErr(fmt.Sprintf("places[%d]", i), "Neplatné súradnice miesta "+p.Name)
		}
		if p.Radius <= 0 {
			p.Radius = 300
		}
		out = append(out, p)
	}
	raw, _ := json.Marshal(out)
	if err := a.Store.SetSetting(ctx, placesKey(companyID), string(raw)); err != nil {
		return nil, err
	}
	a.record(ctx, companyID, audit.CatLedger, "places", "places", 0, fmt.Sprintf("Kniha jázd: %d pomenovaných miest", len(out)), nil, nil)
	return out, nil
}

// PreviewGPSTrips rozdelí záznam GPS (GPX alebo CSV) na jazdy vozidla.
// Nič neukladá; stav tachometra nadväzuje na poslednú jazdu v knihe.
func (a *App) PreviewGPSTrips(ctx context.Context, companyID, vehicleID int64, data []byte) (GPSPreview, error) {
	var res GPSPreview
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return res, err
	}
	v, err := a.activeVehicle(ctx, companyID, vehicleID)
	if err != nil {
		return res, err
	}
	pts, err := logbook.ParseTrack(data)
	switch {
	case errors.Is(err, logbook.ErrGPSFormat):
		return res, vErr("file", "Súbor nie je záznam GPS (GPX alebo CSV so stĺpcami čas, šírka, dĺžka)")
	case errors.Is(err, logbook.ErrGPSEmpty):
		return res, vErr("file", "V súbore nie sú žiadne body s časom a polohou")
	case err != nil:
		return res, err
	}
	places, err := a.gpsPlaces(ctx, companyID)
	if err != nil {
		return res, err
	}
	trips, err := a.Store.ListTrips(ctx, companyID, vehicleID, 0)
	if err != nil {
		return res, err
	}
	res.Points = len(pts)
	res.Trips = []GPSDraft{}
	have := map[string]bool{}
	for _, t := range trips {
		have[t.Date.String()+" "+t.Depart] = true
	}
	odo := int64(-1)
	for _, g := range logbook.SplitTrips(pts, places) {
		start, end := g.Start.In(logbook.Local()), g.End.In(logbook.Local())
		d := GPSDraft{
			Trip: logbook.Trip{VehicleID: vehicleID, Date: invoice.NewDate(start.Year(), start.Month(), start.Day()),
				Depart: start.Format("15:04"), Arrive: end.Format("15:04"), From: g.FromName, To: g.ToName,
				Kind: logbook.TripBusiness, Driver: v.Driver, Km: max(1, int64(math.Round(g.Meters/1000)))},
			FromPoint: []float64{g.From.Lat, g.From.Lon}, ToPoint: []float64{g.To.Lat, g.To.Lon}, Meters: int64(math.Round(g.Meters)),
		}
		d.Duplicate = have[d.Date.String()+" "+d.Depart]
		if !d.Duplicate {
			if odo < 0 {
				odo = logbook.NextOdometer(v, trips, d.Date)
			}
			if odo > 0 {
				d.OdoStart, d.OdoEnd = odo, odo+d.Km
				odo = d.OdoEnd
			}
		}
		res.Trips = append(res.Trips, d)
	}
	return res, nil
}

// SaveGPSTrips uloží skontrolované jazdy naraz: najprv overí všetky, potom
// ich zapíše v poradí (stav tachometra nadväzuje).
func (a *App) SaveGPSTrips(ctx context.Context, companyID, vehicleID int64, trips []logbook.Trip) ([]logbook.Trip, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return nil, err
	}
	v, err := a.activeVehicle(ctx, companyID, vehicleID)
	if err != nil {
		return nil, err
	}
	if len(trips) == 0 {
		return nil, vErr("trips", "Vyberte aspoň jednu jazdu")
	}
	for i := range trips {
		t := &trips[i]
		t.ID, t.VehicleID = 0, vehicleID
		if t.Driver == "" {
			t.Driver = v.Driver
		}
		t.Normalize()
		if err := t.Validate(); err != nil {
			var ve *invoice.ValidationError
			if errors.As(err, &ve) {
				for j := range ve.Fields {
					ve.Fields[j].Field = fmt.Sprintf("trips[%d].%s", i, ve.Fields[j].Field)
				}
			}
			return nil, err
		}
	}
	out := make([]logbook.Trip, 0, len(trips))
	for _, t := range trips {
		saved, err := a.SaveTrip(ctx, companyID, t)
		if err != nil {
			return out, err
		}
		out = append(out, saved)
	}
	return out, nil
}
