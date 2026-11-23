package purchase

import (
	"errors"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
)

// ErrOverpaid — úhrada je vyššia ako zostatok faktúry.
var ErrOverpaid = errors.New("úhrada je vyššia ako zostatok prijatej faktúry")

// Paid vráti uhradenú sumu v mene faktúry. Faktúra uhradená pred
// evidenciou úhrad je uhradená celá.
func (p *Purchase) Paid() money.Dec {
	if len(p.Payments) == 0 && p.Status == StatusPaid {
		return p.Summary.Total
	}
	return p.paymentsSum()
}

func (p *Purchase) paymentsSum() money.Dec {
	sum := money.Zero
	for _, x := range p.Payments {
		sum = sum.Add(x.Amount)
	}
	return sum
}

// PaidAt vráti úhrady s dátumom najneskôr at (kniha záväzkov k dňu).
func (p *Purchase) PaidAt(at invoice.Date) money.Dec {
	if len(p.Payments) == 0 {
		if p.Status == StatusPaid && (p.PaidDate.IsZero() || !p.PaidDate.After(at)) {
			return p.Summary.Total
		}
		return money.Zero
	}
	sum := money.Zero
	for _, x := range p.Payments {
		if !x.Date.After(at) {
			sum = sum.Add(x.Amount)
		}
	}
	return sum
}

// Remaining vráti zostatok na úhradu (pri dobropise záporný = zostáva vrátiť).
func (p *Purchase) Remaining() money.Dec { return p.Summary.Total.Sub(p.Paid()) }

// RemainingEUR vráti zostatok v eurách (v cudzej mene pomerom súm dokladu).
func (p *Purchase) RemainingEUR() money.Dec {
	rem := p.Remaining()
	if !p.Foreign() {
		return rem
	}
	if p.Summary.Total.IsZero() {
		return money.Zero
	}
	return rem.MulRatio(p.TotalEUR(), p.Summary.Total).RoundCents()
}

// PartlyPaid vráti true pre neuhradenú faktúru s časťou úhrady.
func (p *Purchase) PartlyPaid() bool { return p.Status == StatusBooked && !p.Paid().IsZero() }

// SyncPaid prepočíta uhradenú sumu a zostatok.
func (p *Purchase) SyncPaid() {
	p.PaidAmount, p.Balance, p.BalanceEUR = p.Paid(), p.Remaining(), p.RemainingEUR()
}

// AddPayment pripíše úhradu (aj čiastočnú). Suma má znamienko faktúry
// (pri dobropise vrátené peniaze záporne) a neprevýši zostatok. Keď je
// zostatok nula, faktúra je uhradená dátumom poslednej úhrady.
func (p *Purchase) AddPayment(x invoice.Payment) error {
	if p.Status != StatusBooked {
		return invoice.ErrBadTransition
	}
	total, rest := p.Summary.Total, p.Remaining()
	if !total.IsZero() && (x.Amount.IsZero() || x.Amount.Sign() != total.Sign()) {
		return invoice.ErrBadPayment
	}
	if x.Amount.Abs().Cmp(rest.Abs()) > 0 {
		return ErrOverpaid
	}
	if !x.Amount.IsZero() {
		p.Payments = append(p.Payments, x)
	}
	if p.Remaining().IsZero() {
		p.Status, p.PaidDate = StatusPaid, x.Date
	}
	p.SyncPaid()
	return nil
}

// RemovePayment zruší úhradu z daného zdroja (zrušené spárovanie pohybu,
// stornovaný pokladničný doklad). Faktúru uhradenú pred evidenciou úhrad
// vráti do stavu neuhradená. Vráti false, ak taká úhrada nie je.
func (p *Purchase) RemovePayment(source string, ref int64) bool {
	if len(p.Payments) == 0 {
		if p.Status != StatusPaid {
			return false
		}
		p.Status, p.PaidDate = StatusBooked, invoice.Date{}
		p.SyncPaid()
		return true
	}
	for i, x := range p.Payments {
		if x.Source == source && x.Ref == ref {
			p.Payments = append(p.Payments[:i:i], p.Payments[i+1:]...)
			if !p.Remaining().IsZero() {
				p.Status, p.PaidDate = StatusBooked, invoice.Date{}
			}
			p.SyncPaid()
			return true
		}
	}
	return false
}
