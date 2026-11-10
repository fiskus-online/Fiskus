package api

import (
	"errors"
	"net/http"

	"github.com/fiskus-online/fiskus/backend/internal/link"
)

// Krížové prepojenia dokladov: /api/companies/{cid}/links/{kind}/{id}.
// Práva overuje aplikácia podľa druhu dokladu.

func linkRef(w http.ResponseWriter, r *http.Request) (link.Ref, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return link.Ref{}, false
	}
	ref := link.Ref{Kind: link.Kind(r.PathValue("kind")), ID: id}
	if !ref.Kind.Known() {
		writeErr(w, http.StatusNotFound, "Neznámy druh dokladu", nil)
		return ref, false
	}
	return ref, true
}

func failLink(w http.ResponseWriter, err error) {
	if errors.Is(err, link.ErrUnknown) {
		writeErr(w, http.StatusNotFound, err.Error(), nil)
		return
	}
	fail(w, err)
}

func (s *Server) docLinks(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	ref, ok := linkRef(w, r)
	if !ok {
		return
	}
	out, err := s.App.DocLinks(r.Context(), cid, ref)
	if err != nil {
		failLink(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) linkDocs(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	ref, ok := linkRef(w, r)
	if !ok {
		return
	}
	var to link.Ref
	if !decode(w, r, &to) {
		return
	}
	out, err := s.App.LinkDocs(r.Context(), cid, ref, to)
	if err != nil {
		failLink(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) unlinkDocs(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	ref, ok := linkRef(w, r)
	if !ok {
		return
	}
	lid, ok := pathID(w, r, "link")
	if !ok {
		return
	}
	out, err := s.App.UnlinkDocs(r.Context(), cid, ref, lid)
	if err != nil {
		failLink(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// quoteInvoicedBy: POST /quotes/{id}/invoiced {"invoiceId": …} — ponuka
// vyfakturovaná existujúcou faktúrou.
func (s *Server) quoteInvoicedBy(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		InvoiceID int64 `json:"invoiceId"`
	}
	if !decode(w, r, &in) {
		return
	}
	q, err := s.App.QuoteInvoicedBy(r.Context(), cid, id, in.InvoiceID)
	if err != nil {
		failQuote(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.quoteRow(q))
}
