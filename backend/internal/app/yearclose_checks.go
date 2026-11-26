package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/ledger"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/stock"
)

// Kontroly uzávierky podvojného účtovníctva, ktoré spájajú moduly: účtovné
// odpisy z registra majetku, kurzové rozdiely k 31. 12., ocenenie skladu
// a nadväznosť počiatočných stavov na predchádzajúci rok. Fiškus vie každý
// z týchto zápisov vypočítať, preto pri nezhode rok neuzavrie (okrem
// nadväznosti, kde môžu byť ručné stavy zámerné).

// postedLines vráti riadky interného dokladu, ktorý vyhovie pred.
func postedLines(d LedgerData, pred func(InternalDoc) bool) (string, []ledger.Line, bool) {
	for _, x := range d.Internal {
		if pred(x) {
			return x.Number, x.Lines, true
		}
	}
	return "", nil, false
}

func (a *App) integrityChecks(ctx context.Context, companyID int64, year int, add func(YearCheck)) error {
	d, err := a.loadLedger(ctx, companyID, year)
	if err != nil {
		return err
	}
	// účtovné odpisy
	if dp, err := a.AccDepreciationFor(ctx, companyID, year); err == nil && len(dp.Lines) > 0 {
		num, lines, ok := postedLines(d, func(x InternalDoc) bool { return x.Dep })
		c := YearCheck{ID: "depreciation", Blocking: true, Link: "/hlavna-kniha"}
		switch {
		case !ok:
			c.Text = fmt.Sprintf("Účtovné odpisy majetku za %d (%s €) nie sú zaúčtované", year, dp.Total.Display())
		case !sameLines(lines, dp.Lines):
			c.Text = fmt.Sprintf("Doklad odpisov %s nezodpovedá registru majetku — zaúčtujte odpisy znova", num)
		default:
			c.OK, c.Text = true, fmt.Sprintf("Účtovné odpisy sú zaúčtované (%s)", num)
		}
		add(c)
	} else if err != nil {
		return err
	}
	// kurzové rozdiely k 31. 12.
	fx, err := a.FXRevaluationFor(ctx, companyID, year)
	if err != nil {
		return err
	}
	if len(fx.Missing) > 0 {
		add(YearCheck{ID: "fx-rates", Blocking: true, Link: "/kurzy",
			Text: fmt.Sprintf("Chýba kurz k 31. 12. %d pre %s — bez neho sa cudzia mena neprepočíta", year, strings.Join(fx.Missing, ", "))})
	}
	if len(fx.Lines) > 0 {
		num, lines, ok := postedLines(d, func(x InternalDoc) bool { return x.FX })
		c := YearCheck{ID: "fx", Blocking: true, Link: "/hlavna-kniha"}
		switch {
		case !ok:
			c.Text = fmt.Sprintf("Kurzové rozdiely k 31. 12. %d nie sú zaúčtované (%d položiek v cudzej mene)", year, len(fx.Items))
		case !sameLines(lines, fx.Lines):
			c.Text = fmt.Sprintf("Doklad kurzových rozdielov %s nezodpovedá otvoreným položkám — zaúčtujte ho znova", num)
		default:
			c.OK, c.Text = true, fmt.Sprintf("Kurzové rozdiely k 31. 12. sú zaúčtované (%s)", num)
		}
		add(c)
	}
	for i, w := range fx.Warnings {
		add(YearCheck{ID: fmt.Sprintf("fx-warning-%d", i), Text: w, Link: "/hlavna-kniha"})
	}
	// ocenenie skladu
	if err := a.stockValuationCheck(ctx, companyID, year, add); err != nil {
		return err
	}
	// nadväznosť počiatočných stavov
	return a.openingContinuityCheck(ctx, companyID, year, d, add)
}

// stockValuationCheck: výdaj nad stav sa ocení poslednou známou cenou
// a zaúčtuje — taký zápis je odhad, nie ocenenie, a rok sa s ním neuzavrie.
func (a *App) stockValuationCheck(ctx context.Context, companyID int64, year int, add func(YearCheck)) error {
	docs, err := a.Store.ListStockDocs(ctx, companyID)
	if err != nil || len(docs) == 0 {
		return err
	}
	cfg, err := a.loadStock(ctx, companyID)
	if err != nil {
		return err
	}
	inYear := map[int64]bool{}
	for _, x := range docs {
		inYear[x.ID] = x.Date.Year() == year
	}
	_, _, probs := stock.Valuate(docs, cfg.Method)
	var list []string
	for _, p := range probs {
		if inYear[p.DocID] {
			list = append(list, p.Number)
		}
	}
	c := YearCheck{ID: "stock-valuation", Blocking: true, Link: "/sklad", OK: len(list) == 0, Text: "Sklad je ocenený bez chýb"}
	if !c.OK {
		if len(list) > 5 {
			list = append(list[:5], "…")
		}
		c.Text = fmt.Sprintf("Sklad: %d dokladov s výdajom nad stav alebo inou chybou ocenenia (%s) — doplňte príjemky alebo opravte množstvá",
			len(list), strings.Join(list, ", "))
	}
	add(c)
	return nil
}

// openingContinuityCheck porovná počiatočné stavy roka s konečnými stavmi
// predchádzajúceho roka v programe (ak ho Fiškus má).
func (a *App) openingContinuityCheck(ctx context.Context, companyID int64, year int, d LedgerData, add func(YearCheck)) error {
	prev, err := a.Ledger(ctx, companyID, year-1)
	if err != nil {
		return err
	}
	if len(prev.Postings) == 0 && len(prev.Opening) == 0 {
		return nil // predchádzajúci rok nie je vo Fiškuse — stavy sú prevzaté z iného programu
	}
	want := map[string]money.Dec{}
	for _, l := range ledger.OpeningFromClosing(prev.Accounts) {
		want[l.Account] = want[l.Account].Add(l.Debit).Sub(l.Credit)
	}
	diff := openingDiff(d.Opening, want)
	c := YearCheck{ID: "opening", Link: "/hlavna-kniha", OK: len(diff) == 0,
		Text: fmt.Sprintf("Počiatočné stavy nadväzujú na konečné stavy roka %d", year-1)}
	if !c.OK {
		if len(diff) > 5 {
			diff = append(diff[:5], "…")
		}
		c.Text = fmt.Sprintf("Počiatočné stavy sa líšia od konečných stavov roka %d (účty %s) — uzavrite rok %d znova alebo stavy upravte",
			year-1, strings.Join(diff, ", "), year-1)
	}
	add(c)
	return nil
}

// openingDiff vráti účty, na ktorých sa stavy líšia (zoradené).
func openingDiff(got, want map[string]money.Dec) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range []map[string]money.Dec{got, want} {
		for acc := range m {
			if !seen[acc] && !got[acc].Equal(want[acc]) {
				out = append(out, acc)
			}
			seen[acc] = true
		}
	}
	sort.Strings(out)
	return out
}
