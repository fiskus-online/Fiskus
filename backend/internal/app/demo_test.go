package app

import (
	"context"
	"errors"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/store"
	"github.com/fiskus-online/fiskus/backend/internal/store/memstore"
)

func TestLoadDemo(t *testing.T) {
	seed := func(ctx context.Context, a *App) (store.Company, error) {
		c, err := a.CreateCompany(ctx, store.Company{Name: "Demo elektroservis"})
		if err != nil {
			return c, err
		}
		_, err = a.CreateCompany(ctx, store.Company{Name: "Domácnosť Novákovci", Bookkeeping: store.BookkeepingHousehold})
		return c, err
	}
	fresh := func(mode edition.Mode, withSeed bool) *App {
		caps, _ := edition.For(mode)
		a := New(memstore.New(), caps)
		if withSeed {
			a.DemoSeed = seed
		}
		return a
	}
	local := func(*App) context.Context { return ctx }
	admin := func(a *App) context.Context {
		if _, err := a.EnsureAdmin(ctx, "sef@kancelaria.sk", "Šéf", "silne-heslo-123"); err != nil {
			t.Fatal(err)
		}
		u, _ := a.Store.GetUserByEmail(ctx, "sef@kancelaria.sk")
		return auth.WithActor(ctx, auth.Actor{UserID: u.ID, Name: u.Name, SuperAdmin: true})
	}
	user := func(*App) context.Context { return auth.WithActor(ctx, auth.Actor{UserID: 2}) }
	tests := []struct {
		name      string
		app       func() *App
		ctx       func(*App) context.Context
		available bool
		err       error
	}{
		{"desktop prázdny", func() *App { return fresh(edition.Desktop, true) }, local, true, nil},
		{"bez ukážky", func() *App { return fresh(edition.Desktop, false) }, local, false, ErrDemoOff},
		{"už je firma", func() *App {
			a := fresh(edition.Desktop, true)
			_, _ = a.CreateCompany(ctx, store.Company{Name: "Moja firma"})
			return a
		}, local, false, ErrDemoNotEmpty},
		{"server bez správcu", func() *App { return fresh(edition.Server, true) }, user, true, ErrForbidden},
		{"server správca", func() *App { return fresh(edition.Server, true) }, admin, true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := tt.app()
			c0 := tt.ctx(a)
			if got := a.DemoAvailable(ctx); got != tt.available {
				t.Fatalf("dostupná %v", got)
			}
			c, err := a.LoadDemo(c0)
			if !errors.Is(err, tt.err) {
				t.Fatalf("chyba %v, chcem %v", err, tt.err)
			}
			if err == nil {
				if cs, _ := a.Store.ListCompanies(ctx); len(cs) != 2 || c.Name != "Demo elektroservis" || a.DemoAvailable(ctx) {
					t.Fatalf("po načítaní: %+v %+v", c, cs)
				}
				if _, err := a.LoadDemo(c0); !errors.Is(err, ErrDemoNotEmpty) {
					t.Fatalf("druhýkrát: %v", err)
				}
			}
		})
	}
}
