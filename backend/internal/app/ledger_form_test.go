package app

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/ledger"
)

func TestStatementForm(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	tests := []struct {
		name string
		form ledger.Form
		err  bool
		want ledger.Form
		rows int // riadky strany aktív
	}{
		{"predvolený mikro", "", false, ledger.FormMicro, 23},
		{"malá účtovná jednotka", ledger.FormSmall, false, ledger.FormSmall, 78},
		{"neznámy vzor", "velka", true, ledger.FormSmall, 78},
		{"späť na mikro", ledger.FormMicro, false, ledger.FormMicro, 23},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.form != "" {
				err := a.SetStatementForm(ctx, c.ID, 2026, tt.form)
				if (err != nil) != tt.err || (tt.err && !fieldErr(err, "form")) {
					t.Fatalf("chyba %v", err)
				}
			}
			st, err := a.LedgerStatementsFor(ctx, c.ID, 2026)
			if err != nil || st.Form != tt.want || len(st.Assets) != tt.rows {
				t.Fatalf("%s %d %v", st.Form, len(st.Assets), err)
			}
		})
	}
	// vzor je pre každý rok zvlášť
	if st, _ := a.LedgerStatementsFor(ctx, c.ID, 2025); st.Form != ledger.FormMicro {
		t.Fatalf("2025: %s", st.Form)
	}
}
