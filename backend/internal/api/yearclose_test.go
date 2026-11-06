package api

import (
	"fmt"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestYearCloseAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d/year-close", e.cid)
	var info struct {
		CanClose bool
		Checks   []struct{ ID string }
	}
	e.do("GET", base+"/2025", nil, 200, &info)
	if !info.CanClose || len(info.Checks) < 3 {
		t.Fatalf("stav: %+v", info)
	}
	e.do("GET", base+"/x", nil, 400, nil)
	var closed struct{ Close struct{ Closed bool } }
	e.do("POST", base+"/2025", nil, 200, &closed)
	if !closed.Close.Closed {
		t.Fatal("rok nie je uzavretý")
	}
	e.do("PUT", fmt.Sprintf("/api/companies/%d/statements/2025", e.cid), `{}`, 422, nil)
	e.do("POST", base+"/2025/reopen", `{"reason":""}`, 422, nil)
	e.do("POST", base+"/2025/reopen", `{"reason":"oprava dokladu"}`, 200, nil)
	e.do("POST", base+"/2026", nil, 422, nil) // rok neskončil
}
