package api

import (
	"fmt"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
)

func TestLinksAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d", e.cid)
	var inv, q struct {
		ID     int64
		Number string
		Status string
	}
	e.do("POST", base+"/invoices", `{"customer":{"name":"Zákazník s.r.o.","country":"SK"},"issueDate":"2026-10-01","deliveryDate":"2026-10-01","dueDate":"2026-10-15",`+
		`"lines":[{"description":"Práca","quantity":"2","unit":"hod","unitPrice":"50","vatRate":23}]}`, 201, &inv)
	e.do("POST", fmt.Sprintf("%s/invoices/%d/issue", base, inv.ID), nil, 200, &inv)
	e.do("POST", base+"/quotes", `{"customer":{"name":"Zákazník s.r.o.","country":"SK"},"issueDate":"2026-09-20","validUntil":"2026-10-20",`+
		`"lines":[{"description":"Práca","quantity":"2","unit":"hod","unitPrice":"50","vatRate":23}]}`, 201, &q)
	e.do("POST", fmt.Sprintf("%s/quotes/%d/issue", base, q.ID), nil, 200, &q)

	type linked struct {
		Kind     string
		ID       int64
		LinkID   int64
		Relation string
		Number   string
	}
	var list []linked
	e.do("POST", fmt.Sprintf("%s/links/invoice/%d", base, inv.ID), fmt.Sprintf(`{"kind":"quote","id":%d}`, q.ID), 200, &list)
	if len(list) != 1 || list[0].Kind != "quote" || list[0].LinkID == 0 || list[0].Number != q.Number {
		t.Fatalf("prepojenie: %+v", list)
	}
	e.do("POST", fmt.Sprintf("%s/links/invoice/%d", base, inv.ID), fmt.Sprintf(`{"kind":"quote","id":%d}`, q.ID), 422, nil)
	e.do("GET", fmt.Sprintf("%s/links/bank/%d", base, inv.ID), nil, 404, nil)
	e.do("GET", fmt.Sprintf("%s/links/quote/%d", base, 9999), nil, 404, nil)
	e.do("DELETE", fmt.Sprintf("%s/links/quote/%d/%d", base, q.ID, list[0].LinkID), nil, 200, &list)
	if len(list) != 0 {
		t.Fatalf("zrušenie: %+v", list)
	}
	// ponuka vyfakturovaná existujúcou faktúrou
	e.do("POST", fmt.Sprintf("%s/quotes/%d/invoiced", base, q.ID), `{"invoiceId":9999}`, 422, nil)
	e.do("POST", fmt.Sprintf("%s/quotes/%d/invoiced", base, q.ID), fmt.Sprintf(`{"invoiceId":%d}`, inv.ID), 200, &q)
	if q.Status != "invoiced" {
		t.Fatalf("stav: %+v", q)
	}
	e.do("POST", fmt.Sprintf("%s/quotes/%d/invoiced", base, q.ID), fmt.Sprintf(`{"invoiceId":%d}`, inv.ID), 409, nil)
	e.do("GET", fmt.Sprintf("%s/links/invoice/%d", base, inv.ID), nil, 200, &list)
	if len(list) != 1 || list[0].Relation == "" || list[0].LinkID != 0 {
		t.Fatalf("z faktúry: %+v", list)
	}
}
