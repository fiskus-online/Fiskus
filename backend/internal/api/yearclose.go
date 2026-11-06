package api

import (
	"net/http"
	"strconv"
)

// Uzávierka roka (jednoduché účtovníctvo): /api/companies/{cid}/year-close/{year}

func (s *Server) yearCloseYear(w http.ResponseWriter, r *http.Request) (int64, int, bool) {
	cid, ok := s.company(w, r)
	if !ok {
		return 0, 0, false
	}
	year, err := strconv.Atoi(r.PathValue("year"))
	if err != nil || year < 2000 || year > 2100 {
		writeErr(w, http.StatusBadRequest, "Neplatný rok", nil)
		return 0, 0, false
	}
	return cid, year, true
}

func (s *Server) getYearClose(w http.ResponseWriter, r *http.Request) {
	cid, year, ok := s.yearCloseYear(w, r)
	if !ok {
		return
	}
	out, err := s.App.YearCloseFor(r.Context(), cid, year)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) closeYear(w http.ResponseWriter, r *http.Request) {
	cid, year, ok := s.yearCloseYear(w, r)
	if !ok {
		return
	}
	out, err := s.App.CloseYear(r.Context(), cid, year)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// reopenYear: POST …/year-close/{year}/reopen {reason}
func (s *Server) reopenYear(w http.ResponseWriter, r *http.Request) {
	cid, year, ok := s.yearCloseYear(w, r)
	if !ok {
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := s.App.ReopenYear(r.Context(), cid, year, in.Reason)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
