package api

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
)

func TestCalcAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	var info struct{ VATRates []int }
	e.do("GET", "/api/calc", nil, 200, &info)
	if len(info.VATRates) == 0 {
		t.Fatalf("sadzby: %+v", info)
	}
	tests := []struct {
		path, body string
		status     int
		key, want  string
	}{
		{"/api/calc/vat", `{"amount":"123","rate":23,"gross":true}`, 200, "base", "100.00"},
		{"/api/calc/vat", `{"amount":"1","rate":7}`, 422, "", ""},
		{"/api/calc/late-interest", `{"amount":"1000","due":"2026-09-30","paid":"2026-10-30","debtor":"consumer"}`, 200, "interest", "6.29"},
		{"/api/calc/loan", `{"amount":"10000","rate":"6","months":12}`, 200, "payment", "860.66"},
		{"/api/calc/savings", `{"initial":"1000","rate":"12","months":12}`, 200, "balance", "1126.84"},
		{"/api/calc/wage", `{"year":2026,"month":1,"gross":"1500","nczd":true}`, 200, "gross", "1500.00"},
		{"/api/calc/wage", `{"year":2026,"gross":"0"}`, 422, "", ""},
		{"/api/calc/fx", `{"amount":"10","currency":"CZK","date":"2020-01-01"}`, 422, "", ""},
		{"/api/calc/loan", `{"amount":"1","neznáme":1}`, 400, "", ""},
	}
	for _, tt := range tests {
		var out map[string]any
		e.do("POST", tt.path, tt.body, tt.status, &out)
		if tt.key != "" && out[tt.key] != tt.want {
			t.Errorf("%s %s: %v", tt.path, tt.body, out)
		}
	}
}
