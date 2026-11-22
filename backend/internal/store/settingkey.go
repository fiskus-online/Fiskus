package store

import (
	"strconv"
	"strings"
)

// SettingOfCompany vráti true, ak kľúč nastavení patrí firme id: „x.<id>“
// (napr. stock.5, household.assets.5) alebo „x.<id>.<rok|obdobie>“ (napr.
// ledger.5.2026, vat.filed.5.2026-10). Kľúč inej firmy, ktorého rok sa
// zhoduje s id („ledger.3.2026“ pri id 2026), firme nepatrí.
func SettingOfCompany(key string, id int64) bool {
	cid := strconv.FormatInt(id, 10)
	p := strings.Split(key, ".")
	n := len(p)
	numeric := func(s string) bool {
		if s == "" {
			return false
		}
		for _, r := range s {
			if r < '0' || r > '9' {
				return false
			}
		}
		return true
	}
	switch {
	case n >= 2 && p[n-1] == cid && !numeric(p[n-2]):
		return true
	case n >= 3 && p[n-2] == cid && !numeric(p[n-3]):
		return true
	}
	return false
}
