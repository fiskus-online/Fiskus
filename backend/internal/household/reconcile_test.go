package household

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
)

func TestReconcile(t *testing.T) {
	acc := Account{ID: 1, Opening: m("100")}
	es := []Entry{
		{Date: d(2026, 3, 1), AccountID: 1, Amount: m("-20"), Category: "Potraviny"},
		{Date: d(2026, 3, 5), AccountID: 1, ToAccountID: 2, Amount: m("30")}, // prevod z účtu
		{Date: d(2026, 3, 6), AccountID: 2, ToAccountID: 1, Amount: m("5")},  // prevod na účet
		{Date: d(2026, 3, 6), AccountID: 2, Amount: m("-99"), Category: "x"}, // iný účet
		{Date: d(2026, 3, 10), AccountID: 1, Amount: m("500"), Category: "Mzda"},
	}
	cases := []struct {
		name      string
		date      string
		statement string
		book      string
		diff      string
		later     int
	}{
		{"pred prvým pohybom", "2026-02-28", "100", "100", "0", 4},
		{"deň pohybu sa počíta", "2026-03-01", "80", "80", "0", 3},
		{"prevody oboma smermi", "2026-03-06", "55", "55", "0", 1},
		{"na výpise je menej", "2026-03-31", "552.50", "555", "-2.50", 0},
		{"na výpise je viac", "2026-03-31", "556", "555", "1", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Reconcile(acc, es, invoice.MustDate(c.date), m(c.statement))
			if !r.Book.Equal(m(c.book)) || !r.Difference.Equal(m(c.diff)) || r.Later != c.later {
				t.Fatalf("got book %s diff %s later %d", r.Book, r.Difference, r.Later)
			}
		})
	}
}

func TestStale(t *testing.T) {
	es := []Entry{{Date: d(2026, 3, 1), AccountID: 1, Amount: m("-20"), Category: "x"}}
	cases := []struct {
		name string
		acc  Account
		want string
	}{
		{"neodsúhlasený", Account{ID: 1, Opening: m("100")}, "0"},
		{"platí", Account{ID: 1, Opening: m("100"), Reconciled: &Reconciled{Date: d(2026, 3, 31), Balance: m("80")}}, "0"},
		{"pohyby sa zmenili", Account{ID: 1, Opening: m("100"), Reconciled: &Reconciled{Date: d(2026, 3, 31), Balance: m("90")}}, "-10"},
	}
	for _, c := range cases {
		if got := Stale(c.acc, es); !got.Equal(m(c.want)) {
			t.Errorf("%s: got %s", c.name, got)
		}
	}
}

func TestAdjustCategory(t *testing.T) {
	cats := []Category{{Name: "Mzda", Income: true}, {Name: "Iné príjmy", Income: true}, {Name: "Potraviny"}, {Name: "Ostatné"}}
	cases := []struct {
		income bool
		want   string
	}{{true, "Iné príjmy"}, {false, "Ostatné"}}
	for _, c := range cases {
		if got := AdjustCategory(cats, c.income); got != c.want {
			t.Errorf("income %v: got %q", c.income, got)
		}
	}
}
