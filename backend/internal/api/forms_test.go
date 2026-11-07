package api

import (
	"fmt"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
)

func TestFormsAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	var cat struct {
		Forms  []struct{ Key string }
		Issuer string
	}
	e.do("GET", "/api/forms", nil, 200, &cat)
	if len(cat.Forms) < 5 || cat.Issuer != "" {
		t.Fatalf("bez firmy: %+v", cat)
	}
	var prev struct {
		Computed struct{ VAT string }
		Errors   []any
	}
	e.do("POST", "/api/forms/preview", `{"formKey":"ppd","values":{"date":"2026-11-07","partner":"J","purpose":"P","amount":"123","rate":"23"}}`, 200, &prev)
	if prev.Computed.VAT != "23.00" || len(prev.Errors) != 0 {
		t.Fatalf("náhľad: %+v", prev)
	}
	base := fmt.Sprintf("/api/companies/%d", e.cid)
	e.do("GET", base+"/forms", nil, 200, &cat)
	if cat.Issuer == "" {
		t.Fatal("hlavička firmy")
	}
	var def struct{ Key string }
	e.do("POST", base+"/form-defs", `{"name":"Protokol","fields":[{"label":"Zákazka"}]}`, 200, &def)
	var doc struct {
		ID         int64
		CashNumber string
	}
	e.do("POST", base+"/form-docs", `{"formKey":"ppd","values":{"date":"2026-11-07","partner":"J","purpose":"Vklad","amount":"100"}}`, 201, &doc)
	e.do("GET", fmt.Sprintf("%s/form-docs/%d", base, doc.ID), nil, 200, nil)
	e.do("POST", fmt.Sprintf("%s/form-docs/%d/cash", base, doc.ID), nil, 200, &doc)
	if doc.CashNumber == "" {
		t.Fatalf("zápis: %+v", doc)
	}
	e.do("DELETE", fmt.Sprintf("%s/form-docs/%d", base, doc.ID), nil, 422, nil)
	var list []any
	e.do("GET", base+"/form-docs", nil, 200, &list)
	if len(list) != 1 {
		t.Fatalf("zoznam: %d", len(list))
	}
	e.do("DELETE", base+"/form-defs/"+def.Key, nil, 204, nil)
}
