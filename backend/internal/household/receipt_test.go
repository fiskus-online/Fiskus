package household

import (
	"testing"
)

func TestSuggestItem(t *testing.T) {
	hist := []Entry{
		{Date: d(2026, 2, 1), Split: []Split{{Category: "Voľný čas", Note: "Kofola 2L", Amount: m("-1.99")}}},
	}
	cases := []struct{ name, want string }{
		{"ROZOK KLASIK 40G", "Potraviny"},
		{"Mlieko polotučné 1,5%", "Potraviny"},
		{"SAMPON HEAD&SHOULDERS", "Drogéria a domácnosť"},
		{"Toal.pap. 3vr 8ks", "Drogéria a domácnosť"},
		{"MOTOROVY OLEJ 5W30", "Auto a doprava"},
		{"Slnečnicový olej 1l", "Potraviny"},
		{"IBALGIN 400 24TBL", "Zdravie"},
		{"KOFOLA 2L", "Voľný čas"}, // história má prednosť
		{"Darčeková taška", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := SuggestItem(c.name, hist, DefaultCategories); got != c.want {
			t.Errorf("%q: %q, chcem %q", c.name, got, c.want)
		}
	}
}

func TestReceiptSplit(t *testing.T) {
	items := []ReceiptItem{
		{Name: "Rožok", Quantity: "4", Amount: m("0.40"), Category: "Potraviny"},
		{Name: "Šampón", Amount: m("3.49"), Category: "Drogéria a domácnosť"},
		{Name: "Zľava Lidl Plus", Amount: m("-0.50")},
		{Name: "Taška", Amount: m("0.25")},
		{Name: "prázdny", Amount: m("0")},
	}
	cases := []struct {
		name  string
		total string
		lines int
		last  string
	}{
		{"sedí", "3.64", 4, "Taška"},
		{"zaokrúhlenie", "3.65", 5, "Zaokrúhlenie"},
		{"chýbajúca položka", "10.00", 5, "Rozdiel k celkovej sume"},
		{"bez celkovej sumy", "0", 4, "Taška"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			split, amt := ReceiptSplit(items, m(c.total), "Potraviny")
			if len(split) != c.lines || split[len(split)-1].Note != c.last {
				t.Fatalf("položky %+v", split)
			}
			e := Entry{Date: d(2026, 3, 1), AccountID: 1, Amount: amt, Split: split}
			e.Normalize()
			if err := e.Validate(); err != nil {
				t.Fatalf("pohyb z bločku: %v", err)
			}
			if split[2].Category != "Potraviny" || !split[0].Amount.Equal(m("-0.40")) || split[0].Quantity != "4" {
				t.Fatalf("zľava bez kategórie dostane kategóriu obchodu, sumy záporné: %+v", split)
			}
		})
	}
}

func TestSplitEntry(t *testing.T) {
	e := Entry{Date: d(2026, 3, 1), AccountID: 1, Amount: m("-10"), Split: []Split{
		{Category: "Drogéria a domácnosť", Amount: m("-3")}, {Category: "Potraviny", Amount: m("-4")}, {Category: "Potraviny", Amount: m("-3")},
	}}
	e.Normalize()
	if e.Category != "Potraviny" {
		t.Fatalf("kategória najväčšej časti: %q", e.Category)
	}
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := e
	bad.Split = append([]Split(nil), e.Split...)
	bad.Split[0].Amount = m("-2")
	if err := bad.Validate(); err == nil {
		t.Fatal("súčet položiek nesedí")
	}
	ov := Summarize(2026, DefaultCategories, []Entry{e})
	for _, c := range ov.Categories {
		switch c.Name {
		case "Potraviny":
			if !c.Total.Equal(m("7")) {
				t.Fatalf("potraviny %v", c.Total)
			}
		case "Drogéria a domácnosť":
			if !c.Total.Equal(m("3")) {
				t.Fatalf("drogéria %v", c.Total)
			}
		}
	}
	if !ov.TotalOut.Equal(m("10")) {
		t.Fatalf("výdavky %v", ov.TotalOut)
	}
}
