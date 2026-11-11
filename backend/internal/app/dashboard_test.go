package app

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/calendar"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
)

func TestDashboard(t *testing.T) {
	a, c := newApp(t, edition.Desktop) // dnes 29. 10. 2026
	late := issuedOn(t, a, c.ID, "Dlžník s.r.o.", "31322832", "2026-10-20")
	soon := issuedOn(t, a, c.ID, "Agro s.r.o.", "", "2026-11-05")
	if _, _, err := a.SavePurchase(ctx, c.ID, validPurchase()); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveDraft(ctx, c.ID, validDraft()); err != nil {
		t.Fatal(err)
	}
	d, err := a.Dashboard(ctx, c.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if d.Year != 2026 || len(d.Months) != 12 {
		t.Fatalf("rok %d, mesiace %d", d.Year, len(d.Months))
	}
	// príjmy podľa dátumu dodania (6. a 22. 10.), výdavky z prijatej faktúry
	inc := late.Summary.Total
	if d.Net {
		inc = late.Summary.Base
	}
	if !d.Months[9].Income.Equal(inc.Add(inc)) || !d.Income.Equal(inc.Add(inc)) || d.Expense.Sign() <= 0 || !d.Result.Equal(d.Income.Sub(d.Expense)) {
		t.Fatalf("príjmy a výdavky %+v %s %s %s", d.Months[9], d.Income, d.Expense, d.Result)
	}
	r := d.Receivables
	if r == nil || r.Count != 2 || r.OverdueCount != 1 || !r.Total.Equal(late.Summary.Total.Add(soon.Summary.Total)) || r.Top[0].ID != late.ID {
		t.Fatalf("pohľadávky %+v", r)
	}
	if d.Payables == nil || d.Payables.Count != 1 || d.Payables.OverdueCount != 1 {
		t.Fatalf("záväzky %+v", d.Payables)
	}
	found := false
	for _, e := range d.Events {
		found = found || (e.Kind == calendar.KindReceive && e.Date == soon.DueDate)
		if e.Done || e.Kind == calendar.KindHoliday {
			t.Fatalf("splnený termín alebo sviatok: %+v", e)
		}
	}
	if !found {
		t.Fatalf("splatnosť faktúry chýba v termínoch: %+v", d.Events)
	}
	if d.Work == nil || d.Work.Drafts != 1 || d.Money == nil {
		t.Fatalf("práca %+v peniaze %+v", d.Work, d.Money)
	}
	// minulý rok: prázdne mesiace
	if old, err := a.Dashboard(ctx, c.ID, 2025); err != nil || old.Income.Sign() != 0 {
		t.Fatalf("2025: %+v %v", old.Income, err)
	}
}
