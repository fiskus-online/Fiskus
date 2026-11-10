package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/fiskus-online/fiskus/backend/internal/activity"
	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/calendar"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// CRM aktivity: záznam pri kontakte, zákazke alebo ponuke. Záznam zo
// zákazky či ponuky dostane aj kontakt zákazníka, aby ho bolo vidieť
// v histórii kontaktu. Nesplnené úlohy sú v kalendári.

// ActivityFilter vyberie aktivity (0 = bez obmedzenia).
type ActivityFilter struct {
	ContactID int64
	JobID     int64
	QuoteID   int64
	OpenTasks bool // iba nesplnené úlohy
}

// ActivityView je aktivita s označením oneskorenia a názvom súvisiacej veci.
type ActivityView struct {
	activity.Activity
	Late  bool   `json:"late"`
	About string `json:"about,omitempty"` // „Z2026001 Rekonštrukcia…“ / „CP20260005“ / kontakt
	Link  string `json:"link,omitempty"`
}

func activityErr(errs []activity.FieldError) error {
	if len(errs) == 0 {
		return nil
	}
	return &invoice.ValidationError{Fields: errs}
}

// Activities vráti aktivity podľa filtra, najnovšie prvé.
func (a *App) Activities(ctx context.Context, companyID int64, f ActivityFilter) ([]ActivityView, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return nil, err
	}
	list, err := a.Store.ListActivities(ctx, companyID)
	if err != nil {
		return nil, err
	}
	today := a.Today()
	out := []ActivityView{}
	for _, x := range list {
		if (f.ContactID != 0 && x.ContactID != f.ContactID) || (f.JobID != 0 && x.JobID != f.JobID) ||
			(f.QuoteID != 0 && x.QuoteID != f.QuoteID) || (f.OpenTasks && !x.Open()) {
			continue
		}
		v := ActivityView{Activity: x, Late: x.Late(today)}
		v.About, v.Link = a.activityAbout(ctx, companyID, x)
		out = append(out, v)
	}
	return out, nil
}

// activityAbout: čoho sa aktivita týka (zákazka, ponuka, kontakt) a odkaz.
func (a *App) activityAbout(ctx context.Context, companyID int64, x activity.Activity) (string, string) {
	if x.JobID != 0 {
		if j, err := a.Store.GetJob(ctx, companyID, x.JobID); err == nil {
			return j.Number + " " + j.Name, fmt.Sprintf("/zakazky/%d", j.ID)
		}
	}
	if x.QuoteID != 0 {
		if q, err := a.Store.GetQuote(ctx, companyID, x.QuoteID); err == nil {
			return firstNonBlank(q.Number, "koncept ponuky") + " " + q.Customer.Name, fmt.Sprintf("/ponuky/%d", q.ID)
		}
	}
	if x.ContactID != 0 {
		if c, err := a.Store.GetContact(ctx, companyID, x.ContactID); err == nil {
			return c.Name, fmt.Sprintf("/adresar/%d", c.ID)
		}
	}
	return "", ""
}

// SaveActivity založí (ID 0) alebo upraví aktivitu. Autor a čas vzniku
// sa nemenia; kontakt sa doplní zo zákazky alebo ponuky.
func (a *App) SaveActivity(ctx context.Context, companyID int64, x activity.Activity) (ActivityView, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return ActivityView{}, err
	}
	var before *activity.Activity
	if x.ID != 0 {
		old, err := a.Store.GetActivity(ctx, companyID, x.ID)
		if err != nil {
			return ActivityView{}, err
		}
		before = &old
		x.Author, x.Created = old.Author, old.Created
		if x.Done && !old.Done {
			x.DoneDate = a.Today()
		} else if x.Done {
			x.DoneDate = old.DoneDate
		}
	} else {
		x.Author, x.Created = auth.ActorFrom(ctx).Name, a.now()
		if x.Date.IsZero() {
			x.Date = a.Today()
		}
		if x.Done {
			x.DoneDate = a.Today()
		}
	}
	if err := a.linkActivity(ctx, companyID, &x); err != nil {
		return ActivityView{}, err
	}
	x.Normalize()
	if err := activityErr(x.Validate()); err != nil {
		return ActivityView{}, err
	}
	if err := a.Store.SaveActivity(ctx, companyID, &x); err != nil {
		return ActivityView{}, err
	}
	action, what := "create", "Nový záznam CRM"
	if before != nil {
		action, what = "update", "Úprava záznamu CRM"
	}
	v := ActivityView{Activity: x, Late: x.Late(a.Today())}
	v.About, v.Link = a.activityAbout(ctx, companyID, x)
	a.record(ctx, companyID, audit.CatInvoices, action, "activity", x.ID, what+": "+v.About, before, x)
	return v, nil
}

