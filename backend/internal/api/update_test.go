package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/app"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/store"
	"github.com/fiskus-online/fiskus/backend/internal/store/memstore"
	"github.com/fiskus-online/fiskus/backend/internal/update"
)

func TestUpdateAPI(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v0.8.0", "body": "novinky", "assets": []any{}})
	}))
	defer gh.Close()
	caps, _ := edition.For(edition.Desktop)
	a := app.New(memstore.New(), caps)
	c, _ := a.CreateCompany(context.Background(), store.Company{Name: "Firma"})
	m := &update.Manager{Version: "0.7.0", URL: gh.URL, Client: http.DefaultClient, GOOS: "linux", GOARCH: "amd64",
		Target: update.Target{Kind: update.Binary, Exe: filepath.Join(t.TempDir(), "fiskus")}, Dir: t.TempDir()}
	m.Init()
	srv := httptest.NewServer((&Server{App: a, Update: m, Version: "0.7.0"}).Handler())
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	e := &env{t: t, srv: srv, cid: c.ID, app: a, client: &http.Client{Jar: jar}}

	var capsOut struct{ Update bool }
	e.do("GET", "/api/capabilities", nil, 200, &capsOut)
	if !capsOut.Update {
		t.Fatal("capabilities bez aktualizácie")
	}
	var st update.Status
	e.do("GET", "/api/update", nil, 200, &st)
	if st.Current != "0.7.0" || !st.Auto || st.State != update.Idle {
		t.Fatalf("stav: %+v", st)
	}
	e.do("POST", "/api/update/check", nil, 200, &st)
	if st.State != update.Available || st.Latest != "0.8.0" || st.Manual == "" {
		t.Fatalf("kontrola: %+v", st) // vo vydaní chýba súbor → ručne
	}
	e.do("POST", "/api/update/start", nil, 409, nil)
	e.do("PUT", "/api/update/auto", map[string]bool{"auto": false}, 200, &st)
	if st.Auto {
		t.Fatal("vypnutie")
	}
	e.do("POST", "/api/update/postpone", nil, 200, nil)
	e.noCSRF = true
	e.do("POST", "/api/update/postpone", nil, 403, nil)

	// bez aktualizácie (server, prehliadač) adresy nie sú
	plain := setup(t, edition.Desktop)
	plain.do("GET", "/api/capabilities", nil, 200, &capsOut)
	if capsOut.Update {
		t.Fatal("aktualizácia bez správcu")
	}
	plain.do("GET", "/api/update", nil, 404, nil)
}
