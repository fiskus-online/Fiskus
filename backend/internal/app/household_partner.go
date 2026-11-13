package app

import (
	"context"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/household"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// hhPartners vráti adresár domácnosti na párovanie pohybov s partnermi.
func (a *App) hhPartners(ctx context.Context, companyID int64) ([]household.Partner, error) {
	cs, err := a.Store.ListContacts(ctx, companyID, "")
	if err != nil {
		return nil, err
	}
	ps := make([]household.Partner, 0, len(cs))
	for _, c := range cs {
		ps = append(ps, partnerOf(c))
	}
	return ps, nil
}

// partnerOf je kontakt adresára ako partner domácnosti (so všetkými účtami).
func partnerOf(c store.Contact) household.Partner {
	p := household.Partner{ID: c.ID, Name: c.Name, IBAN: c.IBAN}
	for _, x := range c.Accounts {
		p.IBANs = append(p.IBANs, x.IBAN)
	}
	return p
}

// contactAddress je adresa kontaktu v jednom riadku (ulica, PSČ obec).
func contactAddress(c store.Contact) string {
	city := strings.TrimSpace(strings.TrimSpace(c.Zip) + " " + strings.TrimSpace(c.City))
	return strings.Join(nonEmpty(strings.TrimSpace(c.Street), city), ", ")
}

// HouseholdPartner sú platby a záväzky domácnosti s jedným partnerom z adresára.
type HouseholdPartner struct {
	household.PartnerTotals
	Entries     []household.Entry `json:"entries"`
	Obligations []ObligationInfo  `json:"obligations"`
}

// HouseholdPartnerActivity vráti pohyby a záväzky domácnosti s kontaktom
// z adresára: s väzbou na kontakt aj staršie s rovnakým menom príjemcu.
func (a *App) HouseholdPartnerActivity(ctx context.Context, companyID, contactID int64) (HouseholdPartner, error) {
	out := HouseholdPartner{Entries: []household.Entry{}, Obligations: []ObligationInfo{}}
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerRead); err != nil {
		return out, err
	}
	c, err := a.Store.GetContact(ctx, companyID, contactID)
	if err != nil {
		return out, err
	}
	p := partnerOf(c)
	es, err := a.Store.ListHHEntries(ctx, companyID, 0)
	if err != nil {
		return out, err
	}
	today := a.Today()
	mine, totals := household.PartnerEntries(es, p, today.Year())
	out.PartnerTotals = totals
	if mine != nil {
		out.Entries = mine
	}
	os, err := a.Store.ListHHObligations(ctx, companyID)
	if err != nil {
		return out, err
	}
	for _, o := range os {
		if o.OfPartner(p) {
			out.Obligations = append(out.Obligations, ObligationInfo{Obligation: o, State: household.State(o, es, today)})
		}
	}
	return out, nil
}
