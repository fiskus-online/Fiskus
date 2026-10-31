package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/calendar"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/mail"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Upozornenia e-mailom na blížiace sa lehoty: daňové termíny, vlastné
// termíny a splatné prijaté faktúry. Každý termín sa pripomenie raz;
// beží každú hodinu, posiela sa až od 7:00.

// noticeKinds sú druhy termínov, na ktoré sa upozorňuje.
var noticeKinds = map[calendar.Kind]bool{calendar.KindTax: true, calendar.KindOwn: true, calendar.KindPay: true}

func noticeKey(companyID int64) string { return fmt.Sprintf("deadline.notified.%d", companyID) }

// dueEvents vráti nesplnené termíny od dneška do dní vopred, o ktorých ešte
// nebolo upozornenie (kľúč = ID termínu + dátum).
func (a *App) dueEvents(ctx context.Context, companyID int64, days int, sent map[string]string) ([]calendar.Event, error) {
	today := a.Today()
	v, err := a.Calendar(ctx, companyID, today, today.AddDays(days))
	if err != nil {
		return nil, err
	}
	var out []calendar.Event
	for _, e := range v.Events {
		if !noticeKinds[e.Kind] || e.Done {
			continue
		}
		if _, ok := sent[e.ID+"@"+e.Date.String()]; ok {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// NotifyDeadlines pošle upozornenie na blížiace sa lehoty všetkým firmám,
// ktoré majú nastavenú adresu (E-mail → Upozornenia na lehoty).
func (a *App) NotifyDeadlines(ctx context.Context) {
	if a.clock().Hour() < 7 {
		return
	}
	ctx = auth.WithActor(ctx, auth.System)
	companies, err := a.Store.ListCompanies(ctx)
	if err != nil {
		return
	}
	for _, co := range companies {
		if err := a.notifyDeadlines(ctx, co); err != nil {
			log.Printf("upozornenie na lehoty (firma %d): %v", co.ID, err)
		}
	}
}

func (a *App) notifyDeadlines(ctx context.Context, co store.Company) error {
	s, err := a.loadMail(ctx, co.ID)
	if err != nil || !s.Configured() || s.NotifyDeadlines == "" {
		return err
	}
	to, err := mail.ParseList(s.NotifyDeadlines)
	if err != nil || len(to) == 0 {
		return fmt.Errorf("neplatná adresa %q", s.NotifyDeadlines)
	}
	days := s.NotifyDays
	if days < 1 || days > 14 {
		days = 3
	}
	sent := map[string]string{}
	if v, err := a.Store.GetSetting(ctx, noticeKey(co.ID)); err == nil {
		_ = json.Unmarshal([]byte(v), &sent)
	}
	events, err := a.dueEvents(ctx, co.ID, days, sent)
	if err != nil || len(events) == 0 {
		return err
	}
	s.BccSelf = false
	m := mail.Message{To: to, Subject: deadlineSubject(events, a.Today()), Body: a.deadlineBody(co, events)}
	if _, err := a.deliver(ctx, co.ID, s, m, store.Email{DocKind: MailNotice}); err != nil {
		return err // pri chybe sa skúsi znova o hodinu
	}
	today := a.Today()
	for _, e := range events {
		sent[e.ID+"@"+e.Date.String()] = today.String()
	}
	// staré záznamy (termíny pred mesiacom) sa zahodia
	for k := range sent {
		if i := strings.LastIndexByte(k, '@'); i >= 0 {
			if d, err := invoice.ParseDate(k[i+1:]); err == nil && d.Before(today.AddDays(-31)) {
				delete(sent, k)
			}
		}
	}
	b, _ := json.Marshal(sent)
	return a.Store.SetSetting(ctx, noticeKey(co.ID), string(b))
}

func deadlineSubject(es []calendar.Event, today invoice.Date) string {
	first := es[0]
	when := "o " + daysText(today.DaysUntil(first.Date))
	if first.Date == today {
		when = "dnes"
	}
	if len(es) == 1 {
		return fmt.Sprintf("Fiškus: %s — %s", first.Title, when)
	}
	return fmt.Sprintf("Fiškus: %d termíny sa blížia, prvý %s", len(es), when)
}

func daysText(n int) string {
	switch {
	case n == 1:
		return "1 deň"
	case n >= 2 && n <= 4:
		return fmt.Sprintf("%d dni", n)
	}
	return fmt.Sprintf("%d dní", n)
}

func (a *App) deadlineBody(co store.Company, es []calendar.Event) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Dobrý deň,\n\nblížia sa termíny firmy %s:\n\n", co.Name)
	for _, e := range es {
		line := fmt.Sprintf("• %d. %d. %d — %s", e.Date.Day(), e.Date.Month(), e.Date.Year(), e.Title)
		if e.Amount != nil && !e.Amount.IsZero() {
			line += ", " + money2(*e.Amount, e.Curr)
		}
		b.WriteString(line + "\n")
		if e.Detail != "" {
			b.WriteString("  " + e.Detail + "\n")
		}
	}
	b.WriteString("\nVšetky termíny nájdete v programe Fiškus v časti Kalendár")
	if u := strings.TrimRight(a.PublicURL, "/"); u != "" {
		b.WriteString(": " + u + "/kalendar")
	}
	b.WriteString(".\n\nTúto správu poslal Fiškus automaticky. Adresu a počet dní vopred zmeníte v Nastaveniach → E-mail.\n")
	return b.String()
}
