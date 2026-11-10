package api

import (
	"net/http"

	"github.com/fiskus-online/fiskus/backend/internal/job"
)

// Zákazky (CRM): /api/companies/{cid}/jobs.

func (s *Server) listCrmJobs(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	out, err := s.App.CrmJobs(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getCrmJob(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	out, err := s.App.CrmJob(r.Context(), cid, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) saveCrmJob(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var j job.Job
	if !decodeLoose(w, r, &j) {
		return
	}
	j.ID = 0
	if r.PathValue("id") != "" {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		j.ID = id
	}
	out, err := s.App.SaveCrmJob(r.Context(), cid, j)
	if err != nil {
		fail(w, err)
		return
	}
	code := http.StatusOK
	if j.ID == 0 {
		code = http.StatusCreated
	}
	writeJSON(w, code, out)
}

func (s *Server) deleteCrmJob(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.App.DeleteCrmJob(r.Context(), cid, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// invoiceCrmJob: POST /jobs/{id}/invoice — koncept faktúry z nevyfakturovanej
// práce a materiálu.
func (s *Server) invoiceCrmJob(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	inv, err := s.App.InvoiceCrmJob(r.Context(), cid, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, inv)
}
