package app

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/household"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// maxAssets je najviac položiek majetku v domácnosti.
const maxAssets = 200

func assetsKey(companyID int64) string {
	return "household.assets." + strconv.FormatInt(companyID, 10)
}

func (a *App) loadAssets(ctx context.Context, companyID int64) ([]household.Asset, error) {
	raw, err := a.Store.GetSetting(ctx, assetsKey(companyID))
	if errors.Is(err, store.ErrNotFound) || (err == nil && raw == "") {
		return []household.Asset{}, nil
	}
	if err != nil {
		return nil, err
	}
	var out []household.Asset
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (a *App) storeAssets(ctx context.Context, companyID int64, xs []household.Asset) error {
	data, err := json.Marshal(xs)
	if err != nil {
		return err
	}
	return a.Store.SetSetting(ctx, assetsKey(companyID), string(data))
}

// HouseholdAsset je majetok s hodnotou k dnešku.
type HouseholdAsset struct {
	household.Asset
	Value money.Dec `json:"value"`
}

// HouseholdAssets vráti majetok domácnosti.
func (a *App) HouseholdAssets(ctx context.Context, companyID int64) ([]HouseholdAsset, error) {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerRead); err != nil {
		return nil, err
	}
	xs, err := a.loadAssets(ctx, companyID)
	if err != nil {
		return nil, err
	}
	today := a.Today()
	out := make([]HouseholdAsset, 0, len(xs))
	for _, x := range xs {
		out = append(out, HouseholdAsset{Asset: x, Value: x.ValueAt(today)})
	}
	return out, nil
}

// SaveHouseholdAsset pridá (ID 0) alebo upraví majetok.
func (a *App) SaveHouseholdAsset(ctx context.Context, companyID int64, x household.Asset) (household.Asset, error) {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return x, err
	}
	x.Normalize()
	if err := x.Validate(); err != nil {
		return x, err
	}
	xs, err := a.loadAssets(ctx, companyID)
	if err != nil {
		return x, err
	}
	var before *household.Asset
	next := int64(1)
	for i := range xs {
		if xs[i].ID >= next {
			next = xs[i].ID + 1
		}
		if xs[i].ID == x.ID && x.ID != 0 {
			b := xs[i]
			before = &b
			xs[i] = x
		}
	}
	switch {
	case x.ID != 0 && before == nil:
		return x, store.ErrNotFound
	case x.ID == 0 && len(xs) >= maxAssets:
		return x, vErr("name", "Majetku môže byť najviac "+strconv.Itoa(maxAssets)+" položiek")
	case x.ID == 0:
		x.ID = next
		xs = append(xs, x)
	}
	if err := a.storeAssets(ctx, companyID, xs); err != nil {
		return x, err
	}
	action := "create"
	if before != nil {
		action = "update"
	}
	a.record(ctx, companyID, audit.CatLedger, action, "household_asset", x.ID, "Domácnosť: majetok "+x.Name, before, x)
	return x, nil
}

// DeleteHouseholdAsset zmaže majetok (predaný majetok radšej s hodnotou 0,
// aby ostal v histórii).
func (a *App) DeleteHouseholdAsset(ctx context.Context, companyID, id int64) error {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return err
	}
	xs, err := a.loadAssets(ctx, companyID)
	if err != nil {
		return err
	}
	for i, x := range xs {
		if x.ID == id {
			if err := a.storeAssets(ctx, companyID, append(xs[:i:i], xs[i+1:]...)); err != nil {
				return err
			}
			a.record(ctx, companyID, audit.CatLedger, "delete", "household_asset", id, "Domácnosť: zmazaný majetok "+x.Name, x, nil)
			return nil
		}
	}
	return store.ErrNotFound
}

// DebtLine je záväzok alebo pohľadávka v prehľade čistého majetku.
type DebtLine struct {
	ID   int64     `json:"id"`
	Name string    `json:"name"`
	Lent bool      `json:"lent"` // požičané niekomu (pohľadávka)
	Owed money.Dec `json:"owed"`
}

// NetWorthReport je čistý majetok dnes, jeho vývoj po mesiacoch a rozpis.
type NetWorthReport struct {
	Now      household.NetWorth   `json:"now"`
	History  []household.NetWorth `json:"history"` // konce mesiacov + dnešok
	Accounts []HouseholdAccount   `json:"accounts"`
	Assets   []HouseholdAsset     `json:"assets"`
	Debts    []DebtLine           `json:"debts"`
}

// HouseholdNetWorth spočíta čistý majetok domácnosti a jeho vývoj za
// posledných months mesiacov.
func (a *App) HouseholdNetWorth(ctx context.Context, companyID int64, months int) (NetWorthReport, error) {
	var r NetWorthReport
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerRead); err != nil {
		return r, err
	}
	if months < 1 || months > 120 {
		months = 12
	}
	accs, err := a.Store.ListHHAccounts(ctx, companyID)
	if err != nil {
		return r, err
	}
	es, err := a.Store.ListHHEntries(ctx, companyID, 0)
	if err != nil {
		return r, err
	}
	obls, err := a.Store.ListHHObligations(ctx, companyID)
	if err != nil {
		return r, err
	}
	assets, err := a.loadAssets(ctx, companyID)
	if err != nil {
		return r, err
	}
	today := a.Today()
	for _, d := range household.MonthEnds(today, months) {
		r.History = append(r.History, household.NetWorthAt(accs, es, assets, obls, d))
	}
	r.Now = r.History[len(r.History)-1]
	if r.Accounts, err = a.HouseholdAccounts(ctx, companyID); err != nil {
		return r, err
	}
	if r.Assets, err = a.HouseholdAssets(ctx, companyID); err != nil {
		return r, err
	}
	r.Debts = []DebtLine{}
	for _, o := range obls {
		if owed := household.OwedAt(o, es, today); owed.Sign() > 0 {
			r.Debts = append(r.Debts, DebtLine{ID: o.ID, Name: o.Name, Lent: o.Kind == household.Lent, Owed: owed})
		}
	}
	return r, nil
}
