package api

import (
	"net/http"
	"strconv"

	"github.com/fiskus-online/fiskus/backend/internal/app"
	"github.com/fiskus-online/fiskus/backend/internal/forms"
)

// Tlačivá: /api/forms bez firmy (vstavané, nič sa neukladá),
// /api/companies/{cid}/… vo firme (vlastné tlačivá, uložené, zápis do pokladne).

// formCompany: firma z cesty, alebo 0 pre adresy bez firmy.
func (s *Server) formCompany(w http.ResponseWriter, r *http.Request) (int64, bool) {
	if r.PathValue("cid") == "" {
		return 0, true
	}
	return s.company(w, r)
}

func (s *Server) formCatalog(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.formCompany(w, r)
	if !ok {
		return
	}
	out, err := s.App.FormCatalog(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) formPreview(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.formCompany(w, r)
	if !ok {
		return
	}
	var req app.FormPreviewRequest
	if !decode(w, r, &req) {
		return
	}
	out, err := s.App.PreviewForm(r.Context(), cid, req)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) saveFormDef(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var f forms.Form
	if !decode(w, r, &f) {
		return
	}
	out, err := s.App.SaveFormDef(r.Context(), cid, f)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) deleteFormDef(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	if err := s.App.DeleteFormDef(r.Context(), cid, r.PathValue("key")); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listFormDocs(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	out, err := s.App.FormDocs(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getFormDoc(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	out, err := s.App.FormDoc(r.Context(), cid, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) saveFormDoc(w http.ResponseWriter, r *http.Request) {
	saveEntity(s, w, r, func(d *forms.Doc, id int64) { d.ID = id }, func(cid int64, d forms.Doc) (forms.Doc, error) {
		return s.App.SaveFormDoc(r.Context(), cid, d)
	})
}

func (s *Server) deleteFormDoc(w http.ResponseWriter, r *http.Request) {
	s.deleteEntity(w, r, func(cid, id int64) error { return s.App.DeleteFormDoc(r.Context(), cid, id) })
}

// postFormDoc: POST …/form-docs/{id}/cash?registerId=N — zápis do pokladne.
func (s *Server) postFormDoc(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	reg, _ := strconv.ParseInt(r.URL.Query().Get("registerId"), 10, 64)
	out, err := s.App.PostFormDoc(r.Context(), cid, id, reg)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
