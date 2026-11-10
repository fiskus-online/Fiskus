package delivery

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

func sample() Note {
	m := money.MustParse
	return Note{Customer: invoice.Party{Name: "Agro Liptov družstvo", Country: "SK"}, Date: invoice.MustDate("2026-11-10"), Place: " Sušička obilia, Liptovský Mikuláš ",
		PriceMode: vat.PriceNet, Lines: []invoice.Line{
			{Description: "Kábel CYKY-J 5x2,5", Quantity: m("40"), Unit: "m", UnitPrice: m("2.19"), Rate: vat.Rate23},
			{Description: "Istič B16 3P", Quantity: m("3"), Unit: "ks", UnitPrice: m("18.50"), Rate: vat.Rate23},
		}}
}

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		name string
		mod  func(n *Note)
		want string
	}{
		{"v poriadku", func(n *Note) {}, ""},
		{"bez odberateľa", func(n *Note) { n.Customer.Name = " " }, "customer.name"},
		{"bez dátumu", func(n *Note) { n.Date = invoice.Date{} }, "date"},
		{"bez položiek", func(n *Note) { n.Lines = nil }, "lines"},
		{"bez popisu", func(n *Note) { n.Lines[1].Description = "" }, "lines[1].description"},
		{"záporné množstvo", func(n *Note) { n.Lines[0].Quantity = money.MustParse("-1") }, "lines[0].quantity"},
		{"záporná cena", func(n *Note) { n.Lines[0].UnitPrice = money.MustParse("-1") }, "lines[0].unitPrice"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := sample()
			tc.mod(&n)
			errs := n.Validate()
			got := ""
			if len(errs) > 0 {
				got = errs[0].Field
			}
			if got != tc.want {
				t.Fatalf("chyba %q, chcel som %q (%v)", got, tc.want, errs)
			}
		})
	}
}

func TestRecalculate(t *testing.T) {
	n := sample()
	if err := n.Recalculate(); err != nil {
		t.Fatal(err)
	}
	if n.Status != Draft || n.Place != "Sušička obilia, Liptovský Mikuláš" || n.Lines[0].Amount.String() != "87.60" || n.Summary.Base.String() != "143.10" ||
		n.Summary.Total.String() != "176.01" {
		t.Fatalf("prepočet %+v", n)
	}
	n.NonVATPayer = true
	if err := n.Recalculate(); err != nil {
		t.Fatal(err)
	}
	if n.Summary.Total.String() != "143.10" {
		t.Errorf("neplatiteľ %+v", n.Summary)
	}
	for _, tc := range []struct {
		s                     Status
		editable, invoiceable bool
	}{{Draft, true, false}, {Issued, true, true}, {Invoiced, false, false}, {Cancelled, false, false}} {
		n.Status = tc.s
		if n.Editable() != tc.editable || n.Invoiceable() != tc.invoiceable {
			t.Errorf("%s: úprava %v, fakturácia %v", tc.s, n.Editable(), n.Invoiceable())
		}
	}
}
