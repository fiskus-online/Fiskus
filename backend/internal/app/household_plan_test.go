package app

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/household"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	m "github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func TestHouseholdPlan(t *testing.T) {
	a, _ := newApp(t, edition.Desktop) // dnes 29. 10. 2026
	h, _ := a.CreateCompany(ctx, store.Company{Name: "Domácnosť", Bookkeeping: store.BookkeepingHousehold})
	save := func(o household.Obligation) household.Obligation {
		t.Helper()
		o, err := a.SaveHouseholdObligation(ctx, h.ID, o)
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	// obedy v škole september – jún, 10. v mesiaci
	school := save(household.Obligation{Kind: household.Regular, Name: "Obedy – Janko", Creditor: "ZŠ Moyzesova", Installment: m.MustParse("32"),
		FirstDue: invoice.MustDate("2026-09-10"), Months: []int{9, 10, 11, 12, 1, 2, 3, 4, 5, 6}, IBAN: "SK31 1200 0000 1987 4263 7541", VS: "1234",
		Address: "Moyzesova 5, 97401 Banská Bystrica", RemindDays: 5})
	if school.Category != "Bývanie" || len(school.Months) != 10 || school.IBAN != "SK3112000000198742637541" {
		t.Fatalf("škola: %+v", school)
	}
	// pokuta jednorazovo v novembri
	save(household.Obligation{Kind: household.Fine, Name: "Pokuta za parkovanie", Creditor: "Mesto BB", Principal: m.MustParse("50"),
		Installment: m.MustParse("50"), Frequency: household.Once, FirstDue: invoice.MustDate("2026-11-20")})
	// požičané — vráti v decembri
	save(household.Obligation{Kind: household.Lent, Name: "Mišo", Principal: m.MustParse("100"), Installment: m.MustParse("100"),
		Frequency: household.Once, FirstDue: invoice.MustDate("2026-12-01")})

	cases := []struct {
		name                string
		year, month, months int
		overdue             string // po splatnosti
		pay                 []string
		receive             []string
	}{
		{"od novembra na 4 mesiace", 2026, 11, 4, "64", []string{"82", "32", "32", "32"}, []string{"0", "100", "0", "0"}},
		{"leto bez obedov", 2027, 6, 3, "64", []string{"32", "0", "0"}, []string{"0", "0", "0"}},
		{"zlý počet mesiacov = 12", 2026, 11, 99, "64", nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := a.HouseholdPlan(ctx, h.ID, c.year, c.month, c.months)
			if err != nil {
				t.Fatal(err)
			}
			if !v.OverduePay.Equal(m.MustParse(c.overdue)) || len(v.Overdue) != 2 {
				t.Fatalf("po splatnosti %v %+v", v.OverduePay, v.Overdue)
			}
			if c.pay == nil {
				if len(v.Months) != 12 {
					t.Fatalf("mesiacov %d", len(v.Months))
				}
				return
			}
			for i, want := range c.pay {
				if !v.Months[i].Pay.Equal(m.MustParse(want)) || !v.Months[i].Receive.Equal(m.MustParse(c.receive[i])) {
					t.Errorf("%s: platiť %v prijať %v, chcem %s / %s", v.Months[i].Month, v.Months[i].Pay, v.Months[i].Receive, want, c.receive[i])
				}
			}
		})
	}
	v, _ := a.HouseholdPlan(ctx, h.ID, 2026, 11, 1)
	if it := v.Months[0].Items[0]; it.Name != "Obedy – Janko" || it.IBAN == "" || it.VS != "1234" || it.Address == "" || it.Rest.String() != "32.00" {
		t.Fatalf("položka plánu: %+v", it)
	}
	if !v.Pay.Equal(m.MustParse("82")) {
		t.Fatalf("spolu %v", v.Pay)
	}
	if _, err := a.HouseholdPlan(ctx, h.ID, 2026, 13, 1); !fieldErr(err, "from") {
		t.Fatalf("zlý mesiac: %v", err)
	}
	// kalendár: vlastná pripomienka
	cal, _ := a.Calendar(ctx, h.ID, invoice.MustDate("2026-11-01"), invoice.MustDate("2026-11-30"))
	found := false
	for _, e := range cal.Events {
		if e.Title == "Platba: Obedy – Janko" && e.Date == invoice.MustDate("2026-11-10") {
			found = e.RemindDays == 5 && e.Detail == "ZŠ Moyzesova"
		}
	}
	if !found {
		t.Fatalf("termín v kalendári: %+v", cal.Events)
	}
}

func TestHouseholdRemindDays(t *testing.T) {
	a, _ := newApp(t, edition.Desktop) // dnes 29. 10. 2026
	h, _ := a.CreateCompany(ctx, store.Company{Name: "Domácnosť", Bookkeeping: store.BookkeepingHousehold})
	for _, o := range []household.Obligation{
		{Kind: household.Fine, Name: "Krúžok", Installment: m.MustParse("40"), Frequency: household.Once, FirstDue: invoice.MustDate("2026-11-05"), RemindDays: 10},
		{Kind: household.Fine, Name: "Pokuta", Installment: m.MustParse("30"), Frequency: household.Once, FirstDue: invoice.MustDate("2026-11-05")},
		{Kind: household.Fine, Name: "Výlet", Installment: m.MustParse("20"), Frequency: household.Once, FirstDue: invoice.MustDate("2026-11-20"), RemindDays: 10},
	} {
		if _, err := a.SaveHouseholdObligation(ctx, h.ID, o); err != nil {
			t.Fatal(err)
		}
	}
	es, err := a.dueEvents(ctx, h.ID, 3, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	// krúžok o 7 dní s pripomienkou 10 dní áno, pokuta o 7 dní (predvolene 3) nie, výlet o 22 dní nie
	if len(es) != 1 || es[0].Title != "Platba: Krúžok" {
		t.Fatalf("upozornenia: %+v", es)
	}
}
