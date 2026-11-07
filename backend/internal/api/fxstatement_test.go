package api

import (
	"fmt"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
)

func TestFXStatementAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d/ledger/2026/fx", e.cid)
	e.do("PUT", base+"/statement", `{"key":"bank:SK6911000000002600000999","balance":"520"}`, 204, nil)
	e.do("PUT", base+"/statement", `{"key":"bank:SK6911000000002600000999","balance":null}`, 204, nil)
	e.do("PUT", base+"/statement", `{"key":"sale:1","balance":"1"}`, 422, nil)
	var rv struct {
		Items    []any
		Warnings []string
	}
	e.do("GET", base, nil, 200, &rv)
	if rv.Warnings == nil {
		t.Fatal("chýba zoznam upozornení")
	}
}
