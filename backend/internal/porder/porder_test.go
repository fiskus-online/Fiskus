package porder

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
)

var m = money.MustParse

func sample() Order {
	return Order{Supplier: " Elektro veľkoobchod BB ", Date: invoice.MustDate("2026-11-10"), Lines: []Line{
		{ItemID: 13, Name: "Kábel CYKY-J 5x2,5", Unit: "m", Quantity: m("100"), UnitPrice: m("1.35")},
		{Name: "Istič B16 3P", Quantity: m("3"), UnitPrice: m("11.20")},
	}}
}

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		name string
		mod  func(o *Order)
		want string
	}{
		{"v poriadku", func(o *Order) {}, ""},
		{"bez dodávateľa", func(o *Order) { o.Supplier = " " }, "supplier"},
		{"bez dátumu", func(o *Order) { o.Date = invoice.Date{} }, "date"},
		{"dodanie pred objednávkou", func(o *Order) { o.Expected = invoice.MustDate("2026-11-01") }, "expected"},
		{"bez riadkov", func(o *Order) { o.Lines = nil }, "lines"},
		{"riadok bez názvu", func(o *Order) { o.Lines[1].Name = "" }, "lines[1].name"},
		{"nulové množstvo", func(o *Order) { o.Lines[0].Quantity = money.Zero }, "lines[0].quantity"},
		{"menej ako prijaté", func(o *Order) { o.Lines[0].Received = m("120") }, "lines[0].quantity"},
		{"záporná cena", func(o *Order) { o.Lines[1].UnitPrice = m("-1") }, "lines[1].unitPrice"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := sample()
			tc.mod(&o)
			o.Normalize()
			errs := o.Validate()
			got := ""
			if len(errs) > 0 {
				got = errs[0].Field
			}
			if got != tc.want {
				t.Fatalf("chyba %q, chcel som %q (%v)", got, tc.want, errs)
			}
		})
	}
	o := sample()
	o.Normalize()
	if o.Supplier != "Elektro veľkoobchod BB" || o.Status != Draft || o.Lines[1].Unit != "ks" || o.Lines[0].Total.String() != "135.00" || o.Total.String() != "168.60" {
		t.Fatalf("normalizácia %+v", o)
	}
}

func TestReceive(t *testing.T) {
	type step struct {
		qty    map[int]money.Dec
		field  string // očakávaná chyba
		status Status
	}
	for _, tc := range []struct {
		name  string
		start Status
		steps []step
	}{
		{"čiastočne a potom celá", Sent, []step{
			{map[int]money.Dec{0: m("60")}, "", Partial},
			{map[int]money.Dec{0: m("40"), 1: m("3")}, "", Received},
			{map[int]money.Dec{0: m("1")}, "status", Received},
		}},
		{"rozpracovaná sa príjmom stane odoslanou", Draft, []step{
			{map[int]money.Dec{1: m("1")}, "", Partial},
		}},
		{"viac ako zostáva", Sent, []step{
			{map[int]money.Dec{1: m("4")}, "lines[1].receive", Sent},
		}},
		{"nič na príjem", Sent, []step{
			{map[int]money.Dec{0: money.Zero}, "lines", Sent},
		}},
		{"záporné množstvo", Sent, []step{
			{map[int]money.Dec{0: m("-1")}, "lines[0].receive", Sent},
		}},
		{"zrušená", Cancelled, []step{
			{map[int]money.Dec{0: m("1")}, "status", Cancelled},
		}},
		{"neznámy riadok", Sent, []step{
			{map[int]money.Dec{5: m("1")}, "lines", Sent},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := sample()
			o.Normalize()
			o.Status = tc.start
			for i, s := range tc.steps {
				cp := o
				cp.Lines = append([]Line(nil), o.Lines...)
				errs := cp.Receive(s.qty)
				got := ""
				if len(errs) > 0 {
					got = errs[0].Field
				} else {
					o = cp
				}
				if got != s.field || o.Status != s.status {
					t.Fatalf("krok %d: chyba %q (chcel %q), stav %s (chcel %s)", i, got, s.field, o.Status, s.status)
				}
			}
		})
	}
	o := sample()
	o.Status = Partial
	o.Lines[0].Received = m("10")
	o.Lines[0].Received = money.Zero
	o.UpdateStatus()
	if o.Status != Sent {
		t.Errorf("bez príjmu späť na odoslanú: %s", o.Status)
	}
	if r := (Line{Quantity: m("5"), Received: m("7")}).Remaining(); !r.IsZero() {
		t.Errorf("zostatok %s", r)
	}
}
