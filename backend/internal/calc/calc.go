// Package calc sú kalkulačky pre každého (aj bez založenej firmy): DPH,
// úrok z omeškania, úver so splátkovým kalendárom a RPMN, sporenie.
// Mzdovú kalkulačku a kurzy počíta balík app z payroll a fx.
//
// Peniaze sú money.Dec; mocniny (anuita, zložené úročenie) sa počítajú
// presne v zlomkoch (big.Rat) a zaokrúhľujú až na konci.
package calc

import (
	"math"
	"math/big"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/reminder"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

// FieldError je chyba vstupu kalkulačky (pole, správa).
type FieldError struct{ Field, Message string }

func (e *FieldError) Error() string { return e.Message }

func bad(field, msg string) error { return &FieldError{field, msg} }

var hundred = money.FromInt(100)

// ---- DPH ----

// VATInput: suma bez DPH (Gross = false) alebo s DPH (Gross = true).
type VATInput struct {
	Amount money.Dec `json:"amount"`
	Rate   int       `json:"rate"`
	Gross  bool      `json:"gross"`
}

// VATResult je základ, DPH a suma s DPH.
type VATResult struct {
	Base  money.Dec `json:"base"`
	VAT   money.Dec `json:"vat"`
	Total money.Dec `json:"total"`
}

// VAT rozpočíta sumu rovnako ako doklad (vat.Calculate).
func VAT(in VATInput) (VATResult, error) {
	if !vat.Rate(in.Rate).Known() {
		return VATResult{}, bad("rate", "Neznáma sadzba DPH")
	}
	mode := vat.PriceNet
	if in.Gross {
		mode = vat.PriceGross
	}
	s, err := vat.Calculate([]vat.Line{{Quantity: money.FromInt(1), UnitPrice: in.Amount, Rate: vat.Rate(in.Rate)}}, mode, false)
	if err != nil {
		return VATResult{}, err
	}
	return VATResult{Base: s.Base, VAT: s.VAT, Total: s.Total}, nil
}

// ---- úrok z omeškania ----

// Debtor určuje sadzbu úroku z omeškania.
type Debtor string

const (
	DebtorBusiness Debtor = "business" // medzi podnikateľmi (§ 369 ObZ)
	DebtorConsumer Debtor = "consumer" // občianskoprávny vzťah (§ 517 OZ)
	DebtorCustom   Debtor = "custom"   // dohodnutá sadzba
)

// LateInput je dlžná suma, splatnosť a deň úhrady.
type LateInput struct {
	Amount money.Dec    `json:"amount"`
	Due    invoice.Date `json:"due"`  // posledný deň splatnosti
	Paid   invoice.Date `json:"paid"` // deň úhrady (alebo ku dňu)
	Debtor Debtor       `json:"debtor"`
	Rate   money.Dec    `json:"rate"` // dohodnutá sadzba % p. a. (Debtor = custom)
}

// LateResult je výsledok výpočtu úroku z omeškania.
type LateResult struct {
	From     invoice.Date `json:"from"` // prvý deň omeškania
	Days     int          `json:"days"`
	ECB      money.Dec    `json:"ecb"`    // sadzba ECB k prvému dňu omeškania
	Margin   money.Dec    `json:"margin"` // prirážka v p. b.
	Rate     money.Dec    `json:"rate"`   // % p. a.
	Interest money.Dec    `json:"interest"`
	Fee      money.Dec    `json:"fee"` // paušálna náhrada (podnikatelia)
	Total    money.Dec    `json:"total"`
}

// LateInterest vypočíta úrok z omeškania podľa zákonných parametrov law.
func LateInterest(in LateInput, law reminder.Law) (LateResult, error) {
	var r LateResult
	switch {
	case in.Amount.Sign() <= 0:
		return r, bad("amount", "Zadajte dlžnú sumu")
	case in.Due.IsZero():
		return r, bad("due", "Zadajte dátum splatnosti")
	case in.Paid.IsZero():
		return r, bad("paid", "Zadajte dátum úhrady")
	case !in.Due.Before(in.Paid):
		return r, bad("paid", "Úhrada je v lehote splatnosti — úrok z omeškania nevzniká")
	}
	r.From = in.Due.AddDays(1)
	r.Days = r.From.DaysUntil(in.Paid) + 1
	switch in.Debtor {
	case DebtorCustom:
		if in.Rate.Sign() <= 0 {
			return r, bad("rate", "Zadajte dohodnutú sadzbu")
		}
		r.Rate = in.Rate
	case DebtorBusiness, DebtorConsumer, "":
		r.ECB = law.ECB.On(r.From)
		r.Margin = law.ConsumerMargin
		if in.Debtor == DebtorBusiness {
			r.Margin = law.BusinessMargin
			r.Fee = law.FlatFee
		}
		r.Rate = r.ECB.Add(r.Margin)
	default:
		return r, bad("debtor", "Neznámy druh vzťahu")
	}
	r.Interest = reminder.Interest(in.Amount, r.Rate, r.From, in.Paid)
	r.Total = in.Amount.Add(r.Interest).Add(r.Fee)
	return r, nil
}

// ---- úver ----

// LoanInput je anuitný úver s mesačnými splátkami.
type LoanInput struct {
	Amount     money.Dec `json:"amount"`
	Rate       money.Dec `json:"rate"`   // úroková sadzba % p. a.
	Months     int       `json:"months"` // počet splátok
	Fee        money.Dec `json:"fee"`    // jednorazový poplatok za poskytnutie
	MonthlyFee money.Dec `json:"monthlyFee"`
}

// LoanRow je riadok splátkového kalendára.
type LoanRow struct {
	N         int       `json:"n"`
	Payment   money.Dec `json:"payment"`
	Interest  money.Dec `json:"interest"`
	Principal money.Dec `json:"principal"`
	Fee       money.Dec `json:"fee"`
	Balance   money.Dec `json:"balance"`
}

// LoanResult je splátka, celkové náklady, RPMN a splátkový kalendár.
type LoanResult struct {
	Payment  money.Dec `json:"payment"` // mesačná splátka bez poplatku
	Interest money.Dec `json:"interest"`
	Fees     money.Dec `json:"fees"`
	Total    money.Dec `json:"total"` // spolu zaplatí
	Cost     money.Dec `json:"cost"`  // celkové náklady úveru (úroky a poplatky)
	APR      money.Dec `json:"apr"`   // RPMN % p. a.
	Schedule []LoanRow `json:"schedule"`
}

func dec(r *big.Rat) money.Dec {
	d, _ := money.FromRat(r)
	return d
}

// Loan vypočíta anuitnú splátku. Úrok za mesiac = zostatok × sadzba / 12,
// posledná splátka doplatí zvyšok po zaokrúhľovaní.
func Loan(in LoanInput) (LoanResult, error) {
	var r LoanResult
	switch {
	case in.Amount.Sign() <= 0:
		return r, bad("amount", "Zadajte výšku úveru")
	case in.Months < 1 || in.Months > 600:
		return r, bad("months", "Počet splátok je 1 až 600")
	case in.Rate.Sign() < 0 || in.Rate.Cmp(hundred) > 0:
		return r, bad("rate", "Sadzba je 0 až 100 %")
	case in.Fee.Sign() < 0 || in.MonthlyFee.Sign() < 0:
		return r, bad("fee", "Poplatok nemôže byť záporný")
	}
	n := in.Months
	i := new(big.Rat).Quo(in.Rate.Rat(), big.NewRat(1200, 1)) // mesačná sadzba
	pay := new(big.Rat).Quo(in.Amount.Rat(), big.NewRat(int64(n), 1))
	if i.Sign() > 0 {
		// P × i / (1 − (1 + i)^−n) = P × i × q^n / (q^n − 1)
		qn := pow(new(big.Rat).Add(big.NewRat(1, 1), i), n)
		pay = new(big.Rat).Mul(in.Amount.Rat(), i)
		pay.Mul(pay, qn).Quo(pay, new(big.Rat).Sub(qn, big.NewRat(1, 1)))
	}
	r.Payment = dec(pay).RoundCents()
	balance := in.Amount
	for k := 1; k <= n; k++ {
		interest := dec(new(big.Rat).Mul(balance.Rat(), i)).RoundCents()
		principal := r.Payment.Sub(interest)
		if k == n || principal.Cmp(balance) > 0 {
			principal = balance
		}
		balance = balance.Sub(principal)
		row := LoanRow{N: k, Payment: principal.Add(interest), Interest: interest, Principal: principal, Fee: in.MonthlyFee, Balance: balance}
		r.Schedule = append(r.Schedule, row)
		r.Interest = r.Interest.Add(interest)
		r.Total = r.Total.Add(row.Payment).Add(in.MonthlyFee)
	}
	r.Fees = in.Fee.Add(in.MonthlyFee.Mul(money.FromInt(int64(n))))
	r.Total = r.Total.Add(in.Fee)
	r.Cost = r.Total.Sub(in.Amount)
	r.APR = apr(in.Amount.Sub(in.Fee), r.Schedule)
	return r, nil
}

func pow(x *big.Rat, n int) *big.Rat {
	out, b := big.NewRat(1, 1), new(big.Rat).Set(x)
	for ; n > 0; n >>= 1 {
		if n&1 == 1 {
			out.Mul(out, b)
		}
		b.Mul(b, b)
	}
	return out
}

// apr je RPMN: ročná sadzba X, pri ktorej sa vyplatená suma rovná súčasnej
// hodnote splátok, Σ splátka_k / (1 + X)^(k/12) (smernica 2008/48/ES,
// príloha I). Je to percento, nie suma — rieši sa bisekciou vo float64
// a výsledok sa zaokrúhli na 2 miesta.
func apr(paidOut money.Dec, rows []LoanRow) money.Dec {
	if paidOut.Sign() <= 0 {
		return money.Zero
	}
	cash := make([]float64, len(rows))
	for k, row := range rows {
		f, _ := row.Payment.Add(row.Fee).Rat().Float64()
		cash[k] = f
	}
	target, _ := paidOut.Rat().Float64()
	pv := func(x float64) float64 {
		s := 0.0
		for k, c := range cash {
			s += c / math.Pow(1+x, float64(k+1)/12)
		}
		return s
	}
	lo, hi := -0.99, 10.0
	if pv(lo) < target || pv(hi) > target {
		return money.Zero
	}
	for range 200 {
		mid := (lo + hi) / 2
		if pv(mid) > target {
			lo = mid
		} else {
			hi = mid
		}
	}
	return dec(new(big.Rat).SetFloat64((lo + hi) / 2 * 100)).RoundCents()
}

// ---- sporenie ----

// SavingsInput je sporenie s počiatočným a mesačným vkladom, mesačným
// pripisovaním úrokov a zrážkovou daňou z úrokov.
type SavingsInput struct {
	Initial money.Dec `json:"initial"`
	Monthly money.Dec `json:"monthly"`
	Rate    money.Dec `json:"rate"` // % p. a.
	Months  int       `json:"months"`
	Tax     money.Dec `json:"tax"` // daň z úrokov v % (19)
}

// SavingsYear je stav na konci roka sporenia.
type SavingsYear struct {
	Year     int       `json:"year"`
	Deposits money.Dec `json:"deposits"`
	Interest money.Dec `json:"interest"` // čisté úroky od začiatku
	Balance  money.Dec `json:"balance"`
}

// SavingsResult je výsledok sporenia.
type SavingsResult struct {
	Deposits money.Dec     `json:"deposits"`
	Interest money.Dec     `json:"interest"` // úroky pred zdanením
	Tax      money.Dec     `json:"tax"`
	Balance  money.Dec     `json:"balance"`
	Years    []SavingsYear `json:"years"`
}

// Savings vypočíta zostatok sporenia: mesačný vklad na začiatku každého mesiaca, úrok
// za mesiac = zostatok × sadzba / 12, daň sa zrazí z každého pripísaného úroku.
func Savings(in SavingsInput) (SavingsResult, error) {
	var r SavingsResult
	switch {
	case in.Initial.Sign() < 0 || in.Monthly.Sign() < 0:
		return r, bad("monthly", "Vklad nemôže byť záporný")
	case in.Initial.IsZero() && in.Monthly.IsZero():
		return r, bad("monthly", "Zadajte počiatočný alebo mesačný vklad")
	case in.Months < 1 || in.Months > 600:
		return r, bad("months", "Doba sporenia je 1 až 600 mesiacov")
	case in.Rate.Sign() < 0 || in.Rate.Cmp(hundred) > 0:
		return r, bad("rate", "Sadzba je 0 až 100 %")
	case in.Tax.Sign() < 0 || in.Tax.Cmp(hundred) > 0:
		return r, bad("tax", "Daň je 0 až 100 %")
	}
	bal := in.Initial
	r.Deposits = in.Initial
	for k := 1; k <= in.Months; k++ {
		bal = bal.Add(in.Monthly)
		r.Deposits = r.Deposits.Add(in.Monthly)
		gross := bal.MulRatio(in.Rate, money.FromInt(1200)).RoundCents()
		tax := gross.MulRatio(in.Tax, hundred).RoundCents()
		bal = bal.Add(gross).Sub(tax)
		r.Interest, r.Tax = r.Interest.Add(gross), r.Tax.Add(tax)
		if k%12 == 0 || k == in.Months {
			r.Years = append(r.Years, SavingsYear{Year: (k + 11) / 12, Deposits: r.Deposits, Interest: r.Interest.Sub(r.Tax), Balance: bal})
		}
	}
	r.Balance = bal
	return r, nil
}
