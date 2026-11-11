package api

import (
	"fmt"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
)

func TestDashboardAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	var d struct {
		Year   int
		Months []struct{ Month int }
		Events []any
	}
	e.do("GET", fmt.Sprintf("/api/companies/%d/dashboard?year=2026", e.cid), nil, 200, &d)
	if d.Year != 2026 || len(d.Months) != 12 || d.Events == nil {
		t.Fatalf("prehľad %+v", d)
	}
}
