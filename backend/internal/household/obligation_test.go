package household

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
)

func TestObligationValidate(t *testing.T) {
	ok := Obligation{Kind: Loan, Name: "Pôžička", Principal: m("1200"), Installment: m("100"), Frequency: Monthly, FirstDue: d(2026, 1, 15)}
	cases := []struct {
		name  string
		edit  func(*Obligation)
		field string
	}{
		{"v poriadku", func(*Obligation) {}, ""},
		{"bez názvu", func(o *Obligation) { o.Name = "" }, "name"},
		{"bez splátky", func(o *Obligation) { o.Installment = m("0") }, "installment"},
		{"bez splatnosti", func(o *Obligation) { o.FirstDue = invoice.Date{} }, "firstDue"},
		{"dlh bez sumy aj počtu", func(o *Obligation) { o.Principal = m("0") }, "principal"},
		{"pravidelná bez sumy je v poriadku", func(o *Obligation) { o.Kind, o.Principal = Regular, m("0") }, ""},
		{"splatené viac ako dlh", func(o *Obligation) { o.PaidBefore = m("1300") }, "paidBefore"},
		{"neznámy druh", func(o *Obligation) { o.Kind = "x" }, "kind"},
		{"pokuta jednorazovo", func(o *Obligation) { o.Kind, o.Frequency, o.Principal, o.Installment = Fine, Once, m("0"), m("50") }, ""},
		{"zlý mesiac", func(o *Obligation) { o.Months = []int{9, 13} }, "months"},
		{"pripomienka priďaleko", func(o *Obligation) { o.RemindDays = 61 }, "remindDays"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := ok
			c.edit(&o)
			o.Normalize()
			err := o.Validate()
			if c.field == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			ve, okv := err.(*invoice.ValidationError)
			if !okv || ve.Fields[0].Field != c.field {
				t.Fatalf("chcem %s, mám %v", c.field, err)
			}
		})
	}
}

func TestSchedule(t *testing.T) {
	loan := Obligation{ID: 7, Kind: Loan, Principal: m("1050"), Installment: m("200"), Frequency: Monthly, FirstDue: d(2026, 1, 31)}
	today := d(2026, 5, 10)
	es := []Entry{
		{ObligationID: 7, Amount: m("-200")},
		{ObligationID: 7, Amount: m("-200")},
		{ObligationID: 7, Amount: m("-50")},
		{ObligationID: 8, Amount: m("-999")},                              // iný záväzok
		{ObligationID: 7, AccountID: 1, ToAccountID: 2, Amount: m("500")}, // prevod sa nepočíta
	}
	paid := Paid(loan, es)
	if !paid.Equal(m("450")) {
		t.Fatalf("zaplatené %v", paid)
	}
	s := Schedule(loan, paid, today, today)
	want := []struct {
		date   invoice.Date
		amount string
		status DueStatus
	}{
		{d(2026, 1, 31), "200", DuePaid},
		{d(2026, 2, 28), "200", DuePaid}, // február: posledný deň
		{d(2026, 3, 31), "200", DuePartial},
		{d(2026, 4, 30), "200", DueOverdue},
		{d(2026, 5, 31), "200", DueUpcoming},
		{d(2026, 6, 30), "50", DueUpcoming}, // zvyšok
	}
	if len(s) != len(want) {
		t.Fatalf("splátok %d: %+v", len(s), s)
	}
	for i, w := range want {
		if s[i].Date != w.date || !s[i].Amount.Equal(m(w.amount)) {
			t.Errorf("splátka %d: %v %v", i+1, s[i].Date, s[i].Amount)
		}
	}
	// splátka 3 čiastočne zaplatená a po splatnosti → overdue
	if s[2].Status != DueOverdue || !s[2].Paid.Equal(m("50")) || s[0].Status != DuePaid || s[4].Status != DueUpcoming {
		t.Fatalf("stavy: %+v", s)
	}
	st := State(loan, es, today)
	if !st.Remaining.Equal(m("600")) || st.Overdue != 2 || !st.OverdueAmount.Equal(m("350")) || st.NextDue != d(2026, 3, 31) ||
		!st.NextAmount.Equal(m("150")) || st.Left != 4 || st.PaidOff {
		t.Fatalf("stav: %+v", st)
	}
}

