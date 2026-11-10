package api

import (
	"fmt"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
)

func TestSalesOrdersAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d/sales-orders", e.cid)
	type order struct {
		ID     int64
		Number string
		Status string
		Lines  []struct {
			Key       int
			Remaining string
		}
	}
	var o order
	e.do("POST", base, `{"customer":{"name":"Zákazník s.r.o.","country":"SK"},"date":"2026-11-05","ref":"OBJ-1",`+
		`"lines":[{"description":"Práca","quantity":"2","unit":"hod","unitPrice":"50","vatRate":23}]}`, 201, &o)
	if o.Number == "" || o.Status != "open" || len(o.Lines) != 1 || o.Lines[0].Key != 1 {
		t.Fatalf("nová: %+v", o)
	}
	e.do("POST", base, `{"customer":{"name":""},"date":"2026-11-05"}`, 422, nil)
	e.do("POST", fmt.Sprintf("%s/%d/fulfil", base, o.ID), `{"to":"x"}`, 422, nil)
	var doc struct {
		Kind string
		ID   int64
	}
	e.do("POST", fmt.Sprintf("%s/%d/fulfil", base, o.ID), `{"to":"delivery","qty":{"1":"1"}}`, 201, &doc)
	if doc.Kind != "delivery" || doc.ID == 0 {
		t.Fatalf("dodanie: %+v", doc)
	}
	e.do("GET", fmt.Sprintf("%s/%d", base, o.ID), nil, 200, &o)
	if o.Status != "partial" || o.Lines[0].Remaining != "1.00" {
		t.Fatalf("po dodaní: %+v", o)
	}
	e.do("DELETE", fmt.Sprintf("%s/%d", base, o.ID), nil, 422, nil)
	e.do("POST", fmt.Sprintf("%s/%d/cancel", base, o.ID), nil, 200, &o)
	e.do("POST", fmt.Sprintf("%s/%d/reopen", base, o.ID), nil, 200, &o)
	var list []order
	e.do("GET", base, nil, 200, &list)
	if len(list) != 1 || list[0].Status != "partial" {
		t.Fatalf("zoznam: %+v", list)
	}
	e.do("GET", fmt.Sprintf("/api/companies/%d/links/sorder/%d", e.cid, o.ID), nil, 200, nil)
}
