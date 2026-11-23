package ledger

import (
	"strings"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/money"
)

func entriesStr(es []Entry) string {
	var b []string
	for _, e := range es {
		var ls []string
		for _, l := range e.Lines {
			if !l.Debit.IsZero() {
				ls = append(ls, l.Account+" MD "+l.Debit.String())
			} else {
				ls = append(ls, l.Account+" D "+l.Credit.String())
			}
		}
		b = append(b, strings.Join(ls, ", "))
	}
	return strings.Join(b, " | ")
}

func TestClosingEntries(t *testing.T) {
	tb := func(pairs ...string) []Turnover {
		var out []Turnover
		for i := 0; i < len(pairs); i += 2 {
			out = append(out, Turnover{Account: pairs[i], Closing: money.MustParse(pairs[i+1])})
		}
		return out
	}
	tests := []struct {
		name    string
		acc     []Turnover
		result  string
		closing string
		opening string
	}{
		{"zisk 15 000 €", tb("221", "13000", "311", "5000", "321", "-3000", "518", "35000", "602", "-50000"), "15000",
			"710 MD 35000.00, 518 D 35000.00 | 602 MD 50000.00, 710 D 50000.00 | 710 MD 15000.00, 702 D 15000.00 | 221 D 13000.00, 311 D 5000.00, 321 MD 3000.00, 702 MD 15000.00",
			"221 MD 13000.00, 311 MD 5000.00, 321 D 3000.00, 431 D 15000.00, 701 D 18000.00, 701 MD 18000.00"},
		{"strata 2 000 €", tb("221", "1000", "411", "-5000", "429", "2000", "501", "4000", "602", "-2000"), "-2000",
			"710 MD 4000.00, 501 D 4000.00 | 602 MD 2000.00, 710 D 2000.00 | 702 MD 2000.00, 710 D 2000.00 | 221 D 1000.00, 411 MD 5000.00, 429 D 2000.00, 702 D 2000.00",
			"221 MD 1000.00, 411 D 5000.00, 429 MD 2000.00, 431 MD 2000.00, 701 D 5000.00, 701 MD 5000.00"},
		{"bez pohybov", nil, "0", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es, res := ClosingEntries(tt.acc)
			if !res.Equal(money.MustParse(tt.result)) || entriesStr(es) != tt.closing {
				t.Fatalf("výsledok %s\n%s", res, entriesStr(es))
			}
			// po uzávierkových zápisoch majú všetky účty nulový zostatok
			bal := map[string]money.Dec{}
			for _, a := range tt.acc {
				bal[a.Account] = a.Closing
			}
			for _, e := range es {
				if !Balanced(e.Lines) {
					t.Fatalf("nevyrovnaný zápis %q", e.Text)
				}
				for _, l := range e.Lines {
					bal[l.Account] = bal[l.Account].Add(l.Debit).Sub(l.Credit)
				}
			}
			for acc, v := range bal {
				if !v.IsZero() {
					t.Errorf("%s má po uzavretí zostatok %s", acc, v)
				}
			}
			op := OpeningEntries(tt.acc)
			if entriesStr(op) != tt.opening || (len(op) > 0 && !Balanced(op[0].Lines)) {
				t.Fatalf("otvorenie: %s", entriesStr(op))
			}
		})
	}
}
