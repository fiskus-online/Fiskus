package app

import (
	"context"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/household"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// ReconcileInput je zostatok z výpisu banky k dátumu.
type ReconcileInput struct {
	Date    invoice.Date `json:"date"`
	Balance money.Dec    `json:"balance"`
	// Adjust: rozdiel zaúčtovať ako pohyb (Category, inak „Iné príjmy/výdavky“).
	Adjust   bool   `json:"adjust"`
	Category string `json:"category"`
}

// ReconcileResult je výsledok porovnania; Done = účet je odsúhlasený.
type ReconcileResult struct {
	household.Reconciliation
	Category string           `json:"category"` // kategória, do ktorej by šlo vyrovnanie
	Entry    *household.Entry `json:"entry,omitempty"`
	Done     bool             `json:"done"`
}

func (a *App) hhAccount(ctx context.Context, companyID, id int64) (household.Account, error) {
	accs, err := a.Store.ListHHAccounts(ctx, companyID)
	if err != nil {
		return household.Account{}, err
	}
	for _, x := range accs {
		if x.ID == id {
			return x, nil
		}
	}
	return household.Account{}, store.ErrNotFound
}

// ReconcileHouseholdAccount porovná zostatok účtu so zostatkom z výpisu.
// Bez commit iba vráti porovnanie. S commit pri zhode uloží odsúhlasenie;
// pri rozdiele ho uloží iba s Adjust — rozdiel zaúčtuje ako pohyb k dátumu.
func (a *App) ReconcileHouseholdAccount(ctx context.Context, companyID, accountID int64, in ReconcileInput, commit bool) (ReconcileResult, error) {
	perm := auth.PermLedgerRead
	if commit {
		perm = auth.PermLedgerWrite
	}
	if err := a.requireHousehold(ctx, companyID, perm); err != nil {
		return ReconcileResult{}, err
	}
	if in.Date.IsZero() {
		return ReconcileResult{}, vErr("date", "Zadajte dátum zostatku z výpisu")
	}
	if in.Date.After(invoice.Today()) {
		return ReconcileResult{}, vErr("date", "Dátum zostatku nemôže byť v budúcnosti")
	}
	acc, err := a.hhAccount(ctx, companyID, accountID)
	if err != nil {
		return ReconcileResult{}, err
	}
	es, err := a.Store.ListHHEntries(ctx, companyID, 0)
	if err != nil {
		return ReconcileResult{}, err
	}
	s, err := a.householdSettings(ctx, companyID)
	if err != nil {
		return ReconcileResult{}, err
	}
	res := ReconcileResult{Reconciliation: household.Reconcile(acc, es, in.Date, in.Balance)}
	income := res.Difference.Sign() > 0
	res.Category = in.Category
	if res.Category == "" {
		res.Category = household.AdjustCategory(s.Categories, income)
	}
	if !commit {
		return res, nil
	}
	if !res.Difference.IsZero() {
		if !in.Adjust {
			return res, vErr("balance", "Zostatok nesedí o "+res.Difference.Display()+" € — dohľadajte chýbajúci pohyb alebo rozdiel zaúčtujte")
		}
		e, err := a.SaveHouseholdEntry(ctx, companyID, household.Entry{
			Date: in.Date, AccountID: acc.ID, Amount: res.Difference, Category: res.Category,
			Payee: "Vyrovnanie podľa výpisu", Note: "Rozdiel pri odsúhlasení zostatku " + in.Balance.Display() + " €",
		})
		if err != nil {
			return res, err
		}
		res.Entry = &e
	}
	before := acc.Reconciled
	acc.Reconciled = &household.Reconciled{Date: in.Date, Balance: in.Balance}
	if err := a.Store.SaveHHAccount(ctx, companyID, &acc); err != nil {
		return res, err
	}
	a.record(ctx, companyID, audit.CatLedger, "reconcile", "household_account", acc.ID,
		"Domácnosť: odsúhlasený zostatok "+acc.Name+" k "+in.Date.Display()+" "+in.Balance.Display()+" €", before, acc.Reconciled)
	res.Done = true
	return res, nil
}
