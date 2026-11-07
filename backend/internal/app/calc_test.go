package app

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/fx"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/payroll"
)

func TestCalcWage(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	tests := []struct {
		name  string
		in    WageInput
		check func(r payroll.Result) bool
		err   string
	}{
		{"pracovný pomer", WageInput{Year: 2026, Month: 3, Gross: m.MustParse("1500"), NCZD: true},
			func(r payroll.Result) bool {
				return r.Gross.String() == "1500.00" && r.Net.Sign() > 0 && r.Net.Cmp(r.Gross) < 0 && r.Cost.Cmp(r.Gross) > 0 && r.TaxBonus.IsZero()
			}, ""},
		{"s deťmi bonus", WageInput{Year: 2026, Month: 3, Gross: m.MustParse("1500"), NCZD: true, Children: []int{4, 16}},
			func(r payroll.Result) bool { return r.TaxBonus.Sign() > 0 }, ""},
		{"zlý rok", WageInput{Year: 2019, Gross: m.MustParse("1000")}, nil, "year"},
		{"bez mzdy", WageInput{Year: 2026}, nil, "gross"},
		{"vek dieťaťa", WageInput{Year: 2026, Gross: m.MustParse("1000"), Children: []int{30}}, nil, "children[0]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := a.CalcWage(tt.in)
			if tt.err != "" {
				if !fieldErr(err, tt.err) {
					t.Fatalf("chyba %v", err)
				}
				return
			}
			if err != nil || !tt.check(r) {
				t.Fatalf("%+v %v", r, err)
			}
		})
	}
}

func TestCalcFX(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	if err := a.Store.SaveRates(ctx, []fx.Rate{{Currency: "CZK", Date: "2026-11-02", Rate: m.MustParse("24.5"), Source: fx.SourceECB},
		{Currency: "CZK", Date: "2026-11-04", Rate: m.MustParse("25"), Source: fx.SourceECB}}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		in             FXInput
		rate, res, day string
		err            string
	}{
		{FXInput{m.MustParse("100"), "CZK", invoice.MustDate("2026-11-04"), false}, "25.00", "2500.00", "2026-11-04", ""},
		{FXInput{m.MustParse("100"), "czk", invoice.MustDate("2026-11-03"), true}, "24.50", "4.08", "2026-11-02", ""},
		{FXInput{m.MustParse("100"), "CZK", invoice.MustDate("2026-10-01"), false}, "", "", "", "date"},
		{FXInput{m.MustParse("100"), "XXX", invoice.MustDate("2026-11-04"), false}, "", "", "", "currency"},
	}
	for _, tt := range tests {
		r, err := a.CalcFX(ctx, tt.in)
		if tt.err != "" {
			if !fieldErr(err, tt.err) {
				t.Fatalf("%+v: chyba %v", tt.in, err)
			}
			continue
		}
		if err != nil || r.Rate.String() != tt.rate || r.Result.String() != tt.res || r.RateDate != tt.day {
			t.Errorf("%+v: %+v %v", tt.in, r, err)
		}
	}
}
