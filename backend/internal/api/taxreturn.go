package api

import (
	"net/http"
	"strconv"

	"github.com/fiskus-online/fiskus/backend/internal/dpfo"
)

// Daňové priznanie k dani z príjmov FO typ B.

func taxYear(w http.ResponseWriter, r *http.Request) (int, bool) {
	year, err := strconv.Atoi(r.PathValue("year"))
	if err != nil || year < 2000 || year > 2100 {
		writeErr(w, http.StatusBadRequest, "Neplatný rok", nil)
		return 0, false
	}
	return year, true
}

func (s *Server) getTaxReturn(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	year, ok := taxYear(w, r)
	if !ok {
		return
	}
	out, err := s.App.TaxReturnFor(r.Context(), cid, year)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) putTaxReturn(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	year, ok := taxYear(w, r)
	if !ok {
		return
	}
	var in dpfo.Input
	if !decode(w, r, &in) {
		return
	}
	out, err := s.App.SaveTaxReturn(r.Context(), cid, year, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) taxReturnXML(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	year, ok := taxYear(w, r)
	if !ok {
		return
	}
	data, name, err := s.App.TaxReturnXML(r.Context(), cid, year)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_, _ = w.Write(data)
}
