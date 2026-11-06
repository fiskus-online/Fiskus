package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/store/memstore"
)

type memBrowserDB struct{ staged []byte }

func (m *memBrowserDB) Export(context.Context) ([]byte, error) {
	return []byte("SQLite format 3\x00 kópia"), nil
}
func (m *memBrowserDB) StageRestore(_ context.Context, b []byte) error { m.staged = b; return nil }

func browserServer(t *testing.T, caps edition.Capabilities) (*httptest.Server, *memBrowserDB, int64) {
	t.Helper()
	a := app.New(memstore.New(), caps)
	db := &memBrowserDB{}
	a.Browser = db
	c, err := a.CreateCompany(context.Background(), store.Company{Name: "Moja firma"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer((&Server{App: a}).Handler())
	t.Cleanup(srv.Close)
	return srv, db, c.ID
}

func call(t *testing.T, srv *httptest.Server, method, path, ctype string, body io.Reader) (*http.Response, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, body)
	req.Header.Set(CSRFHeader, "1")
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	if out == nil {
		out = map[string]any{"raw": string(b)}
	}
	return res, out
}

// Každá adresa zo zoznamu installOnly existuje a v prehliadači vráti výzvu
// na inštaláciu; v desktopovej verzii nie.
func TestBrowserInstallOnly(t *testing.T) {
	browser, _, _ := browserServer(t, edition.ForBrowser())
	desk, _ := edition.For(edition.Desktop)
	desktop, _, _ := browserServer(t, desk)
	path := func(p string) string {
		p = strings.ReplaceAll(p, "{cid}", "1")
		for _, v := range []string{"{id}", "{name}", "{ico}", "{vatid}", "{msg}"} {
			p = strings.ReplaceAll(p, v, "1")
		}
		return p
	}
	for pattern, feature := range installOnly {
		method, p, _ := strings.Cut(pattern, " ")
		res, body := call(t, browser, method, path(p), "application/json", strings.NewReader("{}"))
		if res.StatusCode != http.StatusNotImplemented || body["install"] != feature {
			t.Errorf("prehliadač %s: %d %v", pattern, res.StatusCode, body)
		}
		res, body = call(t, desktop, method, path(p), "application/json", strings.NewReader("{}"))
		if body["install"] != nil {
			t.Errorf("desktop %s: %d %v", pattern, res.StatusCode, body)
		}
		if res.StatusCode == http.StatusNotFound && body["error"] == "neznáma adresa API" {
			t.Errorf("%s: adresa neexistuje", pattern)
		}
	}
	// bežné funkcie fungujú
	if res, _ := call(t, browser, "GET", "/api/companies/1/invoices", "", nil); res.StatusCode != 200 {
		t.Errorf("faktúry v prehliadači: %d", res.StatusCode)
	}
	res, body := call(t, browser, "GET", "/api/capabilities", "", nil)
	if res.StatusCode != 200 || body["browser"] != true || body["mode"] != "desktop" {
		t.Errorf("schopnosti: %v", body)
	}
	if res, _ := call(t, desktop, "GET", "/api/browser/backup", "", nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("desktop nemá zálohu prehliadača: %d", res.StatusCode)
	}
}

func TestBrowserBackupRestore(t *testing.T) {
	srv, db, _ := browserServer(t, edition.ForBrowser())
	res, body := call(t, srv, "GET", "/api/browser/backup", "", nil)
	if res.StatusCode != 200 || body["remind"] != true {
		t.Fatalf("stav: %d %v", res.StatusCode, body)
	}
	if res, _ := call(t, srv, "POST", "/api/browser/backup", "application/json", strings.NewReader(`{"password":"x"}`)); res.StatusCode != 422 {
		t.Fatalf("krátke heslo: %d", res.StatusCode)
	}
	req, _ := http.NewRequest("POST", srv.URL+"/api/browser/backup", strings.NewReader(`{"password":"heslo-zalohy-1"}`))
	req.Header.Set(CSRFHeader, "1")
	r2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	fbk, _ := io.ReadAll(r2.Body)
	r2.Body.Close()
	if r2.StatusCode != 200 || !strings.Contains(r2.Header.Get("Content-Disposition"), ".fbk") || len(fbk) == 0 {
		t.Fatalf("záloha: %d %s", r2.StatusCode, r2.Header.Get("Content-Disposition"))
	}
	if _, body := call(t, srv, "GET", "/api/browser/backup", "", nil); body["remind"] != false || body["last"] == nil {
		t.Fatalf("po zálohe: %v", body)
	}

	upload := func(password string) *http.Response {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", "fiskus.fbk")
		fw.Write(fbk)
		mw.WriteField("password", password)
		mw.Close()
		res, _ := call(t, srv, "POST", "/api/browser/restore", mw.FormDataContentType(), &buf)
		return res
	}
	if res := upload("zle-heslo-123"); res.StatusCode != 422 {
		t.Fatalf("zlé heslo: %d", res.StatusCode)
	}
	if res := upload("heslo-zalohy-1"); res.StatusCode != 204 || !bytes.HasPrefix(db.staged, []byte("SQLite format 3")) {
		t.Fatalf("obnova: %d", res.StatusCode)
	}
}
