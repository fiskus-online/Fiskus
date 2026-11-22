package purchase

import (
	"errors"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
)

func TestPayments(t *testing.T) {
	d := invoice.MustDate
	mk := func(total string) Purchase {
		p := Purchase{Status: StatusBooked, Summary: Summary{Total: money.MustParse(total)}}
		p.SyncPaid()
		return p
	}
	pay := func(date, amt string, ref int64) invoice.Payment {
		return invoice.Payment{Date: d(date), Amount: money.MustParse(amt), Source: invoice.PaymentBank, Ref: ref}
	}
	tests := []struct {
		name    string
		total   string
		pays    []invoice.Payment
		err     error
		status  Status
		balance string
	}{
		{"čiastočná úhrada", "615", []invoice.Payment{pay("2026-10-20", "200", 1)}, nil, StatusBooked, "415"},
		{"dve úhrady = uhradená", "615", []invoice.Payment{pay("2026-10-20", "200", 1), pay("2026-11-05", "415", 2)}, nil, StatusPaid, "0"},
		{"preplatenie sa odmietne", "615", []invoice.Payment{pay("2026-10-20", "700", 1)}, ErrOverpaid, StatusBooked, "615"},
		{"nulová suma", "615", []invoice.Payment{pay("2026-10-20", "0", 1)}, invoice.ErrBadPayment, StatusBooked, "615"},
		{"dobropis: vrátené peniaze záporne", "-50", []invoice.Payment{pay("2026-10-20", "-20", 1)}, nil, StatusBooked, "-30"},
		{"dobropis: kladná suma je chyba", "-50", []invoice.Payment{pay("2026-10-20", "20", 1)}, invoice.ErrBadPayment, StatusBooked, "-50"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := mk(tt.total)
			var err error
			for _, x := range tt.pays {
				if err = p.AddPayment(x); err != nil {
					break
				}
			}
			if !errors.Is(err, tt.err) || p.Status != tt.status || !p.Balance.Equal(money.MustParse(tt.balance)) {
				t.Fatalf("%v %s zostáva %s", err, p.Status, p.Balance)
			}
		})
	}
}

func TestPaidAtAndRemove(t *testing.T) {
	d := invoice.MustDate
	p := Purchase{Status: StatusBooked, Summary: Summary{Total: money.MustParse("615")}}
	_ = p.AddPayment(invoice.Payment{Date: d("2026-10-20"), Amount: money.MustParse("200"), Source: invoice.PaymentBank, Ref: 1})
	_ = p.AddPayment(invoice.Payment{Date: d("2026-11-05"), Amount: money.MustParse("415"), Source: invoice.PaymentCash, Ref: 9})
	for at, want := range map[string]string{"2026-10-19": "0", "2026-10-20": "200", "2026-10-31": "200", "2026-11-05": "615"} {
		if got := p.PaidAt(d(at)); !got.Equal(money.MustParse(want)) {
			t.Errorf("%s: %s, chcem %s", at, got, want)
		}
	}
	if p.RemovePayment(invoice.PaymentBank, 9) {
		t.Fatal("iný zdroj sa nemá zrušiť")
	}
	if !p.RemovePayment(invoice.PaymentCash, 9) || p.Status != StatusBooked || !p.PaidDate.IsZero() || !p.Balance.Equal(money.MustParse("415")) {
		t.Fatalf("po zrušení: %s %s %s", p.Status, p.PaidDate, p.Balance)
	}
	// faktúra uhradená pred evidenciou úhrad (bez zoznamu) je uhradená celá
	old := Purchase{Status: StatusPaid, PaidDate: d("2026-09-30"), Summary: Summary{Total: money.MustParse("100")}}
	if !old.Paid().Equal(money.MustParse("100")) || !old.PaidAt(d("2026-09-29")).IsZero() || !old.RemovePayment(invoice.PaymentBank, 5) || old.Status != StatusBooked {
		t.Fatalf("stará úhrada: %+v", old)
	}
	// ručné označenie uhradí zvyšok
	if err := p.MarkPaid(d("2026-12-01")); err != nil || p.Status != StatusPaid || len(p.Payments) != 2 || !p.Payments[1].Amount.Equal(money.MustParse("415")) {
		t.Fatalf("MarkPaid: %v %+v", err, p.Payments)
	}
}
