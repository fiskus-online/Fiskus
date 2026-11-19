package app

import (
	"strings"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/household"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func TestHouseholdRules(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	h, err := a.CreateCompany(ctx, store.Company{Name: "Domácnosť", Bookkeeping: store.BookkeepingHousehold})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		rules []household.Rule
		field string
	}{
		{"prázdny text", []household.Rule{{Text: " .. ", Category: "Potraviny"}}, "rules.0"},
		{"neznáma kategória", []household.Rule{{Text: "lidl", Category: "Nič"}}, "rules.0"},
		{"dvakrát", []household.Rule{{Text: "Lidl", Category: "Potraviny"}, {Text: "LIDL", Category: "Iné výdavky"}}, "rules.1"},
		{"dlhý text", []household.Rule{{Text: strings.Repeat("a", 61), Category: "Potraviny"}}, "rules.0"},
		{"v poriadku", []household.Rule{{Text: " Lidl ", Category: "Potraviny"}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := a.SaveHouseholdRules(ctx, h.ID, c.rules)
			if c.field == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if !fieldErr(err, c.field) {
				t.Fatalf("chcem chybu %s: %v", c.field, err)
			}
		})
	}
	rules, _ := a.HouseholdRules(ctx, h.ID)
	if len(rules) != 1 || rules[0].Text != "Lidl" {
		t.Fatalf("pravidlá: %+v", rules)
	}
	// uloženie kategórií pravidlá nezmaže
	cats, _ := a.HouseholdCategories(ctx, h.ID)
	if _, err := a.SaveHouseholdCategories(ctx, h.ID, cats); err != nil {
		t.Fatal(err)
	}
	if rules, _ = a.HouseholdRules(ctx, h.ID); len(rules) != 1 {
		t.Fatal("pravidlá zmizli po uložení kategórií")
	}
	// pravidlo má prednosť pred históriou
	accs, _ := a.HouseholdAccounts(ctx, h.ID)
	old, err := a.SaveHouseholdEntry(ctx, h.ID, household.Entry{Date: invoice.MustDate("2026-10-01"), AccountID: accs[0].ID, Amount: m.MustParse("-5"), Category: "Iné výdavky", Payee: "LIDL"})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := a.HouseholdSuggest(ctx, h.ID, "LIDL Banská Bystrica", false); got != "Potraviny" {
		t.Fatalf("návrh %q", got)
	}
	// staršie pohyby
	ch, err := a.HouseholdRuleChanges(ctx, h.ID, false)
	if err != nil || len(ch) != 1 || ch[0].Entry.ID != old.ID {
		t.Fatalf("náhľad %+v %v", ch, err)
	}
	if _, err := a.HouseholdRuleChanges(ctx, h.ID, true); err != nil {
		t.Fatal(err)
	}
	if e, _ := a.Store.GetHHEntry(ctx, h.ID, old.ID); e.Category != "Potraviny" {
		t.Fatalf("kategória %q", e.Category)
	}
	if ch, _ = a.HouseholdRuleChanges(ctx, h.ID, false); len(ch) != 0 {
		t.Fatalf("po použití ostalo %d", len(ch))
	}
}

func TestHouseholdRulesReceipt(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	h, err := a.CreateCompany(ctx, store.Company{Name: "Domácnosť", Bookkeeping: store.BookkeepingHousehold})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveHouseholdRules(ctx, h.ID, []household.Rule{{Text: "pivo", Category: "Iné výdavky"}, {Text: "Potraviny Jednota", Category: "Potraviny"}}); err != nil {
		t.Fatal(err)
	}
	items := []household.ReceiptItem{
		{Name: "Pivo svetlé 0,5l", Amount: m.MustParse("1.20"), Category: "Potraviny"}, // pravidlo prebije návrh AI
		{Name: "Chlieb", Amount: m.MustParse("2.00")},
	}
	d, err := a.receiptDraft(ctx, h.ID, invoice.MustDate("2026-10-01"), "POTRAVINY JEDNOTA", m.MustParse("3.20"), items, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, s := range d.Entry.Split {
		got[s.Category] = s.Amount.String()
	}
	if got["Iné výdavky"] != "-1.20" || got["Potraviny"] != "-2.00" {
		t.Fatalf("položky %+v", d.Entry.Split)
	}
}
