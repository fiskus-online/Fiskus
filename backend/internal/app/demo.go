package app

import (
	"context"
	"errors"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// Ukážková firma na vyskúšanie: na prázdnej inštalácii (aj vo Fiškuse
// v prehliadači) sa jedným tlačidlom založí ukážková firma s dokladmi
// a domácnosť Novákovci. Dáta pripraví balík demo, ktorý nastaví main
// (DemoSeed) — app ho neimportuje, aby nevznikol cyklus.

var (
	// ErrDemoOff — ukážka nie je v tejto inštalácii dostupná.
	ErrDemoOff = errors.New("ukážková firma nie je dostupná")
	// ErrDemoNotEmpty — ukážka sa dá načítať iba do prázdnej inštalácie.
	ErrDemoNotEmpty = errors.New("ukážkovú firmu možno načítať iba do prázdneho Fiškusa — vo Fiškuse už je firma alebo domácnosť")
)

// DemoAvailable: dá sa načítať ukážka (je nastavená a inštalácia je prázdna).
func (a *App) DemoAvailable(ctx context.Context) bool {
	if a.DemoSeed == nil {
		return false
	}
	n, err := a.Store.CountCompanies(ctx)
	return err == nil && n == 0
}

// LoadDemo založí ukážkovú firmu a domácnosť do prázdnej inštalácie.
func (a *App) LoadDemo(ctx context.Context) (store.Company, error) {
	if a.DemoSeed == nil {
		return store.Company{}, ErrDemoOff
	}
	if a.Caps.Mode == edition.Server && !auth.ActorFrom(ctx).SuperAdmin {
		return store.Company{}, ErrForbidden
	}
	n, err := a.Store.CountCompanies(ctx)
	if err != nil {
		return store.Company{}, err
	}
	if n > 0 {
		return store.Company{}, ErrDemoNotEmpty
	}
	c, err := a.DemoSeed(ctx, a)
	if err != nil {
		return store.Company{}, err
	}
	a.record(ctx, c.ID, audit.CatSettings, "demo", "company", c.ID, "Načítaná ukážková firma "+c.Name, nil, nil)
	return c, nil
}