func TestScheduleKinds(t *testing.T) {
	today := d(2026, 6, 10)
	cases := []struct {
		name    string
		o       Obligation
		entries []Entry
		dues    int
		paidOff bool
		overdue int
		left    int
	}{
		{"jednorazová zaplatená", Obligation{ID: 1, Kind: Debt, Principal: m("300"), Installment: m("300"), Frequency: Once, FirstDue: d(2026, 3, 1), Count: 1},
			[]Entry{{ObligationID: 1, Amount: m("-300")}}, 1, true, 0, 0},
		{"počet splátok bez sumy", Obligation{ID: 1, Kind: Loan, Installment: m("90"), Frequency: Quarterly, FirstDue: d(2026, 1, 10), Count: 4},
			nil, 4, false, 2, 4},
		{"splatené pred evidenciou", Obligation{ID: 1, Kind: Execution, Principal: m("1000"), PaidBefore: m("900"), Installment: m("100"), Frequency: Monthly, FirstDue: d(2026, 7, 1)},
			nil, 1, false, 0, 1},
		{"požičané — splátky sú príjem", Obligation{ID: 1, Kind: Lent, Principal: m("200"), Installment: m("100"), Frequency: Monthly, FirstDue: d(2026, 5, 1)},
			[]Entry{{ObligationID: 1, Amount: m("100")}, {ObligationID: 1, Amount: m("100")}}, 2, true, 0, 0},
		// pravidelná platba: staré obdobia sa neupomínajú, kalendár do roka vopred
		{"pravidelná bez konca", Obligation{ID: 1, Kind: Regular, Installment: m("520"), Frequency: Monthly, FirstDue: d(2025, 1, 15)},
			[]Entry{{ObligationID: 1, Amount: m("-520")}}, 0, false, 2, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := State(c.o, c.entries, today)
			if st.PaidOff != c.paidOff || st.Overdue != c.overdue || st.Left != c.left {
				t.Fatalf("stav %+v", st)
			}
			if c.dues > 0 {
				if s := Schedule(c.o, Paid(c.o, c.entries), today, today); len(s) != c.dues {
					t.Fatalf("splátok %d", len(s))
				}
			}
		})
	}
}

func TestScheduleMonths(t *testing.T) {
	cases := []struct {
		name   string
		o      Obligation
		months []int // očakávané mesiace splátok
	}{
		{"škola september – jún", Obligation{Kind: Regular, Installment: m("30"), Frequency: Monthly, FirstDue: d(2026, 9, 10), Count: 12, Months: []int{1, 2, 3, 4, 5, 6, 9, 10, 11, 12}},
			[]int{9, 10, 11, 12, 1, 2, 3, 4, 5, 6, 9, 10}},
		{"prvý mesiac mimo – začne v septembri", Obligation{Kind: Regular, Installment: m("30"), Frequency: Monthly, FirstDue: d(2026, 7, 10), Count: 3, Months: []int{9, 10, 11, 12}},
			[]int{9, 10, 11}},
		{"mesiace pri štvrťročnej sa ignorujú", Obligation{Kind: Regular, Installment: m("30"), Frequency: Quarterly, FirstDue: d(2026, 1, 10), Count: 3, Months: []int{9}},
			[]int{1, 4, 7}},
		{"všetkých 12 = každý mesiac", Obligation{Kind: Regular, Installment: m("30"), Frequency: Monthly, FirstDue: d(2026, 1, 10), Count: 2, Months: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}},
			[]int{1, 2}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := c.o
			o.Normalize()
			s := Schedule(o, m("0"), d(2026, 1, 1), d(2030, 1, 1))
			if len(s) != len(c.months) {
				t.Fatalf("%d splátok, chcem %d: %+v", len(s), len(c.months), s)
			}
			for i, mo := range c.months {
				if s[i].Date.Month() != mo || s[i].N != i+1 {
					t.Errorf("splátka %d: %s, chcem mesiac %d", i+1, s[i].Date, mo)
				}
			}
		})
	}
}
