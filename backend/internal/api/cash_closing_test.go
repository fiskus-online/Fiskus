package api

import (
	"fmt"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
)

func TestCashClosingAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d/cash/closings", e.cid)
	body := `{"date":"2026-10-12","orp":"888 1234","number":"5","rows":[{"vatRate":23,"gross":"123"}],"card":"23"}`
	var p struct {
		Total, CashPart string
		Errors          []any
	}
	e.do("POST", base+"/preview", body, 200, &p)
	if p.Total != "123.00" || p.CashPart != "100.00" || len(p.Errors) != 0 {
		t.Fatalf("náhľad: %+v", p)
	}
	var c struct {
		ID                  int64
		ORP                 string
		InNumber, OutNumber string
	}
	e.do("POST", base, body, 201, &c)
	if c.ORP != "8881234" || c.InNumber == "" || c.OutNumber == "" {
		t.Fatalf("uzávierka: %+v", c)
	}
	e.do("POST", base, body, 422, nil)
	var list []any
	e.do("GET", base+"?year=2026", nil, 200, &list)
	if len(list) != 1 {
		t.Fatalf("zoznam: %d", len(list))
	}
	e.do("POST", fmt.Sprintf("%s/%d/cancel", base, c.ID), nil, 200, nil)
	e.do("POST", fmt.Sprintf("%s/%d/cancel", base, c.ID), nil, 409, nil)
}
