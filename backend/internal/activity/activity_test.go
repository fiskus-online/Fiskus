package activity

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
)

func TestValidate(t *testing.T) {
	d := invoice.MustDate
	for _, tc := range []struct {
		name string
		a    Activity
		want string
	}{
		{"poznámka", Activity{Text: " Volal, chce cenu do piatku ", Date: d("2026-11-10"), JobID: 3}, ""},
		{"úloha", Activity{Kind: Task, Text: "Ozvať sa k ponuke", Date: d("2026-11-10"), Due: d("2026-11-17"), QuoteID: 9}, ""},
		{"úloha bez termínu", Activity{Kind: Task, Text: "x", Date: d("2026-11-10"), ContactID: 1}, "due"},
		{"neznámy druh", Activity{Kind: "fax", Text: "x", Date: d("2026-11-10"), ContactID: 1}, "kind"},
		{"bez textu", Activity{Text: " ", Date: d("2026-11-10"), ContactID: 1}, "text"},
		{"bez dátumu", Activity{Text: "x", ContactID: 1}, "date"},
		{"bez priradenia", Activity{Text: "x", Date: d("2026-11-10")}, "contactId"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.a
			a.Normalize()
			errs := a.Validate()
			got := ""
			if len(errs) > 0 {
				got = errs[0].Field
			}
			if got != tc.want {
				t.Fatalf("chyba %q, chcel som %q (%v)", got, tc.want, errs)
			}
		})
	}
}

func TestTask(t *testing.T) {
	d := invoice.MustDate
	n := Activity{Kind: Call, Text: "x", Due: d("2026-11-01"), Done: true, DoneDate: d("2026-11-02")}
	n.Normalize()
	if n.Kind != Call || !n.Due.IsZero() || n.Done || n.Open() {
		t.Fatalf("telefonát nemá termín %+v", n)
	}
	task := Activity{Kind: Task, Text: "x", Due: d("2026-11-05")}
	if !task.Open() || !task.Late(d("2026-11-10")) || task.Late(d("2026-11-05")) {
		t.Fatalf("úloha %+v", task)
	}
	task.SetDone(true, d("2026-11-10"))
	if task.Open() || task.Late(d("2026-11-10")) || task.DoneDate.String() != "2026-11-10" {
		t.Fatalf("splnená %+v", task)
	}
	task.SetDone(false, d("2026-11-11"))
	if !task.Open() || !task.DoneDate.IsZero() {
		t.Fatalf("znovu otvorená %+v", task)
	}
	n.SetDone(true, d("2026-11-10"))
	if n.Done {
		t.Error("poznámka sa nesplní")
	}
}
