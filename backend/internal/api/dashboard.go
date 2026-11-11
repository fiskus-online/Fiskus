package api

import (
	"net/http"
	"strconv"
)

// dashboard: GET /api/companies/{cid}/dashboard?year= — prehľad firmy
// (časti podľa práv používateľa, kontroluje ich aplikácia).
func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	d, err := s.App.Dashboard(r.Context(), cid, year)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}
