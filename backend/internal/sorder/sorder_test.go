package sorder

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

func d(s string) money.Dec { return money.MustParse(s) }

func line(item int64, desc, qty, price string) Line {
	return Line{Line: invoice.Line{ItemID: item, Description: desc, Quantity: d(qty), Unit: "ks", UnitPrice: d(price), Rate: vat.Rate23}}
}

func sample() Order {
	return Order{Customer: invoice.Party{Name: "Agro"}, Date: invoice.MustDate("2026-11-10"),
		Lines: []Line{line(7, "Kábel", "10", "2"), line(0, "Montáž", "1", "50")}}
}

func TestRecalculate(t *testing.T) {
	o := sample()
	if err := o.Recalculate(); err != nil {
		t.Fatal(err)
	}
	if o.Lines[0].Key != 1 || o.Lines[1].Key != 2 {
		t.Fatalf("kľúče: %d %d", o.Lines[0].Key, o.Lines[1].Key)
	}
	if !o.Summary.Base.Equal(d("70")) || !o.Lines[0].Amount.Equal(d("20")) {
		t.Fatalf("sumy: %+v", o.Summary)
	}
	// nový riadok dostane ďalší kľúč, aj keď je vložený na začiatok
	o.Lines = append([]Line{line(0, "Doprava", "1", "10")}, o.Lines...)
	_ = o.Recalculate()
	if o.Lines[0].Key != 3 {
		t.Fatalf("kľúč nového riadku: %d", o.Lines[0].Key)
	}
}

func TestStatusFromFulfils(t *testing.T) {
	tests := []struct {
		name    string
		fulfils []Fulfil
		status  Status
		rest    string
	}{
		{"nič", nil, Open, "10"},
		{"časť", []Fulfil{{Kind: ByDelivery, ID: 1, Qty: map[int]money.Dec{1: d("4")}}}, Partial, "6"},
		{"dve dodania", []Fulfil{{Qty: map[int]money.Dec{1: d("4")}}, {Qty: map[int]money.Dec{1: d("6"), 2: d("1")}}}, Done, "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := sample()
			o.Fulfils = tt.fulfils
			_ = o.Recalculate()
			if o.Status != tt.status || !o.Lines[0].Rest.Equal(d(tt.rest)) {
				t.Fatalf("stav %s zvyšok %s", o.Status, o.Lines[0].Rest)
			}
		})
	}
	o := sample()
	o.Status = Cancelled
	o.Fulfils = []Fulfil{{Qty: map[int]money.Dec{1: d("4")}}}
	_ = o.Recalculate()
	if o.Status != Cancelled || o.Editable() {
		t.Fatal("zrušená ostane zrušená")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name  string
		edit  func(*Order)
		field string
	}{
		{"v poriadku", func(*Order) {}, ""},
		{"bez odberateľa", func(o *Order) { o.Customer.Name = " " }, "customer.name"},
		{"dodanie pred dátumom", func(o *Order) { o.Due = invoice.MustDate("2026-11-01") }, "due"},
		{"bez položiek", func(o *Order) { o.Lines = nil }, "lines"},
		{"pod dodané", func(o *Order) { o.Lines[0].Quantity = d("3") }, "lines[0].quantity"},
		{"odstránená dodaná", func(o *Order) { o.Lines = o.Lines[1:] }, "lines"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := sample()
			_ = o.Recalculate()
			o.Fulfils = []Fulfil{{Number: "DL20260001", Qty: map[int]money.Dec{1: d("4")}}}
			tt.edit(&o)
			_ = o.Recalculate()
			errs := o.Validate()
			if tt.field == "" {
				if len(errs) != 0 {
					t.Fatalf("chyby: %+v", errs)
				}
				return
			}
			if len(errs) == 0 || errs[0].Field != tt.field {
				t.Fatalf("čakal som %s, mám %+v", tt.field, errs)
			}
		})
	}
}

func TestPick(t *testing.T) {
	o := sample()
	o.Fulfils = []Fulfil{{Qty: map[int]money.Dec{1: d("4")}}}
	_ = o.Recalculate()
	tests := []struct {
		name  string
		qty   map[int]money.Dec
		lines int
		first string
		err   string
	}{
		{"celý zvyšok", nil, 2, "6", ""},
		{"časť", map[int]money.Dec{1: d("2"), 2: d("0")}, 1, "2", ""},
		{"viac ako zvyšok", map[int]money.Dec{1: d("7")}, 0, "", "lines[0].pick"},
		{"nič", map[int]money.Dec{1: d("0")}, 0, "", "lines"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines, used, errs := o.Pick(tt.qty)
			if tt.err != "" {
				if len(errs) == 0 || errs[0].Field != tt.err {
					t.Fatalf("chyba: %+v", errs)
				}
				return
			}
			if len(errs) > 0 || len(lines) != tt.lines || !lines[0].Quantity.Equal(d(tt.first)) || !used[1].Equal(d(tt.first)) {
				t.Fatalf("riadky %+v použité %+v chyby %+v", lines, used, errs)
			}
		})
	}
	o.Status = Done
	if _, _, errs := o.Pick(nil); len(errs) == 0 {
		t.Fatal("vybavená objednávka sa nedodáva")
	}
}

func TestReserved(t *testing.T) {
	a := sample()
	a.ID = 1
	a.Fulfils = []Fulfil{{Qty: map[int]money.Dec{1: d("4")}}}
	_ = a.Recalculate()
	b := sample()
	b.ID = 2
	_ = b.Recalculate()
	c := sample()
	c.ID, c.Status = 3, Cancelled
	_ = c.Recalculate()
	r := Reserved([]Order{a, b, c}, 0)
	if len(r) != 1 || !r[7].Equal(d("16")) {
		t.Fatalf("rezervácie: %+v", r)
	}
	if r := Reserved([]Order{a, b, c}, 2); !r[7].Equal(d("6")) {
		t.Fatalf("bez objednávky 2: %+v", r)
	}
}
