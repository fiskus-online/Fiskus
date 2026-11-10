package api

import (
	"log"
	"net/http"
)

// Automatická aktualizácia aplikácie (internal/update).

func (s *Server) updateStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.Update.Status())
}

func (s *Server) updateCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Update.Check(r.Context()))
}

func (s *Server) updateStart(w http.ResponseWriter, _ *http.Request) {
	if err := s.Update.Start(); err != nil {
		writeErr(w, http.StatusConflict, err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, s.Update.Status())
}

func (s *Server) updatePostpone(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.Update.Postpone())
}

func (s *Server) updateAuto(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Auto bool `json:"auto"`
	}
	if !decode(w, r, &in) {
		return
	}
	st, err := s.Update.SetAuto(in.Auto)
	if err != nil {
		log.Printf("aktualizácia: nastavenie: %v", err)
		writeErr(w, http.StatusInternalServerError, "Nastavenie sa nepodarilo uložiť.", nil)
		return
	}
	writeJSON(w, http.StatusOK, st)
}
