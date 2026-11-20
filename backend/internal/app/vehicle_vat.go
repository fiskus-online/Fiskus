package app

import (
	"context"
	"errors"

	"github.com/fiskus-online/fiskus/backend/internal/asset"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/logbook"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/store"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

// vehicleAcquisition zistí údaje o nadobudnutí vozidla z registra majetku:
// prvotné použitie (zaradenie), predaj (vyradenie predajom) a daň z prijatej
// faktúry (ak ju vozidlo nemá zadanú ručne).
func (a *App) vehicleAcquisition(ctx context.Context, companyID int64, v logbook.Vehicle) (logbook.Acquisition, error) {
	var acq logbook.Acquisition
	if v.AssetID == 0 {
		return acq, nil
	}
	x, err := a.Store.GetAsset(ctx, companyID, v.AssetID)
	if errors.Is(err, store.ErrNotFound) {
		return acq, nil
	}
	if err != nil {
		return acq, err
	}
	acq.FirstUse, acq.Rate, acq.VAT = x.Acquired, 23, v.AcquiredVAT
	if x.DisposalKind == asset.DisposalSale {
		acq.Sold = x.Disposed
	}
	if x.PurchaseID != 0 {
		p, err := a.Store.GetPurchase(ctx, companyID, x.PurchaseID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return acq, err
		}
		if err == nil {
			t, _ := p.Tax()
			sum, top := money.Zero, money.Zero
			for _, r := range t.Rates {
				sum = sum.Add(r.VAT)
				if r.VAT.Cmp(top) > 0 {
					top, acq.Rate = r.VAT, vat.Rate(r.Rate)
				}
			}
			if acq.VAT.IsZero() && !p.ReverseCharge {
				acq.VAT = sum
			}
		}
	}
	return acq, nil
}

// VehicleVATAdjustments vráti úpravy odpočítanej dane pri osobných
// vozidlách s 50 % odpočtom za rok (year 0 = všetky roky).
func (a *App) VehicleVATAdjustments(ctx context.Context, companyID int64, year int) ([]logbook.VATAdjustment, error) {
	if err := a.Require(ctx, companyID, auth.PermVATRead); err != nil {
		return nil, err
	}
	vs, err := a.Store.ListVehicles(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := []logbook.VATAdjustment{}
	for _, v := range vs {
		acq, err := a.vehicleAcquisition(ctx, companyID, v)
		if err != nil {
			return nil, err
		}
		for _, x := range v.VATAdjustments(acq) {
			if year == 0 || x.Year == year {
				out = append(out, x)
			}
		}
	}
	return out, nil
}