// linkActivity overí zákazku, ponuku a kontakt a doplní kontakt zákazníka.
func (a *App) linkActivity(ctx context.Context, companyID int64, x *activity.Activity) error {
	bad := func(f, m string) error {
		return activityErr([]activity.FieldError{{Field: f, Message: m}})
	}
	if x.JobID != 0 {
		j, err := a.Store.GetJob(ctx, companyID, x.JobID)
		if errors.Is(err, store.ErrNotFound) {
			return bad("jobId", "Zákazka sa nenašla")
		} else if err != nil {
			return err
		}
		if x.ContactID == 0 {
			x.ContactID = j.ContactID
		}
	}
	if x.QuoteID != 0 {
		q, err := a.Store.GetQuote(ctx, companyID, x.QuoteID)
		if errors.Is(err, store.ErrNotFound) {
			return bad("quoteId", "Ponuka sa nenašla")
		} else if err != nil {
			return err
		}
		if x.ContactID == 0 {
			x.ContactID = q.ContactID
		}
	}
	if x.ContactID != 0 {
		if _, err := a.Store.GetContact(ctx, companyID, x.ContactID); errors.Is(err, store.ErrNotFound) {
			return bad("contactId", "Kontakt sa nenašiel")
		} else if err != nil {
			return err
		}
	}
	return nil
}

// SetActivityDone označí úlohu ako splnenú alebo nesplnenú.
func (a *App) SetActivityDone(ctx context.Context, companyID, id int64, done bool) (ActivityView, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return ActivityView{}, err
	}
	x, err := a.Store.GetActivity(ctx, companyID, id)
	if err != nil {
		return ActivityView{}, err
	}
	if x.Kind != activity.Task {
		return ActivityView{}, activityErr([]activity.FieldError{{Field: "kind", Message: "Splniť možno iba úlohu"}})
	}
	before := x
	x.SetDone(done, a.Today())
	if err := a.Store.SaveActivity(ctx, companyID, &x); err != nil {
		return ActivityView{}, err
	}
	v := ActivityView{Activity: x, Late: x.Late(a.Today())}
	v.About, v.Link = a.activityAbout(ctx, companyID, x)
	what := "Splnená úloha: "
	if !done {
		what = "Znovu otvorená úloha: "
	}
	a.record(ctx, companyID, audit.CatInvoices, "update", "activity", x.ID, what+x.Text, before, x)
	return v, nil
}

// DeleteActivity zmaže aktivitu.
func (a *App) DeleteActivity(ctx context.Context, companyID, id int64) error {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return err
	}
	x, err := a.Store.GetActivity(ctx, companyID, id)
	if err != nil {
		return err
	}
	if err := a.Store.DeleteActivity(ctx, companyID, id); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatInvoices, "delete", "activity", id, "Zmazaný záznam CRM: "+x.Text, x, nil)
	return nil
}

// taskEvents: úlohy s termínom do kalendára (splnené aj nesplnené v rozsahu,
// nesplnené po termíne ako zmeškané).
func (a *App) taskEvents(ctx context.Context, companyID int64, to, today invoice.Date) ([]calendar.Event, error) {
	list, err := a.Store.ListActivities(ctx, companyID)
	if err != nil {
		return nil, err
	}
	var out []calendar.Event
	for _, x := range list {
		if x.Kind != activity.Task || x.Due.IsZero() || x.Due.After(to) {
			continue
		}
		about, link := a.activityAbout(ctx, companyID, x)
		e := calendar.Event{ID: fmt.Sprintf("task-%d", x.ID), Date: x.Due, Kind: calendar.KindTask, Title: x.Text, Detail: about,
			Link: link, Done: x.Done, Overdue: x.Late(today)}
		out = append(out, e)
	}
	return out, nil
}
