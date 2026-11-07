package forms

import (
	"errors"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
)

func fields(err error) []string {
	var ve *invoice.ValidationError
	if !errors.As(err, &ve) {
		return nil
	}
	var out []string
	for _, f := range ve.Fields {
		out = append(out, f.Field)
	}
	return out
}

func TestCompute(t *testing.T) {
	ppd, _ := Find(Builtin(), "ppd")
	dl, _ := Find(Builtin(), "dodaci-list")
	tests := []struct {
		name             string
		form             Form
		v                Values
		items            []Item
		base, vat, total string
		itemsTotal       string
		errs             []string
	}{
		{"PPD s DPH 23 %", ppd, Values{"date": "2026-11-07", "partner": "Ján Novák", "purpose": "Tržba", "amount": "123", "rate": "23"}, nil, "100.00", "23.00", "123.00", "0.00", nil},
		{"PPD bez DPH", ppd, Values{"date": "2026-11-07", "partner": "Ján Novák", "purpose": "Vklad", "amount": "50,5"}, nil, "50.50", "0.00", "50.50", "0.00", nil},
		{"PPD nevyplnené", ppd, Values{"amount": "x", "rate": "7"}, nil, "0.00", "0.00", "0.00", "0.00", []string{"date", "partner", "purpose", "amount", "rate"}},
		{"dodací list s položkami", dl, Values{"date": "2026-11-07", "issuer": "Firma", "partner": "Odberateľ"},
			[]Item{{Desc: "Kábel CYKY", Qty: m.MustParse("2.5"), Unit: "m", Price: m.MustParse("1.99")}, {}, {Desc: "Istič", Qty: m.MustParse("1"), Price: m.MustParse("8")}},
			"0.00", "0.00", "0.00", "12.98", nil},
		{"položka bez popisu", dl, Values{"date": "2026-11-07", "issuer": "F", "partner": "O"}, []Item{{Qty: m.MustParse("1")}}, "0.00", "0.00", "0.00", "0.00", []string{"items[0].desc"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := Compute(tt.form, tt.v, tt.items)
			if got := fields(err); len(got) != len(tt.errs) || (len(got) > 0 && got[0] != tt.errs[0]) {
				t.Fatalf("chyby %v, chcem %v", got, tt.errs)
			}
			if c.Base.String() != tt.base || c.VAT.String() != tt.vat || c.Total.String() != tt.total || c.ItemsTotal.String() != tt.itemsTotal {
				t.Errorf("%+v", c)
			}
			if len(c.Lines) != len(tt.items) {
				t.Errorf("riadky %v", c.Lines)
			}
		})
	}
}

func TestCustomForm(t *testing.T) {
	tests := []struct {
		name string
		form Form
		keys []string
		errs []string
	}{
		{"kľúče z názvov", Form{Key: "vlastne", Name: " Preberací protokol ", Layout: LayoutCash, Builtin: true, Cash: "in",
			Fields: []Field{{Label: "Číslo zákazky"}, {Label: "Číslo zákazky"}, {Label: "!!!", Type: Date}}}, []string{"cislo-zakazky", "cislo-zakazky-2", "pole"}, nil},
		{"zachová platné kľúče", Form{Key: "x", Name: "X", Fields: []Field{{Key: "a", Label: "A"}, {Key: "a", Label: "B"}}}, []string{"a", "b"}, nil},
		{"chyby", Form{Key: "Zlý kľúč", Fields: []Field{{Label: "", Type: "obrazok"}}}, nil, []string{"name", "key", "fields[0].label", "fields[0].type"}},
		{"prázdne", Form{Key: "x", Name: "X"}, nil, []string{"fields"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.form
			f.Normalize()
			if f.Builtin || f.Cash != "" || f.Layout != LayoutDocument || f.Paper != "A4" {
				t.Fatalf("vlastné tlačivo: %+v", f)
			}
			err := f.Validate()
			if got := fields(err); len(got) != len(tt.errs) {
				t.Fatalf("chyby %v, chcem %v", got, tt.errs)
			}
			for i, k := range tt.keys {
				if f.Fields[i].Key != k {
					t.Errorf("pole %d: %q, chcem %q", i, f.Fields[i].Key, k)
				}
			}
		})
	}
}
