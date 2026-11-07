package api

import (
	"net/http"

	"github.com/fiskus-online/fiskus/backend/internal/app"
)

// Kalkulačky: bez firmy, nič neukladajú.

func (s *Server) calcInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.App.CalcInfo(r.Context()))
}

func calcHandler[I, O any](f func(I) (O, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in I
		if !decode(w, r, &in) {
			return
		}
		out, err := f(in)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Server) calcFX(w http.ResponseWriter, r *http.Request) {
	var in app.FXInput
	if !decode(w, r, &in) {
		return
	}
	out, err := s.App.CalcFX(r.Context(), in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
