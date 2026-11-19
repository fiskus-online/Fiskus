package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/household"
)

// maxRules je najviac pravidiel kategórií v domácnosti.
const maxRules = 500

// HouseholdRules vráti pravidlá kategórií.
func (a *App) HouseholdRules(ctx context.Context, companyID int64) ([]household.Rule, error) {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerRead); err != nil {
		return nil, err
	}
	s, err := a.householdSettings(ctx, companyID)
	if s.Rules == nil {
		s.Rules = []household.Rule{}
	}
	return s.Rules, err
}

// SaveHouseholdRules uloží pravidlá kategórií (poradie = priorita).
func (a *App) SaveHouseholdRules(ctx context.Context, companyID int64, rules []household.Rule) ([]household.Rule, error) {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return nil, err
	}
	if len(rules) > maxRules {
		return nil, vErr("rules", fmt.Sprintf("Pravidiel môže byť najviac %d", maxRules))
	}
	s, err := a.householdSettings(ctx, companyID)
	if err != nil {
		return nil, err
	}
	cats := map[string]bool{}
	for _, c := range s.Categories {
		cats[c.Name] = true
	}
	seen := map[string]bool{}
	for i := range rules {
		r := &rules[i]
		r.Text = strings.TrimSpace(r.Text)
		f := fmt.Sprintf("rules.%d", i)
		k := household.RuleKey(r.Text)
		switch {
		case k == "":
			return nil, vErr(f, "Zadajte text, podľa ktorého sa pohyb spozná (napr. LIDL)")
		case len([]rune(r.Text)) > 60:
			return nil, vErr(f, "Text pravidla môže mať najviac 60 znakov")
		case !cats[r.Category]:
			return nil, vErr(f, "Kategória „"+r.Category+"“ nie je v zozname")
		case seen[k]:
			return nil, vErr(f, "Pravidlo pre „"+r.Text+"“ je dvakrát")
		}
		seen[k] = true
	}
	before := s.Rules
	s.Rules = rules
	data, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	if err := a.Store.SetSetting(ctx, householdKey(companyID), string(data)); err != nil {
		return nil, err
	}
	a.record(ctx, companyID, audit.CatLedger, "update", "household_rules", 0, "Domácnosť: pravidlá kategórií", before, rules)
	return rules, nil
}

// HouseholdRuleChanges vráti pohyby, ktorým by pravidlá zmenili kategóriu.
// S apply ich aj zmení (každý ako bežná úprava pohybu, s auditom).
func (a *App) HouseholdRuleChanges(ctx context.Context, companyID int64, apply bool) ([]household.RuleChange, error) {
	perm := auth.PermLedgerRead
	if apply {
		perm = auth.PermLedgerWrite
	}
	if err := a.requireHousehold(ctx, companyID, perm); err != nil {
		return nil, err
	}
	s, err := a.householdSettings(ctx, companyID)
	if err != nil {
		return nil, err
	}
	es, err := a.Store.ListHHEntries(ctx, companyID, 0)
	if err != nil {
		return nil, err
	}
	ch := household.RuleChanges(es, s.Rules, s.Categories)
	if !apply {
		return ch, nil
	}
	for _, c := range ch {
		e := c.Entry
		e.Category = c.To
		if _, err := a.SaveHouseholdEntry(ctx, companyID, e); err != nil {
			return nil, err
		}
	}
	return ch, nil
}
