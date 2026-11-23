package store

import "testing"

func TestSettingOfCompany(t *testing.T) {
	tests := []struct {
		key  string
		id   int64
		want bool
	}{
		{"stock.5", 5, true},
		{"household.assets.5", 5, true},
		{"ledger.5.2026", 5, true},
		{"ledger.form.5.2026", 5, true},
		{"vat.filed.5.2026-10", 5, true},
		{"payroll.month.5.2026-10", 5, true},
		{"dpfo.5.2026", 5, true},
		{"ledger.3.2026", 2026, false}, // rok inej firmy
		{"ledger.form.3.2026", 2026, false},
		{"stock.15", 5, false},
		{"ledger.15.2026", 5, false},
		{"legal.vat", 5, false},
		{"5", 5, false},
	}
	for _, tt := range tests {
		if got := SettingOfCompany(tt.key, tt.id); got != tt.want {
			t.Errorf("%s (%d): %v", tt.key, tt.id, got)
		}
	}
}
