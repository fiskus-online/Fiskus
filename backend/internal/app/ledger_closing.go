package app

import (
	"context"
	"fmt"

	"github.com/fiskus-online/fiskus/backend/internal/chart"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/ledger"
	"github.com/fiskus-online/fiskus/backend/internal/money"
)

// LedgerClosing sú uzávierkové zápisy roka (k 31. 12., účty 710 a 702)
// a otváracie zápisy ďalšieho roka (k 1. 1., účet 701).
type LedgerClosing struct {
	Year        int            `json:"year"`
	Closed      bool           `json:"closed"` // rok je uzavretý (inak návrh)
	ClosingDate invoice.Date   `json:"closingDate"`
	OpeningDate invoice.Date   `json:"openingDate"`
	Result      money.Dec      `json:"result"` // výsledok hospodárenia (kladný = zisk)
	Closing     []ledger.Entry `json:"closing"`
	Opening     []ledger.Entry `json:"opening"`
}

// LedgerClosingEntries zostaví uzávierkové a otváracie zápisy zo zostatkov
// hlavnej knihy roka. Do obratov sa nezapočítavajú — sú dokladom
// o uzavretí účtovných kníh.
func (a *App) LedgerClosingEntries(ctx context.Context, companyID int64, year int) (LedgerClosing, error) {
	out := LedgerClosing{Year: year, Closing: []ledger.Entry{}, Opening: []ledger.Entry{}}
	if year < 2001 || year > 2100 {
		return out, mfaFieldErr("year", "Neplatný rok")
	}
	book, err := a.Ledger(ctx, companyID, year)
	if err != nil {
		return out, err
	}
	cb, err := a.loadChart(ctx, companyID)
	if err != nil {
		return out, err
	}
	c := chart.Chart{Accounts: cb.Accounts}
	name := func(es []ledger.Entry) []ledger.Entry {
		for i := range es {
			for j := range es[i].Lines {
				if acc, ok := c.Find(es[i].Lines[j].Account); ok {
					es[i].Lines[j].Name = acc.Name
				}
			}
		}
		return es
	}
	closing, result := ledger.ClosingEntries(book.Accounts)
	out.Result = result
	out.Closing = append(out.Closing, name(closing)...)
	out.Opening = append(out.Opening, name(ledger.OpeningEntries(book.Accounts))...)
	out.ClosingDate = invoice.MustDate(fmt.Sprintf("%d-12-31", year))
	out.OpeningDate = invoice.MustDate(fmt.Sprintf("%d-01-01", year+1))
	out.Closed = a.YearClosed(ctx, companyID, year)
	return out, nil
}
