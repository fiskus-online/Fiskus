//go:build js && wasm && sqlite

// Fiškus v prehliadači: celý program (API) skompilovaný do WebAssembly.
// Beží vo web workeri frontendu (frontend/src/browser/worker.js) spolu so
// SQLite vo WebAssembly; databáza je v úložisku prehliadača (OPFS) na
// počítači používateľa. Rozhranie sa pýta rovnakého API ako pri nainštalovanom
// programe — požiadavky naň presmeruje service worker.
//
//	GOOS=js GOARCH=wasm go build -tags sqlite -o fiskus.wasm ./cmd/fiskus-web
//
// Funkcie, ktoré potrebujú inštaláciu (iné služby, e-maily, mobil v sieti…),
// API odmietne s výzvou na inštaláciu (internal/api/browser.go).
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"syscall/js"
	"time"

	"github.com/fiskus-online/fiskus/backend/internal/api"
	"github.com/fiskus-online/fiskus/backend/internal/app"
	"github.com/fiskus-online/fiskus/backend/internal/demo"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/store/sqlite"
)

// version sa nastaví pri zostavení: -ldflags "-X main.version=1.0.0"
var version = "dev"

const (
	dbFile      = "/fiskus.db"
	snapshot    = "/kopia.db"
	restoreFile = "/obnova.db" // worker ju pri ďalšom štarte presunie na dbFile
)

func main() {
	bridge := js.Global().Get("fiskusSQL")
	st, err := sqlite.Open(dbFile)
	if err != nil {
		fatal(fmt.Errorf("databáza: %w", err))
		return
	}
	a := app.New(st, edition.ForBrowser())
	a.Browser = browserDB{st: st, bridge: bridge}
	a.BackupsOff = "Fiškus v prehliadači sa zálohuje stiahnutím súboru zálohy."
	a.DemoSeed = demo.Seed // ukážková firma na vyskúšanie
	if err := a.LoadLegal(context.Background()); err != nil {
		fatal(fmt.Errorf("legislatíva: %w", err))
		return
	}
	h := (&api.Server{App: a, Version: version}).Handler()
	jar := &cookieJar{v: map[string]string{}}
	js.Global().Set("fiskusServe", js.FuncOf(func(_ js.Value, args []js.Value) any {
		return serve(h, jar, args[0])
	}))
	go recurringLoop(a)
	js.Global().Call("fiskusReady", version)
	select {} // program beží, kým je otvorená stránka
}

func fatal(err error) {
	log.Print(err)
	js.Global().Call("fiskusFailed", err.Error())
}

// serve vybaví požiadavku {method, url, headers: [[k, v]…], body: Uint8Array|null}
// a vráti Promise s {status, headers, body}.
func serve(h http.Handler, jar *cookieJar, req js.Value) any {
	method := req.Get("method").String()
	url := req.Get("url").String()
	var body []byte
	if b := req.Get("body"); !b.IsNull() && !b.IsUndefined() {
		body = make([]byte, b.Get("byteLength").Int())
		js.CopyBytesToGo(body, js.Global().Get("Uint8Array").New(b))
	}
	hdr := http.Header{}
	hs := req.Get("headers")
	for i := 0; i < hs.Length(); i++ {
		hdr.Add(hs.Index(i).Index(0).String(), hs.Index(i).Index(1).String())
	}
	return newPromise(func() (js.Value, error) {
		r := httptest.NewRequest(method, url, bytes.NewReader(body))
		r.Header = hdr
		r.RemoteAddr = "127.0.0.1:0"
		jar.add(r)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		res := w.Result()
		jar.save(res)
		out, _ := io.ReadAll(res.Body)
		headers := js.Global().Get("Array").New()
		for k, vs := range res.Header {
			if strings.EqualFold(k, "Set-Cookie") {
				continue // relácie drží jar (prehliadač ich zo service workera neprijme)
			}
			for _, v := range vs {
				headers.Call("push", js.ValueOf([]any{k, v}))
			}
		}
		b := js.Global().Get("Uint8Array").New(len(out))
		js.CopyBytesToJS(b, out)
		o := js.Global().Get("Object").New()
		o.Set("status", res.StatusCode)
		o.Set("headers", headers)
		o.Set("body", b.Get("buffer"))
		return o, nil
	})
}

