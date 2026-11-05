package household

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

var m = money.MustParse

func d(y, mo, day int) invoice.Date {
	return invoice.NewDate(y, 1, 1).AddMonths(mo - 1).AddDays(day - 1)
}

func TestValidateEntry(t *testing.T) {
	ok := Entry{Date: d(2026, 3, 1), AccountID: 1, Amount: m("-12.50"), Category: "Potraviny"}
	cases := []struct {
		name  string
		e     Entry
		field string
	}{
		{"v poriadku", ok, ""},
		{"bez dátumu", Entry{AccountID: 1, Amount: m("1"), Category: "x"}, "date"},
		{"bez účtu", Entry{Date: d(2026, 1, 1), Amount: m("1"), Category: "x"}, "accountId"},
		{"nulová suma", Entry{Date: d(2026, 1, 1), AccountID: 1, Category: "x"}, "amount"},
		{"bez kategórie", Entry{Date: d(2026, 1, 1), AccountID: 1, Amount: m("1")}, "category"},
		{"prevod bez kategórie je v poriadku", Entry{Date: d(2026, 1, 1), AccountID: 1, ToAccountID: 2, Amount: m("100")}, ""},
		{"prevod na ten istý účet", Entry{Date: d(2026, 1, 1), AccountID: 1, ToAccountID: 1, Amount: m("100")}, "toAccountId"},
		{"záporný prevod", Entry{Date: d(2026, 1, 1), AccountID: 1, ToAccountID: 2, Amount: m("-100")}, "amount"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.e.Validate()
			if c.field == "" {
				if err != nil {
					t.Fatalf("chyba %v", err)
				}
				return
			}
			ve, okv := err.(*invoice.ValidationError)
			if !okv || ve.Fields[0].Field != c.field {
				t.Fatalf("chcem chybu %s, mám %v", c.field, err)
			}
		})
	}
}

func TestBalances(t *testing.T) {
	accs := []Account{{ID: 1, Opening: m("100")}, {ID: 2, Opening: m("0")}}
	es := []Entry{
		{AccountID: 1, Amount: m("1500")},                // výplata
		{AccountID: 1, Amount: m("-45.30")},              // nákup
		{AccountID: 1, ToAccountID: 2, Amount: m("200")}, // výber z bankomatu
		{AccountID: 2, Amount: m("-20")},
	}
	b := Balances(accs, es)
	if !b[1].Equal(m("1354.70")) || !b[2].Equal(m("180")) {
		t.Fatalf("zostatky %v %v", b[1], b[2])
	}
}

func TestSummarize(t *testing.T) {
	cats := []Category{{Name: "Mzda", Income: true}, {Name: "Potraviny", Budget: m("400")}}
	es := []Entry{
		{Date: d(2026, 1, 5), AccountID: 1, Amount: m("1500"), Category: "Mzda"},
		{Date: d(2026, 1, 7), AccountID: 1, Amount: m("-120"), Category: "Potraviny"},
		{Date: d(2026, 1, 9), AccountID: 1, Amount: m("20"), Category: "Potraviny"}, // vrátený tovar
		{Date: d(2026, 2, 1), AccountID: 1, Amount: m("-450"), Category: "Potraviny"},
		{Date: d(2026, 2, 3), AccountID: 1, ToAccountID: 2, Amount: m("300")}, // prevod sa nepočíta
		{Date: d(2025, 12, 31), AccountID: 1, Amount: m("-99"), Category: "Potraviny"},
		{Date: d(2026, 3, 1), AccountID: 1, Amount: m("-10"), Category: "Stará kategória"},
	}
	ov := Summarize(2026, cats, es)
	if !ov.TotalIn.Equal(m("1500")) || !ov.TotalOut.Equal(m("560")) || !ov.Net.Equal(m("940")) {
		t.Fatalf("spolu %v %v", ov.TotalIn, ov.TotalOut)
	}
	if !ov.Expense[0].Equal(m("100")) || !ov.Expense[1].Equal(m("450")) {
		t.Fatalf("mesiace %v %v", ov.Expense[0], ov.Expense[1])
	}
	if len(ov.Categories) != 3 || ov.Categories[2].Name != "Stará kategória" || ov.Categories[2].Income {
		t.Fatalf("kategórie %+v", ov.Categories)
	}
	cases := []struct {
		month   int
		actual  string
		percent int64
		over    bool
	}{
		{1, "100", 25, false},
		{2, "450", 112, true},
	}
	for _, c := range cases {
		b := Budget(ov, c.month)
		if len(b) == 0 || b[0].Name != "Potraviny" || !b[0].Actual.Equal(m(c.actual)) || b[0].Percent != c.percent || b[0].Over != c.over {
			t.Fatalf("mesiac %d: %+v", c.month, b)
		}
	}
	if b := Budget(ov, 4); len(b) != 1 || !b[0].Actual.IsZero() {
		t.Fatalf("apríl: kategória s rozpočtom sa ukáže aj bez výdavkov %+v", b)
	}
}

func TestSuggest(t *testing.T) {
	cats := DefaultCategories
	hist := []Entry{
		{Date: d(2026, 1, 1), Payee: "Mäsiarstvo Novák", Category: "Iné výdavky", Amount: m("-5")},
		{Date: d(2026, 2, 1), Payee: "Mäsiarstvo Novák", Category: "Potraviny", Amount: m("-5")},
	}
	cases := []struct {
		text   string
		income bool
		want   string
	}{
		{"MASIARSTVO NOVAK", false, "Potraviny"}, // podľa histórie, najnovší
		{"LIDL DEKUJE ZA NAKUP", false, "Potraviny"},
		{"Slovnaft a.s. 2341", false, "Auto a doprava"},
		{"ZSE Energia, a.s.", false, "Energie"},
		{"Exekútorský úrad JUDr. X", false, "Splátky a dlhy"},
		{"neznámy obchod", false, ""},
		{"LIDL", true, ""}, // príjem do výdavkovej kategórie nie
		{"", false, ""},
	}
	for _, c := range cases {
		if got := Suggest(c.text, c.income, hist, cats); got != c.want {
			t.Errorf("%q: %q, chcem %q", c.text, got, c.want)
		}
	}
}
