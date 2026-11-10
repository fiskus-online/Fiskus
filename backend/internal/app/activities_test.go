package app

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/activity"
	"github.com/fiskus-online/fiskus/backend/internal/calendar"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/job"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/quote"
	"github.com/fiskus-online/fiskus/backend/internal/store"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

func TestActivities(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	d := invoice.MustDate
	ct, _ := a.SaveContact(ctx, c.ID, store.Contact{Name: "Agro Liptov družstvo"})
	other, _ := a.SaveContact(ctx, c.ID, store.Contact{Name: "Mlyn Brezno s.r.o."})
	j, _ := a.SaveCrmJob(ctx, c.ID, job.Job{Name: "Rozvádzač sušička", ContactID: ct.ID})
	q, _ := a.SaveQuote(ctx, c.ID, quote.Quote{Customer: invoice.Party{Name: other.Name, Country: "SK"}, ContactID: other.ID, IssueDate: d("2026-10-20"),
		ValidUntil: d("2026-11-20"), Currency: "EUR", Lines: []invoice.Line{{Description: "Servis", Quantity: money.MustParse("1"), Unit: "ks", UnitPrice: money.MustParse("90"), Rate: vat.Rate23}}})

	if _, err := a.SaveActivity(ctx, c.ID, activity.Activity{Text: "x"}); !fieldErr(err, "contactId") {
		t.Fatalf("bez priradenia: %v", err)
	}
	if _, err := a.SaveActivity(ctx, c.ID, activity.Activity{Text: "x", JobID: 999}); !fieldErr(err, "jobId") {
		t.Fatalf("neznáma zákazka: %v", err)
	}
	call, err := a.SaveActivity(ctx, c.ID, activity.Activity{Kind: activity.Call, Text: "Volal, chce termín montáže", JobID: j.ID})
	if err != nil {
		t.Fatal(err)
	}
	if call.ContactID != ct.ID || call.Date.String() != "2026-10-29" || call.Created.IsZero() || call.About != j.Number+" "+j.Name || call.Link == "" {
		t.Fatalf("telefonát %+v", call)
	}
	task, err := a.SaveActivity(ctx, c.ID, activity.Activity{Kind: activity.Task, Text: "Ozvať sa k ponuke", QuoteID: q.ID, Due: d("2026-10-27")})
	if err != nil {
		t.Fatal(err)
	}
	if task.ContactID != other.ID || !task.Late {
		t.Fatalf("úloha %+v", task)
	}
	if _, err := a.SaveActivity(ctx, c.ID, activity.Activity{Kind: activity.Note, Text: "Platí načas", ContactID: ct.ID, Date: d("2026-10-01")}); err != nil {
		t.Fatal(err)
	}
	byContact, _ := a.Activities(ctx, c.ID, ActivityFilter{ContactID: ct.ID})
	byJob, _ := a.Activities(ctx, c.ID, ActivityFilter{JobID: j.ID})
	byQuote, _ := a.Activities(ctx, c.ID, ActivityFilter{QuoteID: q.ID})
	open, _ := a.Activities(ctx, c.ID, ActivityFilter{OpenTasks: true})
	if len(byContact) != 2 || byContact[0].ID != call.ID || len(byJob) != 1 || len(byQuote) != 1 || len(open) != 1 {
		t.Fatalf("filtre %d %d %d %d", len(byContact), len(byJob), len(byQuote), len(open))
	}
	// kalendár: zmeškaná úloha
	v, err := a.Calendar(ctx, c.ID, d("2026-10-01"), d("2026-10-31"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range v.Events {
		if e.Kind == calendar.KindTask && e.Title == "Ozvať sa k ponuke" && e.Overdue && e.Link != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("úloha v kalendári %+v", v.Events)
	}
	// splnenie
	if _, err := a.SetActivityDone(ctx, c.ID, call.ID, true); !fieldErr(err, "kind") {
		t.Fatalf("telefonát sa nesplní: %v", err)
	}
	done, err := a.SetActivityDone(ctx, c.ID, task.ID, true)
	if err != nil || !done.Done || done.Late || done.DoneDate.String() != "2026-10-29" {
		t.Fatalf("splnená %+v %v", done, err)
	}
	if open, _ := a.Activities(ctx, c.ID, ActivityFilter{OpenTasks: true}); len(open) != 0 {
		t.Fatalf("otvorené po splnení %+v", open)
	}
	// úprava zachová autora a čas vzniku
	call.Text = "Volal, montáž v utorok"
	upd, err := a.SaveActivity(ctx, c.ID, call.Activity)
	if err != nil || upd.Created != call.Created || upd.Text != "Volal, montáž v utorok" {
		t.Fatalf("úprava %+v %v", upd, err)
	}
	if err := a.DeleteActivity(ctx, c.ID, call.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := a.Activities(ctx, c.ID, ActivityFilter{JobID: j.ID}); len(list) != 0 {
		t.Fatalf("po zmazaní %+v", list)
	}
}
