package api

import (
	"net/http"

	"github.com/fiskus-online/fiskus/backend/internal/app"
)

// Napojenie na Fio API. Token sa prijíma, ale nikdy sa nevracia.

func (s *Server) listFio(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	list, err := s.App.FioAccounts(r.Context(), cid)
	if err != nil {
		failBank(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) saveFio(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var in app.FioAccountInput
	if !decode(w, r, &in) {
		return
	}
	list, err := s.App.SaveFioAccount(r.Context(), cid, in)
	if err != nil {
		failBank(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) deleteFio(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.App.DeleteFioAccount(r.Context(), cid, id)
	if err != nil {
		failBank(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) syncFio(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	res, err := s.App.SyncFio(r.Context(), cid, id)
	if err != nil {
		failBank(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
