package household

import "strings"

// Rule je pravidlo kategórie: keď text obchodu, popisu alebo položky
// bločku obsahuje Text (bez diakritiky, od začiatku slova), pohyb dostane
// kategóriu Category. Pravidlá majú prednosť pred odhadom z histórie.
type Rule struct {
	Text     string `json:"text"`
	Category string `json:"category"`
}

// RuleKey je text pravidla na porovnanie (bez diakritiky, malé písmená).
func RuleKey(text string) string { return fold(text) }

// MatchRule vráti kategóriu prvého pravidla, ktoré na text sedí a ktorého
// kategória je toho istého druhu (príjem/výdavok); "" = žiadne.
func MatchRule(text string, income bool, rules []Rule, cats []Category) string {
	key := fold(text)
	if key == "" {
		return ""
	}
	padded := " " + key + " "
	for _, r := range rules {
		k := fold(r.Text)
		if k == "" || !strings.Contains(padded, " "+k) {
			continue
		}
		for _, c := range cats {
			if c.Name == r.Category && c.Income == income {
				return c.Name
			}
		}
	}
	return ""
}

// RuleChange je pohyb, ktorému pravidlá zmenia kategóriu.
type RuleChange struct {
	Entry Entry  `json:"entry"`
	To    string `json:"to"`
}

// RuleChanges nájde pohyby, ktorých kategória nezodpovedá pravidlám.
// Prevody a pohyby rozdelené na položky sa nemenia.
func RuleChanges(entries []Entry, rules []Rule, cats []Category) []RuleChange {
	out := []RuleChange{}
	for _, e := range entries {
		if e.Transfer() || len(e.Split) > 0 {
			continue
		}
		to := MatchRule(strings.TrimSpace(e.Payee+" "+e.Note), e.Amount.Sign() > 0, rules, cats)
		if to != "" && to != e.Category {
			out = append(out, RuleChange{Entry: e, To: to})
		}
	}
	return out
}
