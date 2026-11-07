package calc

import (
	"errors"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/reminder"
)

func field(err error) string {
	var fe *FieldError
	if errors.As(err, &fe) {
		return fe.Field
	}
	if err != nil {
		return "?" + err.Error()
	}
	return ""
}

func TestVAT(t *testing.T) {
	tests := []struct {
		in               VATInput
		base, vat, total string
		err              string
	}{
		{VATInput{m.MustParse("100"), 23, false}, "100.00", "23.00", "123.00", ""},
		{VATInput{m.MustParse("123"), 23, true}, "100.00", "23.00", "123.00", ""},
		{VATInput{m.MustParse("10"), 19, true}, "8.40", "1.60", "10.00", ""},
		{VATInput{m.MustParse("99.99"), 5, false}, "99.99", "5.00", "104.99", ""},
		{VATInput{m.MustParse("1"), 21, false}, "", "", "", "rate"},
	}
	for _, tt := range tests {
		r, err := VAT(tt.in)
		if field(err) != tt.err {
			t.Fatalf("%+v: chyba %v", tt.in, err)
		}
		if err == nil && (r.Base.String() != tt.base || r.VAT.String() != tt.vat || r.Total.String() != tt.total) {
			t.Errorf("%+v: %+v", tt.in, r)
		}
	}
}

func TestLateInterest(t *testing.T) {
	law := reminder.DefaultLaw()
	d := invoice.MustDate
	tests := []struct {
		name                  string
		in                    LateInput
		days                  int
		rate, interest, total string
		err                   string
	}{
		{"podnikatelia", LateInput{m.MustParse("1000"), d("2026-09-30"), d("2026-10-30"), DebtorBusiness, m.Zero}, 30, "11.65", "9.58", "1049.58", ""},
		{"spotrebiteľ", LateInput{m.MustParse("1000"), d("2026-09-30"), d("2026-10-30"), DebtorConsumer, m.Zero}, 30, "7.65", "6.29", "1006.29", ""},
		{"sadzba k prvému dňu omeškania", LateInput{m.MustParse("1000"), d("2026-09-15"), d("2026-09-16"), DebtorConsumer, m.Zero}, 1, "7.65", "0.21", "1000.21", ""},
		{"dohodnutá cez prelom roka", LateInput{m.MustParse("36500"), d("2025-12-30"), d("2026-01-02"), DebtorCustom, m.MustParse("10")}, 3, "10.00", "30.00", "36530.00", ""},
		{"v lehote", LateInput{m.MustParse("1"), d("2026-09-30"), d("2026-09-30"), DebtorBusiness, m.Zero}, 0, "", "", "", "paid"},
		{"bez sadzby", LateInput{m.MustParse("1"), d("2026-09-30"), d("2026-10-30"), DebtorCustom, m.Zero}, 0, "", "", "", "rate"},
		{"bez sumy", LateInput{m.Zero, d("2026-09-30"), d("2026-10-30"), DebtorBusiness, m.Zero}, 0, "", "", "", "amount"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := LateInterest(tt.in, law)
			if field(err) != tt.err {
				t.Fatalf("chyba %v", err)
			}
			if err == nil && (r.Days != tt.days || r.Rate.String() != tt.rate || r.Interest.String() != tt.interest || r.Total.String() != tt.total) {
				t.Errorf("%+v", r)
			}
		})
	}
}

func TestLoan(t *testing.T) {
	tests := []struct {
		name                          string
		in                            LoanInput
		payment, interest, total, apr string
		err                           string
	}{
		{"6 % na rok", LoanInput{Amount: m.MustParse("10000"), Rate: m.MustParse("6"), Months: 12}, "860.66", "327.96", "10327.96", "6.17", ""},
		{"bez úroku", LoanInput{Amount: m.MustParse("1000"), Months: 3}, "333.33", "0.00", "1000.00", "0.00", ""},
		{"s poplatkami", LoanInput{Amount: m.MustParse("10000"), Rate: m.MustParse("6"), Months: 12, Fee: m.MustParse("100"), MonthlyFee: m.MustParse("2")},
			"860.66", "327.96", "10451.96", "8.65", ""},
		{"bez sumy", LoanInput{Months: 12}, "", "", "", "", "amount"},
		{"bez splátok", LoanInput{Amount: m.MustParse("1"), Months: 0}, "", "", "", "", "months"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := Loan(tt.in)
			if field(err) != tt.err {
				t.Fatalf("chyba %v", err)
			}
			if err != nil {
				return
			}
			if r.Payment.String() != tt.payment || r.Interest.String() != tt.interest || r.Total.String() != tt.total || r.APR.String() != tt.apr {
				t.Errorf("splátka %s úroky %s spolu %s RPMN %s", r.Payment, r.Interest, r.Total, r.APR)
			}
			last := r.Schedule[len(r.Schedule)-1]
			if len(r.Schedule) != tt.in.Months || !last.Balance.IsZero() {
				t.Errorf("kalendár %d, zostatok %s", len(r.Schedule), last.Balance)
			}
		})
	}
}

func TestSavings(t *testing.T) {
	tests := []struct {
		name                         string
		in                           SavingsInput
		deposits, interest, tax, bal string
		years                        int
		err                          string
	}{
		{"jednorazový vklad", SavingsInput{Initial: m.MustParse("1000"), Rate: m.MustParse("12"), Months: 12}, "1000.00", "126.84", "0.00", "1126.84", 1, ""},
		{"mesačne s daňou", SavingsInput{Initial: m.MustParse("1000"), Monthly: m.MustParse("100"), Rate: m.MustParse("3"), Months: 24, Tax: m.MustParse("19")}, "3400.00", "137.61", "26.11", "3511.50", 2, ""},
		{"bez vkladu", SavingsInput{Rate: m.MustParse("3"), Months: 12}, "", "", "", "", 0, "monthly"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := Savings(tt.in)
			if field(err) != tt.err {
				t.Fatalf("chyba %v", err)
			}
			if err == nil && (r.Deposits.String() != tt.deposits || r.Interest.String() != tt.interest || r.Tax.String() != tt.tax || r.Balance.String() != tt.bal || len(r.Years) != tt.years) {
				t.Errorf("%+v", r)
			}
		})
	}
}
