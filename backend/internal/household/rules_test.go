package household

import "testing"

func TestMatchRule(t *testing.T) {
	cats := []Category{{Name: "Mzda", Income: true}, {Name: "Potraviny"}, {Name: "Neduhy"}, {Name: "Auto"}}
	rules := []Rule{
		{Text: "Tabak", Category: "Neduhy"},
		{Text: "LIDL", Category: "Potraviny"},
		{Text: "Slovnaft", Category: "Auto"},
		{Text: "výplata", Category: "Mzda"},
		{Text: "zmazaná", Category: "Nič"},
	}
	cases := []struct {
		text   string
		income bool
		want   string
	}{
		{"LIDL DAKUJEME ZA NAKUP", false, "Potraviny"},
		{"Lidl Banská Bystrica", false, "Potraviny"},
		{"platba kartou lidl", false, "Potraviny"},
		{"KALIDL", false, ""},                     // iba od začiatku slova
		{"Tabakova sklada LIDL", false, "Neduhy"}, // prvé pravidlo vyhráva
		{"SLOVNAFT a.s.", false, "Auto"},
		{"Výplata 10/2026", true, "Mzda"},
		{"Výplata 10/2026", false, ""}, // kategória iného druhu
		{"Slovnaft", true, ""},
		{"zmazaná vec", false, ""}, // kategória neexistuje
		{"", false, ""},
	}
	for _, c := range cases {
		if got := MatchRule(c.text, c.income, rules, cats); got != c.want {
			t.Errorf("%q income=%v: got %q, want %q", c.text, c.income, got, c.want)
		}
	}
}

func TestRuleChanges(t *testing.T) {
	cats := []Category{{Name: "Potraviny"}, {Name: "Iné výdavky"}}
	rules := []Rule{{Text: "lidl", Category: "Potraviny"}}
	es := []Entry{
		{ID: 1, Payee: "LIDL", Amount: m("-5"), Category: "Iné výdavky"},
		{ID: 2, Payee: "LIDL", Amount: m("-5"), Category: "Potraviny"},
		{ID: 3, Note: "nákup lidl", Amount: m("-5"), Category: "Iné výdavky"},
		{ID: 4, Payee: "LIDL", AccountID: 1, ToAccountID: 2, Amount: m("5")},
		{ID: 5, Payee: "LIDL", Amount: m("-5"), Category: "Iné výdavky", Split: []Split{{Category: "Iné výdavky", Amount: m("-5")}}},
		{ID: 6, Payee: "Kaufland", Amount: m("-5"), Category: "Iné výdavky"},
	}
	got := RuleChanges(es, rules, cats)
	if len(got) != 2 || got[0].Entry.ID != 1 || got[1].Entry.ID != 3 || got[0].To != "Potraviny" {
		t.Fatalf("%+v", got)
	}
}
