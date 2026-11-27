package dpfo

import "github.com/fiskus-online/fiskus/backend/internal/money"

// Ročné zúčtovanie preddavkov na daň zo závislej činnosti (§ 38 zákona
// o dani z príjmov). Riadky podľa vzoru tlačiva MF SR „Ročné zúčtovanie
// preddavkov na daň z príjmov fyzickej osoby zo závislej činnosti“:
// 01 príjmy, 02 poistné, 03 základ, 04 nezdaniteľné časti, 05 zdaniteľná
// mzda, 06 daň, 09 – 14 daňový bonus, 18 preddavky, 19 – 22 výsledok.
// Zamestnanecká prémia a bonus na zaplatené úroky Fiškus nepočíta.

// SettlementInput sú ročné údaje zamestnanca.
type SettlementInput struct {
	Income    money.Dec // r. 01 zdaniteľné príjmy
	Agreement money.Dec // r. 01a z toho z dohôd
	Social    money.Dec // r. 02a poistné na sociálne poistenie
	Health    money.Dec // r. 02b poistné na zdravotné poistenie
	DDS       money.Dec // zaplatené príspevky na DDS (III. pilier)
	Spouse    *Spouse   // NČZD na manžela/ku; nil = neuplatňuje
	Children  []Child   // deti, na ktoré si uplatňuje bonus
	BonusPaid money.Dec // r. 10 bonus priznaný a vyplatený zamestnávateľom
	Prepaid   money.Dec // r. 18 zrazené preddavky (neupravené o bonus)
}

// Settlement sú riadky ročného zúčtovania.
type Settlement struct {
	R01, R01a, R02, R02a, R02b, R03 money.Dec
	R04a, R04b, R04c, R04, R05, R06 money.Dec
	R09, R10, R11, R12, R13, R14    money.Dec
	R18, R19, R20                   money.Dec
	// R21: daň na úhradu (> 0) alebo preplatok (< 0) po bonuse.
	R21 money.Dec
	// R22: zamestnávateľ zrazí zamestnancovi (> 0) alebo vyplatí (< 0).
	R22   money.Dec
	Notes []string
}

// AnnualNCZD je ročná nezdaniteľná časť na daňovníka zo základu dane.
func AnnualNCZD(base money.Dec, p Params) money.Dec { return nczd(base, p) }

// AnnualTax je daň podľa § 15 zo základu (pásma sadzieb roka).
func AnnualTax(base money.Dec, p Params) money.Dec { return progressive(base, p.Brackets) }

// Settle vypočíta ročné zúčtovanie.
func Settle(in SettlementInput, p Params) Settlement {
	var s Settlement
	s.R01, s.R01a = in.Income.RoundCents(), in.Agreement.RoundCents()
	s.R02a, s.R02b = in.Social.RoundCents(), in.Health.RoundCents()
	s.R02 = s.R02a.Add(s.R02b)
	s.R03 = maxDec(money.Zero, s.R01.Sub(s.R02))
	s.R04a = nczd(s.R03, p)
	s.R04b = spouseNCZD(in.Spouse, s.R03, p)
	s.R04c = minDec(maxDec(money.Zero, in.DDS.RoundCents()), p.DDSMax)
	s.R04 = minDec(s.R03, s.R04a.Add(s.R04b).Add(s.R04c))
	s.R05 = s.R03.Sub(s.R04)
	s.R06 = progressive(s.R05, p.Brackets)

	bonus, err := Bonus(in.Children, s.R03, p)
	if err != nil {
		s.Notes = append(s.Notes, "Daňový bonus sa nedá vypočítať: "+err.Error())
		bonus = money.Zero
	}
	s.R09, s.R10 = bonus, in.BonusPaid.RoundCents()
	s.R11 = maxDec(money.Zero, s.R09.Sub(s.R10))
	s.R12 = maxDec(money.Zero, s.R11.Sub(s.R06))
	s.R13 = minDec(money.Zero, s.R09.Sub(s.R10)) // záporné: bonus na vybratie
	s.R14 = maxDec(money.Zero, s.R06.Sub(s.R09))

	s.R18 = in.Prepaid.RoundCents()
	d := s.R06.Sub(s.R18)
	s.R19 = maxDec(money.Zero, d)
	s.R20 = minDec(money.Zero, d) // záporné: preplatok
	s.R21 = s.R06.Sub(s.R09).Add(s.R10).Add(s.R12).Sub(s.R18)
	// rozdiel dane mínus rozdiel bonusu
	s.R22 = d.Sub(s.R09.Sub(s.R10))
	return s
}
