package app

import (
	"context"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/household"
)

// Párovanie bločkov s platbami kartou: bloček (z eKasy alebo z fotky) má
// položky a kategórie, platba z banky iba sumu a obchod. Keď sedí suma a
// platba prišla v deň nákupu alebo o pár dní neskôr, je to ten istý výdavok
// — zostane jeden pohyb: bloček na bankovom účte.

// pairDays: koľko dní po nákupe sa platba kartou môže objaviť v banke.
const pairDays = 4

// pairable: bloček, ktorý ešte nemá platbu z banky.
func pairable(e household.Entry) bool {
	return e.Receipt != nil && e.Paired == "" && !e.Transfer() && e.Amount.Sign() < 0
}

// bankOnly: pohyb z banky bez bločku (čaká na bloček).
func bankOnly(e household.Entry) bool {
	return strings.HasPrefix(e.Source, "bank:") && e.Receipt == nil && len(e.Split) == 0 &&
		!e.Transfer() && e.ObligationID == 0 && e.Amount.Sign() < 0
}

// pairScore: či ide o ten istý nákup (-1 = nie). Vyššie je lepšie: zhoda
// názvu obchodu, potom menší odstup dní.
func pairScore(receipt, payment household.Entry) int {
	if receipt.Amount.Cmp(payment.Amount) != 0 {
		return -1
	}
	days := receipt.Date.DaysUntil(payment.Date)
	if days < 0 || days > pairDays {
		return -1
	}
	score := pairDays - days
	if sameShop(receipt, payment) {
		score += 10
	}
	return score
}

// sameShop: spoločné slovo názvu obchodu na bločku a v texte platby.
func sameShop(receipt, payment household.Entry) bool {
	names := receipt.Payee
	if receipt.Receipt != nil {
		names += " " + receipt.Receipt.Seller.Name
	}
	text := " " + strings.Join(household.Words(payment.Payee+" "+payment.Note), " ") + " "
	for _, w := range household.Words(names) {
		if len(w) >= 3 && !shopNoise[w] && strings.Contains(text, " "+w+" ") {
			return true
		}
	}
	return false
}

// slová, ktoré sú v názve skoro každej firmy
var shopNoise = map[string]bool{"sro": true, "spol": true, "vos": true, "slovakia": true, "slovensko": true, "slovenska": true,
	"republika": true, "republic": true, "ceska": true, "kft": true, "gmbh": true, "obchod": true, "predajna": true, "nakup": true, "platba": true, "karta": true, "kartou": true}

// best vráti index kandidáta s najlepším skóre; pri rovnakom skóre dvoch
// rôznych kandidátov sa nepáruje (nevieme, ktorý to je).
func best(scores []int) int {
	idx, top, tie := -1, -1, false
	for i, s := range scores {
		switch {
		case s < 0:
		case s > top:
			idx, top, tie = i, s, false
		case s == top:
			tie = true
		}
	}
	if tie {
		return -1
	}
	return idx
}

// pairReceipts pri načítaní pohybov z banky: preskočí už načítané (aj tie,
// ktoré sú spárované s bločkom), platbu kartou spojí s bločkom a vráti
// pohyby, ktoré treba pridať.
func (a *App) pairReceipts(ctx context.Context, companyID int64, es []household.Entry, res *HouseholdImport) ([]household.Entry, error) {
	hist, err := a.Store.ListHHEntries(ctx, companyID, 0)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, e := range hist {
		if e.Source != "" {
			known[e.Source] = true
		}
		if e.Paired != "" {
			known[e.Paired] = true
		}
	}
	used := map[int64]bool{}
	var rest []household.Entry
	for _, b := range es {
		if known[b.Source] {
			res.Skipped++
			continue
		}
		known[b.Source] = true
		idx := -1
		if b.Amount.Sign() < 0 {
			scores := make([]int, len(hist))
			for i, r := range hist {
				scores[i] = -1
				if pairable(r) && !used[r.ID] {
					scores[i] = pairScore(r, b)
				}
			}
			idx = best(scores)
		}
		if idx < 0 {
			rest = append(rest, b)
			continue
		}
		r := hist[idx]
		before := r
		r.AccountID, r.Paired = b.AccountID, b.Source
		if err := a.Store.SaveHHEntry(ctx, companyID, &r); err != nil {
			return nil, err
		}
		used[r.ID] = true
		hist[idx] = r
		res.Paired++
		res.Entries = append(res.Entries, r)
		a.record(ctx, companyID, audit.CatLedger, "pair", "household_entry", r.ID,
			"Domácnosť: bloček spárovaný s platbou kartou z banky "+entrySummary(b), before, r)
	}
	return rest, nil
}

// bankPaymentFor nájde k novému bločku platbu kartou, ktorá už prišla z banky.
func (a *App) bankPaymentFor(ctx context.Context, companyID int64, receipt household.Entry) (*household.Entry, error) {
	if !pairable(receipt) {
		return nil, nil
	}
	hist, err := a.Store.ListHHEntries(ctx, companyID, 0)
	if err != nil {
		return nil, err
	}
	scores := make([]int, len(hist))
	for i, b := range hist {
		scores[i] = -1
		if bankOnly(b) {
			scores[i] = pairScore(receipt, b)
		}
	}
	if i := best(scores); i >= 0 {
		return &hist[i], nil
	}
	return nil, nil
}
