package app

import (
	"strings"
	"testing"
	"time"

	"github.com/fiskus-online/fiskus/backend/internal/calendar"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
)

func TestNotifyDeadlines(t *testing.T) {
	a, c, f := mailApp(t)
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC) // Today v testoch je 29. 10. 2026
	a.Now = func() time.Time { return now }
	set := smtpOK
	set.NotifyDeadlines, set.NotifyDays = "ja@firma.sk", 3
	if _, err := a.SetMailSettings(ctx, c.ID, set, false); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveOwnEvent(ctx, c.ID, calendar.Own{Title: "Odvody do Sociálnej poisťovne", Date: invoice.MustDate("2026-10-31")}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveOwnEvent(ctx, c.ID, calendar.Own{Title: "STK", Date: invoice.MustDate("2026-11-20")}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		hour    int
		wantNew int // nových správ
		check   func(t *testing.T)
	}{
		{"v noci sa neposiela", 5, 0, nil},
		{"ráno upozornenie", 8, 1, func(t *testing.T) {
			m := f.last()
			if m.To[0] != "ja@firma.sk" || !strings.Contains(m.Body, "Odvody do Sociálnej poisťovne") || strings.Contains(m.Body, "STK") || !strings.Contains(m.Subject, "o 2 dni") {
				t.Errorf("správa %q\n%s", m.Subject, m.Body)
			}
		}},
		{"druhýkrát nie", 10, 0, nil},
	}
	for _, tt := range tests {
		before := len(f.sent)
		now = time.Date(2026, 10, 29, tt.hour, 0, 0, 0, time.UTC)
		a.NotifyDeadlines(ctx)
		if got := len(f.sent) - before; got != tt.wantNew {
			t.Fatalf("%s: %d správ", tt.name, got)
		}
		if tt.check != nil {
			tt.check(t)
		}
	}
	// bez adresy sa neposiela nič
	set.NotifyDeadlines = ""
	_, _ = a.SetMailSettings(ctx, c.ID, set, false)
	_, _ = a.SaveOwnEvent(ctx, c.ID, calendar.Own{Title: "Poistka", Date: invoice.MustDate("2026-10-30")})
	before := len(f.sent)
	a.NotifyDeadlines(ctx)
	if len(f.sent) != before {
		t.Error("bez adresy sa poslalo")
	}
	// neplatný počet dní
	set.NotifyDays = 30
	if _, err := a.SetMailSettings(ctx, c.ID, set, false); err == nil || !strings.Contains(err.Error(), "1 až 14") {
		t.Errorf("počet dní: %v", err)
	}
}