// newPromise spustí f v gorutine (môže čakať na JavaScript) a vráti Promise.
func newPromise(f func() (js.Value, error)) js.Value {
	var handler js.Func
	handler = js.FuncOf(func(_ js.Value, args []js.Value) any {
		resolve, reject := args[0], args[1]
		go func() {
			defer handler.Release()
			v, err := f()
			if err != nil {
				reject.Invoke(js.Global().Get("Error").New(err.Error()))
				return
			}
			resolve.Invoke(v)
		}()
		return nil
	})
	return js.Global().Get("Promise").New(handler)
}

// await počká na Promise z JavaScriptu.
func await(p js.Value) (js.Value, error) {
	type result struct {
		v   js.Value
		err error
	}
	ch := make(chan result, 1)
	ok := js.FuncOf(func(_ js.Value, args []js.Value) any {
		v := js.Undefined()
		if len(args) > 0 {
			v = args[0]
		}
		ch <- result{v: v}
		return nil
	})
	bad := js.FuncOf(func(_ js.Value, args []js.Value) any {
		msg := "chyba"
		if len(args) > 0 {
			msg = args[0].Get("message").String()
		}
		ch <- result{err: errors.New(msg)}
		return nil
	})
	defer ok.Release()
	defer bad.Release()
	p.Call("then", ok, bad)
	r := <-ch
	return r.v, r.err
}

// cookieJar drží cookies relácie (napr. odomknutie PIN-om) počas behu stránky.
type cookieJar struct {
	mu sync.Mutex
	v  map[string]string
}

func (j *cookieJar) add(r *http.Request) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for k, v := range j.v {
		r.AddCookie(&http.Cookie{Name: k, Value: v})
	}
}

func (j *cookieJar) save(res *http.Response) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, c := range res.Cookies() {
		if c.MaxAge < 0 || c.Value == "" {
			delete(j.v, c.Name)
		} else {
			j.v[c.Name] = c.Value
		}
	}
}

// browserDB: databáza v úložisku prehliadača (pozri app.BrowserDB).
type browserDB struct {
	st     *sqlite.Store
	bridge js.Value
}

func (b browserDB) call(name string, args ...any) (v js.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	return b.bridge.Call(name, args...), nil
}

// Export: konzistentná kópia cez VACUUM INTO (počká na prebiehajúcu transakciu).
func (b browserDB) Export(ctx context.Context) ([]byte, error) {
	_, _ = b.call("removeFile", snapshot)
	defer b.call("removeFile", snapshot)
	if err := b.st.Snapshot(ctx, snapshot); err != nil {
		return nil, err
	}
	v, err := b.call("readFile", snapshot)
	if err != nil {
		return nil, err
	}
	out := make([]byte, v.Length())
	js.CopyBytesToGo(out, v)
	return out, nil
}

// StageRestore zapíše databázu vedľa aktuálnej a overí ju; pri ďalšom
// štarte ju worker presunie na miesto aktuálnej.
func (b browserDB) StageRestore(ctx context.Context, db []byte) error {
	arr := js.Global().Get("Uint8Array").New(len(db))
	js.CopyBytesToJS(arr, db)
	p, err := b.call("writeFile", restoreFile, arr)
	if err != nil {
		return err
	}
	if _, err := await(p); err != nil {
		return err
	}
	if err := (&sqlite.Store{}).Verify(ctx, restoreFile); err != nil {
		_, _ = b.call("removeFile", restoreFile)
		return err
	}
	return nil
}

// recurringLoop vytvára pravidelné faktúry, kým je Fiškus otvorený.
func recurringLoop(a *app.App) {
	ctx := context.Background()
	t := time.NewTimer(5 * time.Second)
	for range t.C {
		if res, err := a.RunRecurring(ctx); err != nil {
			log.Printf("pravidelná fakturácia: %v", err)
		} else if res.Created > 0 {
			log.Printf("pravidelná fakturácia: vytvorených %d faktúr", res.Created)
		}
		t.Reset(time.Hour)
	}
}
