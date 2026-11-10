package quote

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

func sample() Quote {
	return Quote{
		Customer:   invoice.Party{Name: "Odberateľ s.r.o.", Country: "SK"},
		IssueDate:  invoice.MustDate("2026-10-29"),
		ValidUntil: invoice.MustDate("2026-11-28"),
		PriceMode:  vat.PriceNet,
		Lines: []invoice.Line{
			{Description: "Montáž", Quantity: m.MustParse("2"), Unit: "hod", UnitPrice: m.MustParse("40"), Rate: 23},
			{Description: "Kábel", Quantity: m.MustParse("10"), Unit: "m", UnitPrice: m.MustParse("1.50"), Rate: 23},
		},
	}
}

func fields(err error) map[string]string {
	out := map[string]string{}
	var ve *invoice.ValidationError
	if errors.As(err, &ve) {
		for _, f := range ve.Fields {
			out[f.Field] = f.Message
		}
	}
	return out
}

func TestRecalculate(t *testing.T) {
	q := sample()
	if err := q.Recalculate(); err != nil {
		t.Fatal(err)
	}
	if q.Lines[0].Amount.String() != "80.00" || q.Summary.Base.String() != "95.00" || q.Summary.Total.String() != "116.85" {
		t.Fatalf("sumy: %s %s %s", q.Lines[0].Amount, q.Summary.Base, q.Summary.Total)
	}
	if q.Currency != "EUR" {
		t.Fatal("predvolená mena")
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(*Quote)
		field string
		msg   string
	}{
		{"platná", func(*Quote) {}, "", ""},
		{"v korunách bez kurzu", func(q *Quote) { q.Currency = "CZK" }, "", ""},
		{"neznáma mena", func(q *Quote) { q.Currency = "XYZ" }, "currency", "Neznáma mena"},
		{"bez odberateľa", func(q *Quote) { q.Customer.Name = " " }, "customer.name", "Zadajte odberateľa"},
		{"bez položiek", func(q *Quote) { q.Lines = nil }, "lines", "Ponuka musí mať aspoň jednu položku"},
		{"bez platnosti", func(q *Quote) { q.ValidUntil = invoice.Date{} }, "validUntil", "Zadajte, dokedy ponuka platí"},
		{"platnosť pred dátumom", func(q *Quote) { q.ValidUntil = invoice.MustDate("2026-10-01") }, "validUntil",
			"Platnosť nemôže skončiť pred dátumom ponuky"},
		{"stará sadzba", func(q *Quote) { q.Lines[0].Rate = 20 }, "lines[0].vatRate", "Sadzba 20 % neplatí k dátumu ponuky 29. 10. 2026"},
		{"prenesenie bez IČ DPH", func(q *Quote) { q.ReverseCharge = true }, "customer.icdph",
			"Pri prenesení daňovej povinnosti je IČ DPH odberateľa povinné"},
		{"zahraničie bez druhu plnenia", func(q *Quote) { q.Customer.Country = "CZ" }, "supplyKind",
			"Pri zahraničnom odberateľovi vyberte, či ide o tovar alebo službu"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := sample()
			c.edit(&q)
			got := fields(q.Validate())
			if c.field == "" {
				if len(got) != 0 {
					t.Fatalf("neočakávané chyby: %v", got)
				}
				return
			}
			if got[c.field] != c.msg {
				t.Fatalf("%s: %q, chcel som %q (všetky: %v)", c.field, got[c.field], c.msg, got)
			}
			if _, ok := got["dueDate"]; ok {
				t.Fatal("chyba splatnosti nepatrí k ponuke")
			}
		})
	}
}

