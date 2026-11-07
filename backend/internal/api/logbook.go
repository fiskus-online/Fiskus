package api

import (
	"io"
	"net/http"
	"strconv"

	"github.com/fiskus-online/fiskus/backend/internal/logbook"
)

func (s *Server) yearParam(r *http.Request) int {
	y, _ := strconv.Atoi(r.URL.Query().Get("year"))
	if y == 0 {
		y = s.App.Today().Year()
	}
	return y
}

func (s *Server) listVehicles(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	out, err := s.App.Vehicles(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// saveEntity spracuje POST (nový) a PUT …/{id} (úprava) záznamu knihy jázd.
func saveEntity[T any](s *Server, w http.ResponseWriter, r *http.Request, setID func(*T, int64), save func(cid int64, x T) (T, error)) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var x T
	if !decode(w, r, &x) {
		return
	}
	setID(&x, 0)
	status := http.StatusCreated
	if r.PathValue("id") != "" {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		setID(&x, id)
		status = http.StatusOK
	}
	out, err := save(cid, x)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, status, out)
}

func (s *Server) deleteEntity(w http.ResponseWriter, r *http.Request, del func(cid, id int64) error) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := del(cid, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) saveVehicle(w http.ResponseWriter, r *http.Request) {
	saveEntity(s, w, r, func(v *logbook.Vehicle, id int64) { v.ID = id }, func(cid int64, v logbook.Vehicle) (logbook.Vehicle, error) {
		return s.App.SaveVehicle(r.Context(), cid, v)
	})
}

func (s *Server) deleteVehicle(w http.ResponseWriter, r *http.Request) {
	s.deleteEntity(w, r, func(cid, id int64) error { return s.App.DeleteVehicle(r.Context(), cid, id) })
}

func (s *Server) listTrips(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	out, err := s.App.Trips(r.Context(), cid, id, s.yearParam(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) saveTrip(w http.ResponseWriter, r *http.Request) {
	saveEntity(s, w, r, func(t *logbook.Trip, id int64) { t.ID = id }, func(cid int64, t logbook.Trip) (logbook.Trip, error) {
		return s.App.SaveTrip(r.Context(), cid, t)
	})
}

func (s *Server) deleteTrip(w http.ResponseWriter, r *http.Request) {
	s.deleteEntity(w, r, func(cid, id int64) error { return s.App.DeleteTrip(r.Context(), cid, id) })
}

func (s *Server) listFuel(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	out, err := s.App.Fuels(r.Context(), cid, id, s.yearParam(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) saveFuel(w http.ResponseWriter, r *http.Request) {
	saveEntity(s, w, r, func(f *logbook.Fuel, id int64) { f.ID = id }, func(cid int64, f logbook.Fuel) (logbook.Fuel, error) {
		return s.App.SaveFuel(r.Context(), cid, f)
	})
}

func (s *Server) deleteFuel(w http.ResponseWriter, r *http.Request) {
	s.deleteEntity(w, r, func(cid, id int64) error { return s.App.DeleteFuel(r.Context(), cid, id) })
}

func (s *Server) fuelDocs(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	out, err := s.App.FuelDocs(r.Context(), cid, s.yearParam(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) vehicleReport(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	out, err := s.App.VehicleReport(r.Context(), cid, id, s.yearParam(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) postLogbook(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	year, err := strconv.Atoi(r.PathValue("year"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Neplatný rok", nil)
		return
	}
	sum, err := s.App.PostLogbook(r.Context(), cid, id, year)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"posted": sum})
}

// gpsPreview: POST /api/companies/{cid}/vehicles/{id}/gps (telo = GPX alebo CSV)
func (s *Server) gpsPreview(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxStatement))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "Súbor je príliš veľký (najviac 20 MB)", nil)
		return
	}
	out, err := s.App.PreviewGPSTrips(r.Context(), cid, id, data)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) gpsSave(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var trips []logbook.Trip // návrhy z náhľadu môžu niesť ďalšie polia
	if !decodeLoose(w, r, &trips) {
		return
	}
	out, err := s.App.SaveGPSTrips(r.Context(), cid, id, trips)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) gpsPlaces(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	out, err := s.App.GPSPlaces(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) gpsSavePlaces(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var places []logbook.Place
	if !decode(w, r, &places) {
		return
	}
	out, err := s.App.SaveGPSPlaces(r.Context(), cid, places)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
