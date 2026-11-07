package api

import (
	"fmt"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
)

func TestAccDepreciationAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d", e.cid)
	e.do("GET", base+"/ledger/2026/depreciation", nil, 422, nil) // jednoduché účtovníctvo
	e.do("PUT", base, `{"name":"Test s.r.o.","bookkeeping":"double"}`, 200, nil)
	e.do("POST", base+"/assets", `{"name":"Stroj","kind":"tangible","group":1,"acquired":"2025-03-15","cost":"4800","accMonths":48}`, 201, nil)
	var dp struct {
		Total string `json:"total"`
		Lines []any  `json:"lines"`
	}
	e.do("GET", base+"/ledger/2026/depreciation", nil, 200, &dp)
	if dp.Total != "1200.00" || len(dp.Lines) != 2 {
		t.Fatalf("návrh: %+v", dp)
	}
	var doc struct {
		Number string `json:"number"`
		Dep    bool   `json:"dep"`
	}
	e.do("POST", base+"/ledger/2026/depreciation", nil, 200, &doc)
	if !doc.Dep || doc.Number == "" {
		t.Fatalf("doklad: %+v", doc)
	}
}
