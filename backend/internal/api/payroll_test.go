package api

import (
	"fmt"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestPayrollAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d", e.cid)
	var emp struct {
		ID int64 `json:"id"`
	}
	e.do("POST", base+"/employees", `{"lastName":"Nová","contract":"tpp","insurer":"25","start":"2026-01-01","wage":"1500","nczd":true}`, 201, &emp)
	e.do("PUT", fmt.Sprintf("%s/employees/%d", base, emp.ID), `{"lastName":"Nová","firstName":"Jana","contract":"tpp","insurer":"25","start":"2026-01-01","wage":"1500","nczd":true}`, 200, nil)
	e.do("POST", base+"/employees", `{"lastName":""}`, 422, nil)
	var list []map[string]any
	e.do("GET", base+"/employees", nil, 200, &list)
	if len(list) != 1 || list[0]["firstName"] != "Jana" {
		t.Fatalf("zoznam: %v", list)
	}
	var v struct {
		Closed bool `json:"closed"`
		Rows   []struct {
			Result struct {
				Net string `json:"net"`
			} `json:"result"`
		} `json:"rows"`
		Totals struct {
			ToPay string `json:"toPay"`
		} `json:"totals"`
	}
	e.do("GET", base+"/payroll/2026/3", nil, 200, &v)
	if len(v.Rows) != 1 || v.Rows[0].Result.Net != "1134.52" || v.Totals.ToPay != "1134.52" {
		t.Fatalf("marec: %+v", v)
	}
	e.do("PUT", base+"/payroll/2026/3", fmt.Sprintf(`{"inputs":[{"employeeId":%d,"deduction":"34.52"}]}`, emp.ID), 200, &v)
	if v.Totals.ToPay != "1100.00" {
		t.Fatalf("zrážka: %+v", v.Totals)
	}
	e.do("POST", base+"/payroll/2026/3/close", nil, 200, &v)
	if !v.Closed {
		t.Fatal("neuzavreté")
	}
	e.do("POST", base+"/payroll/2026/3/reopen", `{"reason":"oprava zrážky"}`, 200, &v)
	e.do("GET", base+"/payroll/2026/13", nil, 400, nil)
	e.do("GET", base+"/payroll/2020/1", nil, 422, nil)
	e.do("DELETE", fmt.Sprintf("%s/employees/%d", base, emp.ID), nil, 204, nil)
}
