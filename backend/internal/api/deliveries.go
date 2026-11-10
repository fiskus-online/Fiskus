package api

import (
	"context"
	"net/http"

	"github.com/fiskus-online/fiskus/backend/internal/app"
	"github.com/fiskus-online/fiskus/backend/internal/delivery"
)

// Dodacie listy: /api/companies/{cid}/deliveries.

func (s *Server) listDeliveries(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	out, err := s.App.Deliveries(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getDelivery(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	out, err := s.App.Delivery(r.Context(), cid, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) saveDelivery(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var n delivery.Note
	if !decodeLoose(w, r, &n) {
		return
	}
	n.ID = 0
	if r.PathValue("id") != "" {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		n.ID = id
	}
	out, err := s.App.SaveDelivery(r.Context(), cid, n)
	if err != nil {
		fail(w, err)
		return
	}
	code := http.StatusOK
	if n.ID == 0 {
		code = http.StatusCreated
	}
	writeJSON(w, code, out)
}

func (s *Server) deleteDelivery(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.App.DeleteDelivery(r.Context(), cid, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deliveryAction: POST /deliveries/{id}/issue | cancel.
func (s *Server) deliveryAction(fn func(*app.App, context.Context, int64, int64) (delivery.Note, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cid, _ := pathID(w, r, "cid")
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		out, err := fn(s.App, r.Context(), cid, id)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// invoiceDeliveries: POST /deliveries/invoice {ids} — zberná faktúra.
func (s *Server) invoiceDeliveries(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if !decodeLoose(w, r, &body) {
		return
	}
	inv, err := s.App.InvoiceDeliveries(r.Context(), cid, body.IDs)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, inv)
}

// deliveryFromQuote: POST /quotes/{id}/delivery.
func (s *Server) deliveryFromQuote(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	out, err := s.App.DeliveryFromQuote(r.Context(), cid, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}
