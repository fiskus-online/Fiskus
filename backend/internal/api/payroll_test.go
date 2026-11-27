package api

import (
	"fmt"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
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
	// príkazy na úhradu
	var pays struct {
		Closed   bool             `json:"closed"`
		Payments []map[string]any `json:"payments"`
	}
	e.do("GET", base+"/payroll/2026/3/payments", nil, 200, &pays)
	if !pays.Closed || len(pays.Payments) == 0 {
		t.Fatalf("úhrady: %+v", pays)
	}
	e.do("PUT", base+"/payroll/payment-settings", `{"socialIban":"SK00"}`, 422, nil)
	e.do("PUT", base+"/payroll/payment-settings", `{"socialIban":"SK2381800000007000000011","socialVs":"1234567890","taxOud":"1234567890"}`, 200, nil)
	e.do("POST", base+"/payroll/2026/3/payment-order", `{"keys":[]}`, 422, nil)
	// mesačný výkaz poistného: chýbajú údaje (VS, deň výplaty, zostavovateľ, RČ)
	e.do("GET", base+"/payroll/2026/3/mvpp", nil, 422, nil)
	// ročné zúčtovanie
	var rz struct {
		List []struct {
			Rows map[string]string `json:"rows"`
		} `json:"list"`
	}
	e.do("GET", base+"/payroll-settlement/2026", nil, 200, &rz)
	if len(rz.List) != 1 || rz.List[0].Rows["01"] == "" {
		t.Fatalf("ročné zúčtovanie: %+v", rz)
	}
	e.do("PUT", base+"/payroll-settlement/2026/1", `{"requested":true,"dds":"-5"}`, 422, nil)
	e.do("PUT", base+"/payroll-settlement/2026/1", `{"requested":true,"dds":"60"}`, 200, &rz)
	if rz.List[0].Rows["04c"] != "60.00" {
		t.Fatalf("DDS: %+v", rz.List[0].Rows)
	}
	e.do("GET", base+"/payroll-settlement/2019", nil, 422, nil)
	// zaúčtovanie miezd: iba v podvojnom účtovníctve
	e.do("GET", base+"/payroll/2026/3/posting", nil, 422, nil)
	e.do("PUT", base, `{"name":"Test s.r.o.","bookkeeping":"double"}`, 200, nil)
	var pp struct {
		Lines  []map[string]any `json:"lines"`
		Posted string           `json:"posted"`
		ToPay  string           `json:"toPay"`
	}
	e.do("GET", base+"/payroll/2026/3/posting", nil, 200, &pp)
	if len(pp.Lines) == 0 || pp.ToPay != "1100.00" || pp.Posted != "" {
		t.Fatalf("návrh: %+v", pp)
	}
	var doc struct {
		Number  string `json:"number"`
		Payroll string `json:"payroll"`
	}
	e.do("POST", base+"/payroll/2026/3/posting", nil, 200, &doc)
	if doc.Payroll != "2026-03" || doc.Number == "" {
		t.Fatalf("doklad: %+v", doc)
	}
	e.do("POST", base+"/payroll/2026/3/reopen", `{"reason":"oprava zrážky"}`, 200, &v)
	e.do("GET", base+"/payroll/2026/13", nil, 400, nil)
	e.do("GET", base+"/payroll/2020/1", nil, 422, nil)
	e.do("DELETE", fmt.Sprintf("%s/employees/%d", base, emp.ID), nil, 204, nil)
}
