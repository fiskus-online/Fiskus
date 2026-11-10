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

func TestJobsAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d/jobs", e.cid)
	type jobRow struct {
		ID      int64
		Number  string
		Name    string
		Status  string
		Summary struct{ Docs int }
		Docs    []struct{ Kind string }
	}
	var j jobRow
	e.do("POST", base, `{"name":""}`, 422, nil)
	e.do("POST", base, `{"name":"Rekonštrukcia RS","status":"active","budget":"8690"}`, 201, &j)
	if j.Number == "" || j.Status != "active" {
		t.Fatalf("nová: %+v", j)
	}
	var inv struct{ ID int64 }
	e.do("POST", fmt.Sprintf("/api/companies/%d/invoices", e.cid), `{"customer":{"name":"Zákazník s.r.o.","country":"SK"},"issueDate":"2026-10-01","deliveryDate":"2026-10-01","dueDate":"2026-10-15",`+
		`"lines":[{"description":"Práca","quantity":"2","unit":"hod","unitPrice":"50","vatRate":23}]}`, 201, &inv)
	e.do("POST", fmt.Sprintf("/api/companies/%d/links/job/%d", e.cid, j.ID), fmt.Sprintf(`{"kind":"invoice","id":%d}`, inv.ID), 200, nil)
	e.do("GET", fmt.Sprintf("%s/%d", base, j.ID), nil, 200, &j)
	if j.Summary.Docs != 1 || len(j.Docs) != 1 || j.Docs[0].Kind != "invoice" {
		t.Fatalf("detail: %+v", j)
	}
	e.do("PUT", fmt.Sprintf("%s/%d", base, j.ID), `{"name":"Rekonštrukcia","status":"done"}`, 200, &j)
	var list []jobRow
	e.do("GET", base, nil, 200, &list)
	if len(list) != 1 || list[0].Status != "done" || list[0].Name != "Rekonštrukcia" {
		t.Fatalf("zoznam: %+v", list)
	}
	// faktúra z výkazu práce: bez zákazníka z adresára nejde
	e.do("PUT", fmt.Sprintf("%s/%d", base, j.ID), `{"name":"Rekonštrukcia","status":"active",`+
		`"work":[{"date":"2026-10-05","description":"Montáž","hours":"3","rate":"40"}]}`, 200, &j)
	e.do("POST", fmt.Sprintf("%s/%d/invoice", base, j.ID), nil, 422, nil)
	var ct struct{ ID int64 }
	e.do("POST", fmt.Sprintf("/api/companies/%d/contacts", e.cid), `{"name":"Zákazník s.r.o.","country":"SK"}`, 201, &ct)
	e.do("PUT", fmt.Sprintf("%s/%d", base, j.ID), fmt.Sprintf(`{"name":"Rekonštrukcia","status":"active","contactId":%d,`+
		`"work":[{"date":"2026-10-05","description":"Montáž","hours":"3","rate":"40"}]}`, ct.ID), 200, &j)
	var drafted struct {
		Status string
		Lines  []struct{ Description string }
	}
	e.do("POST", fmt.Sprintf("%s/%d/invoice", base, j.ID), nil, 201, &drafted)
	if drafted.Status != "draft" || len(drafted.Lines) != 1 {
		t.Fatalf("koncept zo zákazky: %+v", drafted)
	}
	e.do("DELETE", fmt.Sprintf("%s/%d", base, j.ID), nil, 422, nil) // má vyfakturovanú prácu
}

func TestPurchaseOrdersAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d/purchase-orders", e.cid)
	type row struct {
		ID     int64
		Number string
		Status string
		Total  string
	}
	var o row
	e.do("POST", base, `{"supplier":"","lines":[]}`, 422, nil)
	e.do("POST", base, `{"supplier":"Hornbach","date":"2026-11-10","lines":[{"name":"Hmoždinky","quantity":"2","unitPrice":"3"}]}`, 201, &o)
	if o.Number == "" || o.Status != "draft" || o.Total != "6.00" {
		t.Fatalf("nová: %+v", o)
	}
	e.do("POST", fmt.Sprintf("%s/%d/send", base, o.ID), nil, 200, &o)
	if o.Status != "sent" {
		t.Fatalf("odoslaná: %+v", o)
	}
	var rec struct{ Order row }
	e.do("POST", fmt.Sprintf("%s/%d/receive", base, o.ID), `{"quantity":["2"]}`, 200, &rec)
	if rec.Order.Status != "received" {
		t.Fatalf("prijatá: %+v", rec)
	}
	e.do("POST", fmt.Sprintf("%s/%d/cancel", base, o.ID), nil, 422, nil)
	var list []row
	e.do("GET", base, nil, 200, &list)
	if len(list) != 1 {
		t.Fatalf("zoznam %+v", list)
	}
	var re []struct{ ItemID int64 }
	e.do("GET", fmt.Sprintf("/api/companies/%d/stock/reorder", e.cid), nil, 200, &re)
	e.do("POST", fmt.Sprintf("/api/companies/%d/stock/reorder", e.cid), `{"picks":[]}`, 422, nil)
	e.do("DELETE", fmt.Sprintf("%s/%d", base, o.ID), nil, 422, nil)
}

func TestDeliveriesAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d/deliveries", e.cid)
	type row struct {
		ID     int64
		Number string
		Status string
	}
	var n row
	e.do("POST", base, `{"customer":{"name":"Zákazník s.r.o."},"lines":[]}`, 422, nil)
	e.do("POST", base, `{"customer":{"name":"Zákazník s.r.o."},"date":"2026-11-10","lines":[{"description":"Montáž","quantity":"2","unit":"hod","unitPrice":"35","vatRate":23}]}`, 201, &n)
	if n.Status != "draft" || n.Number != "" {
		t.Fatalf("koncept %+v", n)
	}
	e.do("POST", fmt.Sprintf("%s/%d/issue", base, n.ID), nil, 200, &n)
	if n.Status != "issued" || n.Number == "" {
		t.Fatalf("vydaný %+v", n)
	}
	e.do("DELETE", fmt.Sprintf("%s/%d", base, n.ID), nil, 422, nil)
	var inv struct {
		ID     int64
		Status string
	}
	e.do("POST", base+"/invoice", fmt.Sprintf(`{"ids":[%d]}`, n.ID), 201, &inv)
	if inv.Status != "draft" {
		t.Fatalf("faktúra %+v", inv)
	}
	e.do("POST", fmt.Sprintf("%s/%d/cancel", base, n.ID), nil, 422, nil)
	var list []row
	e.do("GET", base, nil, 200, &list)
	if len(list) != 1 || list[0].Status != "invoiced" {
		t.Fatalf("zoznam %+v", list)
	}
	e.do("POST", fmt.Sprintf("/api/companies/%d/quotes/999/delivery", e.cid), nil, 404, nil)
}
