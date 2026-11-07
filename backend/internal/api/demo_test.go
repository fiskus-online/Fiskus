package api

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/store/memstore"
)

func TestDemo(t *testing.T) {
	caps, _ := edition.For(edition.Desktop)
	a := app.New(memstore.New(), caps)
	srv := httptest.NewServer((&Server{App: a}).Handler())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	e := &env{t: t, srv: srv, app: a, client: &http.Client{Jar: jar}}

	var c struct {
		Demo bool `json:"demo"`
	}
	e.do("GET", "/api/capabilities", nil, 200, &c)
	if c.Demo {
		t.Fatal("bez DemoSeed sa ukážka ponúknuť nemá")
	}
	e.do("POST", "/api/demo", "{}", 404, nil)

	a.DemoSeed = func(ctx context.Context, a *app.App) (store.Company, error) {
		return a.CreateCompany(ctx, store.Company{Name: "Demo elektroservis"})
	}
	e.do("GET", "/api/capabilities", nil, 200, &c)
	if !c.Demo {
		t.Fatal("prázdny Fiškus má ponúknuť ukážku")
	}
	var got store.Company
	e.do("POST", "/api/demo", "{}", 201, &got)
	if got.Name != "Demo elektroservis" {
		t.Fatalf("firma %+v", got)
	}
	c.Demo = false
	e.do("GET", "/api/capabilities", nil, 200, &c)
	if c.Demo {
		t.Fatal("po načítaní sa ukážka už neponúka")
	}
	e.do("POST", "/api/demo", "{}", 409, nil)
}
