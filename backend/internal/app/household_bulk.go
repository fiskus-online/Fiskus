package app

import (
	"context"
	"errors"

	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/household"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
)

// maxBulk je najviac pohybov v jednej hromadnej úprave.
const maxBulk = 5000

// BulkInput je hromadná úprava označených pohybov: zmena (kategória, účet)
// alebo zmazanie.
type BulkInput struct {
	IDs    []int64 `json:"ids"`
	Delete bool    `json:"delete"`
	household.BulkChange
}

// BulkSkip je pohyb, ktorý sa nezmenil, s dôvodom.
type BulkSkip struct {
	ID     int64  `json:"id"`
	What   string `json:"what"` // dátum, kategória, suma, obchod
	Reason string `json:"reason"`
}

// BulkResult je výsledok hromadnej úpravy.
type BulkResult struct {
	Changed int        `json:"changed"`
	Skipped []BulkSkip `json:"skipped"`
}

// BulkHouseholdEntries zmení alebo zmaže označené pohyby. Každý pohyb sa
// ukladá ako pri bežnej úprave (kontroly, audit); pohyb, ktorý sa zmeniť
// nedá, sa preskočí a vráti s dôvodom.
func (a *App) BulkHouseholdEntries(ctx context.Context, companyID int64, in BulkInput) (BulkResult, error) {
	res := BulkResult{Skipped: []BulkSkip{}}
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return res, err
	}
	if len(in.IDs) == 0 {
		return res, vErr("ids", "Označte aspoň jeden pohyb")
	}
	if len(in.IDs) > maxBulk {
		return res, vErr("ids", "Naraz sa dá upraviť najviac 5000 pohybov")
	}
	if !in.Delete && in.Category == "" && in.AccountID == 0 {
		return res, vErr("category", "Vyberte kategóriu alebo účet")
	}
	if in.AccountID != 0 {
		accs, err := a.Store.ListHHAccounts(ctx, companyID)
		if err != nil {
			return res, err
		}
		found := false
		for _, acc := range accs {
			if acc.ID == in.AccountID {
				found = true
				if acc.Closed {
					return res, vErr("accountId", "Účet „"+acc.Name+"“ je zrušený")
				}
			}
		}
		if !found {
			return res, vErr("accountId", "Účet neexistuje")
		}
	}
	s, err := a.householdSettings(ctx, companyID)
	if err != nil {
		return res, err
	}
	seen := map[int64]bool{}
	for _, id := range in.IDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		e, err := a.Store.GetHHEntry(ctx, companyID, id)
		if err != nil {
			return res, err // neznámy pohyb (iná firma) — nič ďalšie
		}
		if in.Delete {
			if err := a.DeleteHouseholdEntry(ctx, companyID, id); err != nil {
				return res, err
			}
			res.Changed++
			continue
		}
		next, reason := household.BulkApply(e, in.BulkChange, s.Categories)
		if reason == "" && next.Category == e.Category && next.AccountID == e.AccountID {
			continue // už je taký
		}
		if reason == "" {
			if _, err := a.SaveHouseholdEntry(ctx, companyID, next); err != nil {
				var ve *invoice.ValidationError
				if !errors.As(err, &ve) || len(ve.Fields) == 0 {
					return res, err
				}
				reason = ve.Fields[0].Message
			}
		}
		if reason != "" {
			res.Skipped = append(res.Skipped, BulkSkip{ID: id, What: entrySummary(e), Reason: reason})
			continue
		}
		res.Changed++
	}
	return res, nil
}
