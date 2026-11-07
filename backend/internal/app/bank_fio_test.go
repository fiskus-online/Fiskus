package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
)

const fioToken = "AbCdEf0123456789AbCdEf0123456789AbCdEf0123456789AbCdEf0123456789"

func TestFio(t *testing.T) {
	data, err := os.ReadFile("../bank/testdata/fio.json")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	var lastPath atomic.Value
	status := atomic.Int32{}
	status.Store(200)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		lastPath.Store(r.URL.Path)
		if s := int(status.Load()); s != 200 {
			w.WriteHeader(s)
			return
		}
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	a, c := newApp(t, edition.Desktop)
	a.FioURL = srv.URL + "/v1/rest"
	clock := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	a.Now = func() time.Time { return clock }

	if _, err := a.SaveFioAccount(ctx, c.ID, FioAccountInput{Token: "krátky"}); !fieldErr(err, "token") {
		t.Fatalf("zlý token: %v", err)
	}
	list, err := a.SaveFioAccount(ctx, c.ID, FioAccountInput{Label: "Fio – bežný účet", Token: fioToken})
	if err != nil || len(list) != 1 || !list[0].TokenSet {
		t.Fatalf("napojenie: %+v %v", list, err)
	}
	// token sa nikde nevracia
	if b, _ := json.Marshal(list); strings.Contains(string(b), fioToken) {
		t.Fatal("token vo výstupe")
	}
	if got, _ := a.FioAccounts(ctx, c.ID); strings.Contains(func() string { b, _ := json.Marshal(got); return string(b) }(), fioToken) {
		t.Fatal("token v zozname")
	}

	res, err := a.SyncFio(ctx, c.ID, list[0].ID)
	if err != nil || res.Imported != 3 || res.Format != "Fio API" || res.Account != "SK2683300000002801234567" {
		t.Fatalf("stiahnutie: %+v %v", res, err)
	}
	if p := lastPath.Load().(string); !strings.HasSuffix(p, "/periods/"+fioToken+"/2026-08-01/2026-10-29/transactions.json") {
		t.Fatalf("obdobie prvého stiahnutia: %s", p)
	}
	// limit 30 s: ďalšie volanie hneď sa ani nepošle
	if _, err := a.SyncFio(ctx, c.ID, list[0].ID); !fieldErr(err, "fio") || calls.Load() != 1 {
		t.Fatalf("limit 30 s: %v (%d volaní)", err, calls.Load())
	}
	clock = clock.Add(31 * time.Second)
	res, err = a.SyncFio(ctx, c.ID, list[0].ID)
	if err != nil || res.Imported != 0 || res.Duplicates != 3 {
		t.Fatalf("opakované stiahnutie: %+v %v", res, err)
	}
	if p := lastPath.Load().(string); !strings.Contains(p, "/2026-10-28/2026-10-29/") {
		t.Fatalf("obdobie od posledného dňa: %s", p)
	}
	acc, _ := a.FioAccounts(ctx, c.ID)
	if acc[0].Account != "SK2683300000002801234567" || acc[0].LastSync != "2026-10-29" {
		t.Fatalf("stav: %+v", acc[0])
	}

	// chyby banky: token sa neobjaví v hlásení
	for _, s := range []int{404, 409, 500, 502} {
		clock = clock.Add(31 * time.Second)
		status.Store(int32(s))
		_, err := a.SyncFio(ctx, c.ID, list[0].ID)
		if err == nil || strings.Contains(err.Error(), fioToken) {
			t.Fatalf("%d: %v", s, err)
		}
	}
	status.Store(200)
	srv.Close()
	clock = clock.Add(31 * time.Second)
	if _, err := a.SyncFio(ctx, c.ID, list[0].ID); err == nil || strings.Contains(err.Error(), fioToken) {
		t.Fatalf("nedostupný server: %v", err)
	}

	// úprava bez tokenu ponechá token, zmazanie
	if list, err = a.SaveFioAccount(ctx, c.ID, FioAccountInput{ID: list[0].ID, Label: "Fio"}); err != nil || !list[0].TokenSet || list[0].Label != "Fio" {
		t.Fatalf("úprava: %+v %v", list, err)
	}
	if list, err = a.DeleteFioAccount(ctx, c.ID, list[0].ID); err != nil || len(list) != 0 {
		t.Fatalf("zmazanie: %+v %v", list, err)
	}

	// v prehliadači nie
	b, bc := newApp(t, edition.Desktop)
	b.Caps.Browser = true
	if _, err := b.SaveFioAccount(ctx, bc.ID, FioAccountInput{Token: fioToken}); !fieldErr(err, "fio") {
		t.Fatalf("prehliadač: %v", err)
	}
}
