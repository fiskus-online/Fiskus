package api

import (
	"context"
	"net/http"

	"github.com/fiskus-online/fiskus/backend/internal/app"
	"github.com/fiskus-online/fiskus/backend/internal/porder"
)

// Objednávky dodávateľom: /api/companies/{cid}/purchase-orders
// a návrh na objednanie /api/companies/{cid}/stock/reorder.

func (s *Server) listPurchaseOrders(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	out, err := s.App.PurchaseOrders(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	out, err := s.App.PurchaseOrder(r.Context(), cid, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) savePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var o porder.Order
	if !decodeLoose(w, r, &o) {
		return
	}
	o.ID = 0
	if r.PathValue("id") != "" {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		o.ID = id
	}
	out, err := s.App.SavePurchaseOrder(r.Context(), cid, o)
	if err != nil {
		fail(w, err)
		return
	}
	code := http.StatusOK
	if o.ID == 0 {
		code = http.StatusCreated
	}
	writeJSON(w, code, out)
}

func (s *Server) deletePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.App.DeletePurchaseOrder(r.Context(), cid, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// porderAction: POST /purchase-orders/{id}/send | cancel.
func (s *Server) porderAction(fn func(*app.App, context.Context, int64, int64) (porder.Order, error)) http.HandlerFunc {
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

func (s *Server) receivePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body app.POrderReceipt
	if !decodeLoose(w, r, &body) {
		return
	}
	out, err := s.App.ReceivePurchaseOrder(r.Context(), cid, id, body)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) reorder(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	out, err := s.App.Reorder(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) ordersFromReorder(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var body struct {
		Picks []app.ReorderPick `json:"picks"`
	}
	if !decodeLoose(w, r, &body) {
		return
	}
	out, err := s.App.OrdersFromReorder(r.Context(), cid, body.Picks)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}