func TestLifecycle(t *testing.T) {
	q := sample()
	if err := q.Accept(); !errors.Is(err, ErrBadTransition) {
		t.Fatal("koncept sa nedá prijať")
	}
	if err := q.Issue("CP20260001"); err != nil {
		t.Fatal(err)
	}
	if q.Status != StatusIssued || q.Number != "CP20260001" || q.Editable() {
		t.Fatalf("po vystavení: %+v", q)
	}
	if err := q.Issue("CP20260002"); !errors.Is(err, ErrBadTransition) {
		t.Fatal("druhé vystavenie")
	}
	steps := []struct {
		name string
		do   func() error
		want Status
		ok   bool
	}{
		{"odmietnuť", q.Reject, StatusRejected, true},
		{"odmietnutú prijať (zákazník si to rozmyslel)", q.Accept, StatusAccepted, true},
		{"vyfakturovať", func() error { return q.MarkInvoiced(7, "FA20260012") }, StatusInvoiced, true},
		{"vyfakturovanú odmietnuť", q.Reject, StatusInvoiced, false},
		{"druhá faktúra", func() error { return q.MarkInvoiced(8, "FA20260013") }, StatusInvoiced, false},
	}
	for _, s := range steps {
		err := s.do()
		if (err == nil) != s.ok || q.Status != s.want {
			t.Fatalf("%s: err=%v stav=%s", s.name, err, q.Status)
		}
	}
	if q.InvoiceID != 7 || q.InvoiceNumber != "FA20260012" {
		t.Fatalf("odkaz na faktúru: %d %s", q.InvoiceID, q.InvoiceNumber)
	}
}

func TestIssueValidates(t *testing.T) {
	q := sample()
	q.Lines = nil
	if err := q.Issue("CP1"); err == nil || q.Number != "" || q.Status == StatusIssued {
		t.Fatal("neplatná ponuka sa vystavila")
	}
}

func TestToInvoice(t *testing.T) {
	q := sample()
	q.Note = "Termín do konca roka."
	q.ContactID = 3
	if err := q.Issue("CP20260004"); err != nil {
		t.Fatal(err)
	}
	inv := q.ToInvoice(invoice.MustDate("2026-11-05"), 14)
	if inv.Kind != invoice.KindInvoice || inv.Status != invoice.StatusDraft || inv.Number != "" {
		t.Fatalf("druh/stav: %+v", inv)
	}
	if inv.IssueDate.String() != "2026-11-05" || inv.DeliveryDate.String() != "2026-11-05" || inv.DueDate.String() != "2026-11-19" {
		t.Fatalf("dátumy: %s %s %s", inv.IssueDate, inv.DeliveryDate, inv.DueDate)
	}
	if inv.Customer != q.Customer || inv.ContactID != 3 || len(inv.Lines) != 2 {
		t.Fatal("odberateľ alebo položky")
	}
	if inv.Note != "Podľa cenovej ponuky CP20260004.\nTermín do konca roka." {
		t.Fatalf("poznámka: %q", inv.Note)
	}
	inv.Lines[0].Description = "zmena"
	if q.Lines[0].Description != "Montáž" {
		t.Fatal("faktúra zdieľa položky s ponukou")
	}
	if err := inv.Recalculate(); err != nil || !inv.Summary.Total.Equal(q.Summary.Total) {
		t.Fatalf("suma faktúry %s ≠ ponuky %s", inv.Summary.Total, q.Summary.Total)
	}
}

func TestCopyAndExpired(t *testing.T) {
	q := sample()
	_ = q.Issue("CP20260001")
	q.InvoiceID = 9
	today := invoice.MustDate("2026-12-01")
	if !q.Expired(today) || q.Expired(invoice.MustDate("2026-11-28")) {
		t.Fatal("platnosť")
	}
	c := q.Copy(today, 30)
	if c.ID != 0 || c.Number != "" || c.Status != StatusDraft || c.InvoiceID != 0 {
		t.Fatalf("kópia: %+v", c)
	}
	if c.IssueDate.String() != "2026-12-01" || c.ValidUntil.String() != "2026-12-31" {
		t.Fatal("dátumy kópie")
	}
	c.Lines[0].Description = "x"
	if q.Lines[0].Description == "x" {
		t.Fatal("kópia zdieľa položky")
	}
	if c.Expired(today) {
		t.Fatal("koncept nie je po platnosti")
	}
}

