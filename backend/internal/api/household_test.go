package api

import (
	"fmt"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestHouseholdAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	// domácnosť vo firme nejde
	e.do("GET", fmt.Sprintf("/api/companies/%d/household/accounts", e.cid), nil, 422, nil)
	var h struct{ ID int64 }
	e.do("POST", "/api/companies", `{"name":"Domácnosť","bookkeeping":"household"}`, 201, &h)
	base := fmt.Sprintf("/api/companies/%d/household", h.ID)
	var acc struct{ ID int64 }
	e.do("POST", base+"/accounts", `{"name":"Peňaženka","kind":"cash","opening":"50"}`, 201, &acc)
	e.do("POST", base+"/entries", fmt.Sprintf(`{"date":"2026-10-02","accountId":%d,"amount":"-12.30","category":"Potraviny","payee":"Lidl"}`, acc.ID), 201, nil)
	e.do("POST", base+"/entries", fmt.Sprintf(`{"date":"2026-10-02","accountId":%d,"amount":"-1"}`, acc.ID), 422, nil)
	var list []struct{ Amount string }
	e.do("GET", base+"/entries?year=2026", nil, 200, &list)
	if len(list) != 1 || list[0].Amount != "-12.30" {
		t.Fatalf("pohyby: %+v", list)
	}
	var sum struct {
		TotalOut string
		Total    string
		Month    int
	}
	e.do("GET", base+"/summary?year=2026&month=10", nil, 200, &sum)
	if sum.TotalOut != "12.30" || sum.Total != "37.70" || sum.Month != 10 {
		t.Fatalf("prehľad: %+v", sum)
	}
	var sug struct{ Category string }
	e.do("GET", base+"/suggest?text=TESCO", nil, 200, &sug)
	if sug.Category != "Potraviny" {
		t.Fatalf("návrh: %+v", sug)
	}
	e.do("PUT", base+"/categories", `[{"name":"Príjem","income":true},{"name":"Jedlo","budget":"300"}]`, 200, nil)
	e.do("GET", base+"/draws?year=2026", nil, 200, nil)
	e.do("DELETE", fmt.Sprintf("%s/accounts/%d", base, acc.ID), nil, 422, nil) // má pohyby

	// záväzky
	var o struct{ ID int64 }
	e.do("POST", base+"/obligations", `{"kind":"loan","name":"Pôžička"}`, 422, nil)
	e.do("POST", base+"/obligations", `{"kind":"loan","name":"Pôžička","principal":"600","installment":"100","firstDue":"2026-11-10","category":"Jedlo"}`, 201, &o)
	e.do("POST", fmt.Sprintf("%s/obligations/%d/pay", base, o.ID), fmt.Sprintf(`{"date":"2026-11-09","accountId":%d}`, acc.ID), 201, nil)
	var det struct {
		State    struct{ Remaining string }
		Schedule []struct{ Status string }
	}
	e.do("GET", fmt.Sprintf("%s/obligations/%d", base, o.ID), nil, 200, &det)
	if det.State.Remaining != "500.00" || len(det.Schedule) != 6 || det.Schedule[0].Status != "paid" {
		t.Fatalf("záväzok: %+v", det)
	}
	e.do("GET", base+"/obligations", nil, 200, nil)
	e.do("DELETE", fmt.Sprintf("%s/obligations/%d", base, o.ID), nil, 422, nil)
}
