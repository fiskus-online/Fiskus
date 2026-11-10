package api

import (
	"net/http"
	"strconv"

	"github.com/fiskus-online/fiskus/backend/internal/activity"
	"github.com/fiskus-online/fiskus/backend/internal/app"
)

// CRM aktivity: /api/companies/{cid}/activities?contact=&job=&quote=&open=1.

func (s *Server) listActivities(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	q := r.URL.Query()
	n := func(k string) int64 {
		v, _ := strconv.ParseInt(q.Get(k), 10, 64)
		return v
	}
	out, err := s.App.Activities(r.Context(), cid, app.ActivityFilter{ContactID: n("contact"), JobID: n("job"), QuoteID: n("quote"), OpenTasks: q.Get("open") == "1"})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) saveActivity(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var x activity.Activity
	if !decodeLoose(w, r, &x) {
		return
	}
	x.ID = 0
	if r.PathValue("id") != "" {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		x.ID = id
	}
	out, err := s.App.SaveActivity(r.Context(), cid, x)
	if err != nil {
		fail(w, err)
		return
	}
	code := http.StatusOK
	if x.ID == 0 {
		code = http.StatusCreated
	}
	writeJSON(w, code, out)
}

func (s *Server) deleteActivity(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.App.DeleteActivity(r.Context(), cid, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// doneActivity: POST /activities/{id}/done {done}.
func (s *Server) doneActivity(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Done bool `json:"done"`
	}
	if !decodeLoose(w, r, &body) {
		return
	}
	out, err := s.App.SetActivityDone(r.Context(), cid, id, body.Done)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
