package job

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
)

func TestValidate(t *testing.T) {
	d := invoice.MustDate
	for _, tc := range []struct {
		name string
		j    Job
		want string // pole s chybou
	}{
		{"v poriadku", Job{Name: "Rekonštrukcia RS", Start: d("2026-10-01"), Due: d("2026-10-31")}, ""},
		{"bez názvu", Job{Name: "  "}, "name"},
		{"zlý stav", Job{Name: "x", Status: "x"}, "status"},
		{"termín pred začiatkom", Job{Name: "x", Start: d("2026-10-10"), Due: d("2026-10-01")}, "due"},
		{"záporná cena", Job{Name: "x", Budget: money.MustParse("-1")}, "budget"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			j := tc.j
			j.Normalize()
			errs := j.Validate()
			got := ""
			if len(errs) > 0 {
				got = errs[0].Field
			}
			if got != tc.want || j.Status == "" {
				t.Fatalf("%+v %+v", errs, j)
			}
		})
	}
}

func TestStatus(t *testing.T) {
	today := invoice.MustDate("2026-11-10")
	j := Job{Name: "x", Status: Active, Due: invoice.MustDate("2026-11-01")}
	if !j.Late(today) {
		t.Fatal("po termíne")
	}
	j.SetStatus(Done, today)
	if j.Closed != today || j.Late(today) || j.Status.Open() {
		t.Fatalf("dokončená: %+v", j)
	}
	j.SetStatus(Done, invoice.MustDate("2026-12-01"))
	if j.Closed != today {
		t.Fatal("dátum dokončenia sa nemení")
	}
	j.SetStatus(Active, today)
	if !j.Closed.IsZero() {
		t.Fatal("znovu otvorená")
	}
}
