package api

import (
	"net/http"
	"strconv"

	"github.com/fiskus-online/fiskus/backend/internal/cash"
)

// cashClosings: GET /api/companies/{cid}/cash/closings?year=2026 — uzávierky e-kasy.
func (s *Server) cashClosings(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	if year == 0 {
		year = s.App.Today().Year()
	}
	out, err := s.App.CashClosings(r.Context(), cid, year)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) previewCashClosing(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var c cash.Closing
	if !decode(w, r, &c) {
		return
	}
	out, err := s.App.PreviewCashClosing(r.Context(), cid, c)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createCashClosing(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var c cash.Closing
	if !decode(w, r, &c) {
		return
	}
	out, err := s.App.CreateCashClosing(r.Context(), cid, c)
	if err != nil {
		failCash(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) cancelCashClosing(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	out, err := s.App.CancelCashClosing(r.Context(), cid, id)
	if err != nil {
		failCash(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
