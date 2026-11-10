package api

import (
	"context"
	"net/http"

	"github.com/fiskus-online/fiskus/backend/internal/app"
	"github.com/fiskus-online/fiskus/backend/internal/sorder"
)

// Prijaté objednávky: /api/companies/{cid}/sales-orders.

func (s *Server) listSalesOrders(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	out, err := s.App.SalesOrders(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getSalesOrder(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	out, err := s.App.SalesOrder(r.Context(), cid, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) saveSalesOrder(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var n sorder.Order
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
	out, err := s.App.SaveSalesOrder(r.Context(), cid, n)
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

func (s *Server) deleteSalesOrder(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.App.DeleteSalesOrder(r.Context(), cid, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// salesOrderAction: POST /sales-orders/{id}/cancel | reopen.
func (s *Server) salesOrderAction(fn func(*app.App, context.Context, int64, int64) (sorder.Order, error)) http.HandlerFunc {
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

// fulfilSalesOrder: POST /sales-orders/{id}/fulfil {to, qty} — koncept
// dodacieho listu alebo faktúry z objednávky.
func (s *Server) fulfilSalesOrder(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body app.SalesOrderFulfil
	if !decodeLoose(w, r, &body) {
		return
	}
	out, err := s.App.FulfilSalesOrder(r.Context(), cid, id, body)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// salesOrderFromQuote: POST /quotes/{id}/sales-order.
func (s *Server) salesOrderFromQuote(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	out, err := s.App.SalesOrderFromQuote(r.Context(), cid, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}