// SQLite ukladá ponuku ako JSON — po prečítaní musí byť rovnaká.
func TestJSONRoundTrip(t *testing.T) {
	q := sample()
	q.ID, q.CompanyID, q.ContactID, q.Note = 5, 2, 3, "pozn."
	if err := q.Issue("CP20260001"); err != nil {
		t.Fatal(err)
	}
	for _, x := range []Quote{q, {Status: StatusDraft}} {
		b, err := json.Marshal(x)
		if err != nil {
			t.Fatal(err)
		}
		var back Quote
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(back, x) {
			t.Fatalf("po JSON:\n got %+v\nwant %+v", back, x)
		}
	}
}

func TestBillPart(t *testing.T) {
	p := m.MustParse
	q0 := func() Quote {
		return Quote{Status: StatusAccepted, Number: "CP20260009", Lines: []invoice.Line{
			{Description: "Rozvádzač RM1", Quantity: p("1"), Unit: "ks", UnitPrice: p("4200")},
			{Description: "Montáž", Quantity: p("40"), Unit: "hod", UnitPrice: p("35")},
			{Description: "Zľava", Quantity: p("-1"), Unit: "ks", UnitPrice: p("100")},
		}}
	}
	type step struct {
		qty    []string
		err    bool
		status Status
	}
	for _, tc := range []struct {
		name  string
		steps []step
		left  []string
	}{
		{"polovica a zvyšok", []step{
			{[]string{"0.5", "20", "-0.5"}, false, StatusAccepted},
			{[]string{"0.5", "20", "-0.5"}, false, StatusInvoiced},
		}, []string{"0.00", "0.00", "0.00"}},
		{"po položkách", []step{
			{[]string{"1", "0", "0"}, false, StatusAccepted},
			{[]string{"0", "12", "0"}, false, StatusAccepted},
		}, []string{"0.00", "28.00", "-1.00"}},
		{"viac ako zostáva", []step{{[]string{"0", "41", "0"}, true, StatusAccepted}}, []string{"1.00", "40.00", "-1.00"}},
		{"zlé znamienko pri zľave", []step{{[]string{"0", "0", "1"}, true, StatusAccepted}}, []string{"1.00", "40.00", "-1.00"}},
		{"nič", []step{{[]string{"0", "0", "0"}, true, StatusAccepted}}, []string{"1.00", "40.00", "-1.00"}},
		{"zlý počet", []step{{[]string{"1"}, true, StatusAccepted}}, []string{"1.00", "40.00", "-1.00"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := q0()
			for i, s := range tc.steps {
				qty := make([]m.Dec, len(s.qty))
				for k, x := range s.qty {
					qty[k] = p(x)
				}
				err := q.BillPart(int64(i+1), qty)
				if (err != nil) != s.err || q.Status != s.status {
					t.Fatalf("krok %d: chyba %v, stav %s", i, err, q.Status)
				}
			}
			for i, x := range q.Left() {
				if x.String() != tc.left[i] {
					t.Fatalf("zostatok %v", q.Left())
				}
			}
		})
	}
	q := q0()
	_ = q.BillPart(7, []m.Dec{p("1"), p("40"), p("-1")})
	if q.Status != StatusInvoiced || q.InvoiceID != 7 || q.Amendable() {
		t.Fatalf("celá %+v", q)
	}
	if !q.ReleaseParts(func(id int64) bool { return id != 7 }) || q.Status != StatusAccepted || q.Billed != nil || !q.Amendable() {
		t.Fatalf("uvoľnenie %+v", q)
	}
	if q.ReleaseParts(func(int64) bool { return false }) {
		t.Error("nič na uvoľnenie")
	}
	part := q0()
	_ = part.BillPart(3, []m.Dec{p("1"), p("0"), p("0")})
	if err := part.Reject(); err != ErrBadTransition {
		t.Errorf("čiastočne vyfakturovaná sa neodmieta: %v", err)
	}
	draft := q0()
	draft.Status = StatusDraft
	if err := draft.BillPart(1, []m.Dec{p("1"), p("0"), p("0")}); err != ErrBadTransition {
		t.Errorf("koncept: %v", err)
	}
}
