package app

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/relay"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// Telefón mimo domu: Fiškus v prehliadači telefónu (app.fiskus.online) má
// kópiu údajov z počítača na čítanie a nové záznamy (pohyb, kontakt, koncept
// faktúry…) zapisuje do fronty. Počítač je hlavný: frontu si vyzdvihne zo
// schránky (internal/relay), záznamy prehrá cez svoje API s právami
// spárovaného telefónu a pošle telefónu čerstvú kópiu. Čísla dokladov
// prideľuje iba počítač, takže nevzniknú duplicity.
//
// Schránka vidí iba zašifrované údaje; kľúč je v odkaze z párovacieho QR
// kódu (relay.PairLink).

const (
	syncDesktopKey    = "sync.desktop"  // spárované telefóny (s tajomstvami — do kópie nejde)
	syncAppliedPrefix = "sync.applied." // + kanál: výsledky prehratých záznamov (ide do kópie)
	// SyncSnapshotKey je verzia kópie zapísaná do kópie pre telefón.
	SyncSnapshotKey = "sync.snapshot"
	syncPhoneKey    = "phone" // stav telefónu (v samostatnom úložisku, nie v kópii)
	maxSyncPhones   = 5
	syncAppliedKeep = 300
	// SyncAppURL je Fiškus v prehliadači, ktorý telefón otvorí z QR kódu.
	SyncAppURL = "https://app.fiskus.online"
	// syncUploadEvery: zmenené údaje sa telefónu pošlú najviac takto často
	// (hneď, keď si telefón kópiu vypýta alebo počítač prehrá jeho záznamy).
	syncUploadEvery = 30 * time.Minute
)

// Chyby synchronizácie.
var (
	ErrPhoneCopy  = errors.New("na telefóne je kópia údajov z počítača — toto urobte v počítači")
	ErrSyncOff    = errors.New("telefón mimo domu je iba v nainštalovanom Fiškuse (počítač) a vo Fiškuse v prehliadači (telefón)")
	ErrSyncPhones = fmt.Errorf("spárovať sa dá najviac %d telefónov — odoberte nepoužívaný", maxSyncPhones)
	ErrSyncNewer  = errors.New("údaje z počítača sú z novšej verzie Fiškusa — telefón ich načíta po ďalšej aktualizácii Fiškusa v prehliadači")
)

// syncWrites: čo smie telefón zapísať (vzor cesty API → druh vytvoreného
// záznamu). Iba nové záznamy a úhrady existujúcich — úpravy a mazanie
// robí počítač. Druh slúži na prepojenie: keď telefón vytvorí kontakt
// a hneď ho použije v pohybe, počítač v pohybe použije svoje id kontaktu.
var syncWrites = map[string]string{
	"POST /api/companies/{cid}/contacts":                       "contacts",
	"POST /api/companies/{cid}/household/entries":              "household/entries",
	"POST /api/companies/{cid}/household/obligations/{id}/pay": "",
	"POST /api/companies/{cid}/cash":                           "cash",
	"POST /api/companies/{cid}/invoices":                       "invoices",
	"POST /api/companies/{cid}/purchases":                      "purchases",
	"POST /api/companies/{cid}/quotes":                         "quotes",
	"POST /api/companies/{cid}/trips":                          "trips",
	"POST /api/companies/{cid}/calendar/own":                   "calendar",
	"POST /api/companies/{cid}/calendar/own/{id}/done":         "",
}

// syncTitles: popis záznamu vo fronte telefónu.
var syncTitles = map[string]string{
	"contacts":          "Nový kontakt",
	"household/entries": "Pohyb v domácnosti",
	"cash":              "Pokladničný doklad",
	"invoices":          "Koncept faktúry",
	"purchases":         "Prijatá faktúra",
	"quotes":            "Cenová ponuka",
	"trips":             "Jazda",
	"calendar":          "Udalosť v kalendári",
}

// syncRefs: polia (na najvyššej úrovni tela), ktoré môžu odkazovať na
// záznam vytvorený na telefóne → jeho druh. Odkaz sa vo fronte zapíše ako
// id záznamu fronty (SyncItem.Refs), nie ako číslo — číslo z kópie na
// telefóne môže v počítači patriť inému záznamu.
var syncRefs = map[string]string{"contactId": "contacts"}

// SyncWritable: smie telefón zapisovať cez tento vzor cesty (a aký záznam vznikne)?
func SyncWritable(pattern string) (kind string, ok bool) {
	kind, ok = syncWrites[pattern]
	return kind, ok
}

// SyncLocalOnly: zápis, ktorý nemení údaje (náhľad, kalkulačka, záloha,
// samotná synchronizácia) — telefón ho vybaví sám a do fronty nejde.
func SyncLocalOnly(pattern string) bool {
	_, path, _ := strings.Cut(pattern, " ")
	return strings.HasSuffix(path, "/preview") || strings.HasPrefix(path, "/api/calc/") ||
		path == "/api/browser/backup" || strings.HasPrefix(path, "/api/sync/")
}

// ReplayFunc vykoná požiadavku na API v mene actora (nastavuje api.Server).
type ReplayFunc func(ctx context.Context, actor auth.Actor, method, path string, body []byte) (status int, resp []byte)

// SyncSource vytvorí kópiu databázy pre telefón (bez hesiel a kľúčov,
// s verziou v nastavení SyncSnapshotKey) — nastavuje cmd/fiskus.
type SyncSource interface {
	PhoneSnapshot(ctx context.Context, version int64) ([]byte, error)
	// Changes rastie s každou zmenou údajov (stačí v rámci behu programu).
	Changes(ctx context.Context) (int64, error)
}

// SyncState je úložisko stavu telefónu mimo hlavnej databázy (hlavnú
// databázu nahrádza kópia z počítača) — nastavuje cmd/fiskus-web.
type SyncState interface {
	Get(ctx context.Context, key string) (string, error) // store.ErrNotFound, keď nie je
	Set(ctx context.Context, key, value string) error
}

// SyncItem je záznam z telefónu (zašifrovaný ide cez schránku).
type SyncItem struct {
	ID      string          `json:"id"`
	At      string          `json:"at"`
	Method  string          `json:"method"`
	Path    string          `json:"path"`
	Body    json.RawMessage `json:"body,omitempty"`
	Kind    string          `json:"kind,omitempty"`    // druh vytvoreného záznamu
	LocalID int64           `json:"localId,omitempty"` // id záznamu v kópii na telefóne
	Title   string          `json:"title"`
	// Refs: pole tela → id záznamu fronty, ktorý odkazovaný záznam vytvoril.
	Refs map[string]string `json:"refs,omitempty"`
}

// SyncResult je výsledok prehratia záznamu v počítači.
type SyncResult struct {
	ID      string `json:"id"`
	Status  string `json:"status"` // ok | error
	Error   string `json:"error,omitempty"`
	Kind    string `json:"kind,omitempty"`
	LocalID int64  `json:"localId,omitempty"`
	NewID   int64  `json:"newId,omitempty"`
	At      string `json:"at"`
	Title   string `json:"title"`
}

// SyncPhoneInfo je spárovaný telefón (v počítači, bez tajomstva).
type SyncPhoneInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Created    string `json:"created"`
	LastUpload string `json:"lastUpload,omitempty"` // posledná kópia poslaná telefónu
	LastItem   string `json:"lastItem,omitempty"`   // posledný prijatý záznam
	Received   int    `json:"received"`             // prijatých záznamov
	LastError  string `json:"lastError,omitempty"`
}

type syncPhone struct {
	SyncPhoneInfo
	Secret string `json:"secret"`
}

type syncDesktop struct {
	Relay  string      `json:"relay,omitempty"` // vlastná schránka ("" = predvolená)
	Phones []syncPhone `json:"phones"`
}

// SyncPhoneStatus je stav na telefóne.
type SyncPhoneStatus struct {
	Desktop   string        `json:"desktop"`             // meno počítača
	Version   string        `json:"version,omitempty"`   // kópia, ktorú telefón používa (čas)
	Staged    bool          `json:"staged"`              // nová kópia čaká na načítanie
	Pending   int           `json:"pending"`             // záznamov čaká na prenos do počítača
	Failed    []SyncResult  `json:"failed"`              // záznamy, ktoré počítač neprijal
	Items     []SyncItemRow `json:"items"`               // čakajúce záznamy
	LastCheck string        `json:"lastCheck,omitempty"` // posledné spojenie so schránkou
	LastError string        `json:"lastError,omitempty"`
}

// SyncItemRow je čakajúci záznam na telefóne.
type SyncItemRow struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	At     string `json:"at"`
	Status string `json:"status"` // local (ešte neodoslaný) | sent (v schránke)
}

// SyncStatus je stav synchronizácie pre rozhranie.
type SyncStatus struct {
	Role         string           `json:"role"` // desktop | phone | "" (nedostupné)
	Relay        string           `json:"relay"`
	DefaultRelay string           `json:"defaultRelay"`
	Phones       []SyncPhoneInfo  `json:"phones,omitempty"`
	Phone        *SyncPhoneStatus `json:"phone,omitempty"`
}

// SyncPairing je odkaz pre nový telefón (do QR kódu).
type SyncPairing struct {
	Link  string        `json:"link"`
	Phone SyncPhoneInfo `json:"phone"`
}

type phoneItem struct {
	SyncItem
	Status string `json:"status"` // local | sent
}

type phoneState struct {
	Secret    string       `json:"secret"`
	Desktop   string       `json:"desktop"`
	Relay     string       `json:"relay,omitempty"`
	Joined    string       `json:"joined"`
	Version   int64        `json:"version"`          // kópia v hlavnej databáze
	Staged    int64        `json:"staged,omitempty"` // pripravená na načítanie
	Outbox    []phoneItem  `json:"outbox"`
	Failed    []SyncResult `json:"failed"`
	LastCheck string       `json:"lastCheck,omitempty"`
	LastError string       `json:"lastError,omitempty"`
	Wanted    string       `json:"wanted,omitempty"`
}

type syncMem struct {
	mu      sync.Mutex
	run     sync.Mutex // jeden priebeh synchronizácie naraz
	phone   *bool      // je toto telefón s kópiou? (nil = ešte nezistené)
	changes map[string]int64
	upload  map[string]time.Time
	fast    time.Time // po spárovaní sa schránka kontroluje častejšie
	kick    chan struct{}
}

// SyncRole: desktop (nainštalovaný Fiškus), phone (Fiškus v prehliadači)
// alebo "" (server — telefón tam ide priamo cez https).
func (a *App) SyncRole() string {
	switch {
	case a.Browser != nil && a.SyncState != nil:
		return "phone"
	case a.Caps.Mode == edition.Desktop && !a.Caps.Browser && a.SyncSource != nil:
		return "desktop"
	}
	return ""
}

func (a *App) relayURL(custom string) string {
	if custom != "" {
		return custom
	}
	if a.SyncRelayURL != "" {
		return a.SyncRelayURL
	}
	return relay.DefaultURL
}

func (a *App) relayClient(secretB64, custom string) (relay.Client, error) {
	secret, err := base64.RawURLEncoding.DecodeString(secretB64)
	if err != nil {
		return relay.Client{}, err
	}
	k, err := relay.Derive(secret)
	if err != nil {
		return relay.Client{}, err
	}
	return relay.Client{Base: a.relayURL(custom), Keys: k, HTTP: a.SyncHTTP}, nil
}

// SyncKick spustí synchronizáciu hneď (napr. po zázname na telefóne).
func (a *App) SyncKick() {
	a.syncMem.mu.Lock()
	if a.syncMem.kick == nil {
		a.syncMem.kick = make(chan struct{}, 1)
	}
	ch := a.syncMem.kick
	a.syncMem.mu.Unlock()
	select {
	case ch <- struct{}{}:
	default:
	}
}

// SyncLoop synchronizuje, kým beží program: počítač každých 10 minút
// (prvých 10 minút po spárovaní každých 15 sekúnd), telefón každé 2 minúty
// a hneď po novom zázname.
func (a *App) SyncLoop(ctx context.Context) {
	a.SyncKick()
	a.syncMem.mu.Lock()
	kick := a.syncMem.kick
	a.syncMem.mu.Unlock()
	for {
		wait := 10 * time.Minute
		if a.SyncRole() == "phone" {
			wait = 2 * time.Minute
		}
		a.syncMem.mu.Lock()
		if a.now().Before(a.syncMem.fast) {
			wait = 15 * time.Second
		}
		a.syncMem.mu.Unlock()
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-kick:
			t.Stop()
			time.Sleep(2 * time.Second) // viac záznamov za sebou naraz
		case <-t.C:
		}
		if err := a.SyncRun(auth.WithActor(ctx, auth.System)); err != nil && !errors.Is(err, ErrSyncOff) {
			log.Printf("telefón mimo domu: %v", err)
		}
	}
}

// SyncRun vykoná jeden priebeh synchronizácie.
func (a *App) SyncRun(ctx context.Context) error {
	switch a.SyncRole() {
	case "desktop":
		return a.desktopSync(ctx)
	case "phone":
		return a.phoneSync(ctx)
	}
	return ErrSyncOff
}

// SyncStatus vráti stav synchronizácie.
func (a *App) SyncStatus(ctx context.Context) (SyncStatus, error) {
	st := SyncStatus{Role: a.SyncRole(), DefaultRelay: a.relayURL("")}
	switch st.Role {
	case "desktop":
		if err := requireSuperAdmin(ctx); err != nil {
			return st, err
		}
		c, err := a.loadSyncDesktop(ctx)
		if err != nil {
			return st, err
		}
		st.Relay = c.Relay
		st.Phones = []SyncPhoneInfo{}
		for _, p := range c.Phones {
			st.Phones = append(st.Phones, p.SyncPhoneInfo)
		}
	case "phone":
		ps, ok, err := a.loadPhone(ctx)
		if err != nil || !ok {
			return st, err
		}
		st.Relay = ps.Relay
		st.Phone = ps.status()
	}
	return st, nil
}

func (ps phoneState) status() *SyncPhoneStatus {
	out := &SyncPhoneStatus{Desktop: ps.Desktop, Staged: ps.Staged != 0, Failed: ps.Failed, Items: []SyncItemRow{},
		LastCheck: ps.LastCheck, LastError: ps.LastError}
	if out.Failed == nil {
		out.Failed = []SyncResult{}
	}
	if ps.Version > 0 {
		out.Version = time.UnixMilli(ps.Version).UTC().Format(time.RFC3339)
	}
	for _, it := range ps.Outbox {
		out.Pending++
		out.Items = append(out.Items, SyncItemRow{ID: it.ID, Title: it.Title, At: it.At, Status: it.Status})
	}
	return out
}

// ---- počítač ----

func (a *App) loadSyncDesktop(ctx context.Context) (syncDesktop, error) {
	var c syncDesktop
	v, err := a.Store.GetSetting(ctx, syncDesktopKey)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return c, nil
	case err != nil:
		return c, err
	}
	err = json.Unmarshal([]byte(v), &c)
	return c, err
}

func (a *App) saveSyncDesktop(ctx context.Context, c syncDesktop) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return a.Store.SetSetting(ctx, syncDesktopKey, string(b))
}

func (a *App) requireSyncDesktop(ctx context.Context) error {
	if a.SyncRole() != "desktop" {
		return ErrSyncOff
	}
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	if auth.ActorFrom(ctx).Mobile {
		return ErrForbidden // telefón nespravuje párovanie
	}
	return nil
}

// SetSyncRelay nastaví vlastnú schránku ("" = schránka Fiškusa). Spárované
// telefóny treba spárovať znova (odkaz nesie adresu schránky).
func (a *App) SetSyncRelay(ctx context.Context, url string) (SyncStatus, error) {
	if err := a.requireSyncDesktop(ctx); err != nil {
		return SyncStatus{}, err
	}
	url = strings.TrimRight(strings.TrimSpace(url), "/")
	if url != "" {
		if err := relay.ValidURL(url); err != nil {
			return SyncStatus{}, mfaFieldErr("relay", err.Error())
		}
	}
	c, err := a.loadSyncDesktop(ctx)
	if err != nil {
		return SyncStatus{}, err
	}
	c.Relay = url
	if err := a.saveSyncDesktop(ctx, c); err != nil {
		return SyncStatus{}, err
	}
	a.record(ctx, 0, audit.CatSettings, "update", "sync", 0, "Schránka pre telefón: "+a.relayURL(url), nil, nil)
	return a.SyncStatus(ctx)
}

// SyncPair spáruje nový telefón: vytvorí tajomstvo, pošle do schránky prvú
// kópiu údajov a vráti odkaz do QR kódu.
func (a *App) SyncPair(ctx context.Context, name string) (SyncPairing, error) {
	if err := a.requireSyncDesktop(ctx); err != nil {
		return SyncPairing{}, err
	}
	c, err := a.loadSyncDesktop(ctx)
	if err != nil {
		return SyncPairing{}, err
	}
	if len(c.Phones) >= maxSyncPhones {
		return SyncPairing{}, mfaFieldErr("phones", ErrSyncPhones.Error())
	}
	name = strings.TrimSpace(name)
	if r := []rune(name); len(r) > 40 {
		name = string(r[:40])
	}
	if name == "" {
		name = "Telefón"
	}
	secret, err := relay.NewSecret()
	if err != nil {
		return SyncPairing{}, err
	}
	id := make([]byte, 6)
	_, _ = rand.Read(id)
	p := syncPhone{SyncPhoneInfo: SyncPhoneInfo{ID: hex.EncodeToString(id), Name: name, Created: a.now().UTC().Format(time.RFC3339)},
		Secret: base64.RawURLEncoding.EncodeToString(secret)}
	c.Phones = append(c.Phones, p)
	if err := a.saveSyncDesktop(ctx, c); err != nil {
		return SyncPairing{}, err
	}
	a.record(ctx, 0, audit.CatSettings, "create", "sync_phone", 0, "Spárovaný telefón mimo domu: "+name, nil, nil)
	a.syncMem.mu.Lock()
	a.syncMem.fast = a.now().Add(10 * time.Minute)
	a.syncMem.mu.Unlock()
	// prvá kópia hneď, aby ju telefón po naskenovaní QR kódu našiel
	a.syncMem.run.Lock()
	err = a.uploadPhone(ctx, &c, len(c.Phones)-1)
	a.syncMem.run.Unlock()
	if err != nil {
		c.Phones[len(c.Phones)-1].LastError = err.Error()
	}
	_ = a.saveSyncDesktop(ctx, c)
	link := relay.PairLink(a.syncAppURL(), secret, a.desktopName(ctx), c.Relay)
	a.SyncKick()
	return SyncPairing{Link: link, Phone: c.Phones[len(c.Phones)-1].SyncPhoneInfo}, nil
}

func (a *App) syncAppURL() string {
	if a.SyncApp != "" {
		return a.SyncApp
	}
	return SyncAppURL
}

// desktopName: meno počítača v telefóne (meno počítača v systéme, inak
// prvá firma alebo domácnosť).
func (a *App) desktopName(ctx context.Context) string {
	if a.SyncDesktopName != "" {
		return a.SyncDesktopName
	}
	if cos, err := a.Store.ListCompanies(ctx); err == nil && len(cos) > 0 {
		return cos[0].Name
	}
	return "Počítač"
}

// SyncRemovePhone odoberie telefón a zmaže jeho schránku.
func (a *App) SyncRemovePhone(ctx context.Context, id string) (SyncStatus, error) {
	if err := a.requireSyncDesktop(ctx); err != nil {
		return SyncStatus{}, err
	}
	c, err := a.loadSyncDesktop(ctx)
	if err != nil {
		return SyncStatus{}, err
	}
	out := c.Phones[:0]
	var removed *syncPhone
	for _, p := range c.Phones {
		if p.ID == id {
			p := p
			removed = &p
			continue
		}
		out = append(out, p)
	}
	if removed == nil {
		return SyncStatus{}, store.ErrNotFound
	}
	c.Phones = out
	if err := a.saveSyncDesktop(ctx, c); err != nil {
		return SyncStatus{}, err
	}
	if cl, err := a.relayClient(removed.Secret, c.Relay); err == nil {
		_ = cl.Wipe(ctx) // bez internetu schránka zanikne sama po 60 dňoch
	}
	a.record(ctx, 0, audit.CatSettings, "delete", "sync_phone", 0, "Odobraný telefón mimo domu: "+removed.Name, nil, nil)
	return a.SyncStatus(ctx)
}

// desktopSync: pre každý telefón prehrá záznamy zo schránky a pošle kópiu,
// keď si ju telefón pýta, keď prišli záznamy, alebo keď sa údaje zmenili.
func (a *App) desktopSync(ctx context.Context) error {
	a.syncMem.run.Lock()
	defer a.syncMem.run.Unlock()
	c, err := a.loadSyncDesktop(ctx)
	if err != nil || len(c.Phones) == 0 {
		return err
	}
	var errs []error
	for i := range c.Phones {
		if err := a.desktopSyncPhone(ctx, &c, i); err != nil {
			c.Phones[i].LastError = err.Error()
			errs = append(errs, fmt.Errorf("%s: %w", c.Phones[i].Name, err))
		}
	}
	if err := a.saveSyncDesktop(ctx, c); err != nil {
		return err
	}
	a.noteChanges(ctx, c)
	return errors.Join(errs...)
}

// noteChanges: stav zmien po vlastnom zápise synchronizácie (aby sa za zmenu
// nepovažoval).
func (a *App) noteChanges(ctx context.Context, c syncDesktop) {
	n, err := a.SyncSource.Changes(ctx)
	if err != nil {
		return
	}
	a.syncMem.mu.Lock()
	defer a.syncMem.mu.Unlock()
	if a.syncMem.changes == nil {
		a.syncMem.changes = map[string]int64{}
	}
	for _, p := range c.Phones {
		if _, ok := a.syncMem.upload[p.ID]; ok {
			a.syncMem.changes[p.ID] = n
		}
	}
}

func (a *App) desktopSyncPhone(ctx context.Context, c *syncDesktop, i int) error {
	p := &c.Phones[i]
	cl, err := a.relayClient(p.Secret, c.Relay)
	if err != nil {
		return err
	}
	st, err := cl.State(ctx)
	if err != nil {
		return err
	}
	p.LastError = ""
	applied := 0
	if len(st.Inbox) > 0 {
		results, err := a.loadApplied(ctx, cl.Keys.Channel)
		if err != nil {
			return err
		}
		done := map[string]bool{}
		for _, r := range results {
			done[r.ID] = true
		}
		ids := map[string]int64{} // id záznamu fronty → id vytvoreného záznamu v počítači
		for _, r := range results {
			if r.Status == "ok" && r.NewID > 0 {
				ids[r.ID] = r.NewID
			}
		}
		actor := auth.Local
		actor.Name, actor.Mobile = "Telefón mimo domu: "+p.Name, true
		for _, in := range st.Inbox {
			sealed, err := cl.GetItem(ctx, in.ID)
			if err != nil {
				return err
			}
			var it SyncItem
			plain, err := cl.Keys.Open(relay.KindItem, sealed)
			if err == nil {
				err = json.Unmarshal(plain, &it)
			}
			switch {
			case err != nil:
				log.Printf("telefón mimo domu %s: záznam %s sa nedá prečítať: %v", p.Name, in.ID, err)
			case done[it.ID]:
				// už prehratý (schránka ho nestihla zmazať)
			default:
				r := a.replayItem(ctx, actor, it, ids)
				results = append(results, r)
				done[it.ID] = true
				applied++
				p.Received++
				p.LastItem = a.now().UTC().Format(time.RFC3339)
				if err := a.saveApplied(ctx, cl.Keys.Channel, results); err != nil {
					return err
				}
			}
			if err := cl.DeleteItem(ctx, in.ID); err != nil {
				return err
			}
		}
	}
	a.syncMem.mu.Lock()
	last, uploaded := a.syncMem.upload[p.ID]
	prev, known := a.syncMem.changes[p.ID]
	a.syncMem.mu.Unlock()
	changed := true
	if n, err := a.SyncSource.Changes(ctx); err == nil && known {
		changed = n != prev
	}
	due := applied > 0 || st.Want != "" || st.Snapshot == nil || !uploaded ||
		changed && a.now().Sub(last) >= syncUploadEvery
	if !due {
		return nil
	}
	return a.uploadPhone(ctx, c, i)
}

func (a *App) loadApplied(ctx context.Context, channel string) ([]SyncResult, error) {
	var out []SyncResult
	v, err := a.Store.GetSetting(ctx, syncAppliedPrefix+channel)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return out, nil
	case err != nil:
		return nil, err
	}
	return out, json.Unmarshal([]byte(v), &out)
}

func (a *App) saveApplied(ctx context.Context, channel string, rs []SyncResult) error {
	if len(rs) > syncAppliedKeep {
		rs = rs[len(rs)-syncAppliedKeep:]
	}
	b, err := json.Marshal(rs)
	if err != nil {
		return err
	}
	return a.Store.SetSetting(ctx, syncAppliedPrefix+channel, string(b))
}

// uploadPhone pošle telefónu čerstvú kópiu údajov.
func (a *App) uploadPhone(ctx context.Context, c *syncDesktop, i int) error {
	p := &c.Phones[i]
	cl, err := a.relayClient(p.Secret, c.Relay)
	if err != nil {
		return err
	}
	version := a.now().UnixMilli()
	db, err := a.SyncSource.PhoneSnapshot(ctx, version)
	if err != nil {
		return fmt.Errorf("kópia údajov: %w", err)
	}
	var zb bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&zb, gzip.BestCompression)
	if _, err := zw.Write(db); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	sealed, err := cl.Keys.Seal(relay.KindSnapshot, zb.Bytes())
	if err != nil {
		return err
	}
	if err := cl.PutSnapshot(ctx, version, sealed); err != nil {
		return err
	}
	p.LastUpload, p.LastError = a.now().UTC().Format(time.RFC3339), ""
	a.syncMem.mu.Lock()
	if a.syncMem.upload == nil {
		a.syncMem.upload = map[string]time.Time{}
	}
	a.syncMem.upload[p.ID] = a.now()
	a.syncMem.mu.Unlock()
	return nil
}

// replayItem prehrá záznam z telefónu cez API (s kontrolou, či je povolený).
// ids: id záznamu fronty → id záznamu, ktorý vytvoril (doplní sa).
func (a *App) replayItem(ctx context.Context, actor auth.Actor, it SyncItem, ids map[string]int64) SyncResult {
	r := SyncResult{ID: it.ID, Kind: it.Kind, LocalID: it.LocalID, At: a.now().UTC().Format(time.RFC3339), Title: it.Title}
	fail := func(msg string) SyncResult {
		r.Status, r.Error = "error", msg
		return r
	}
	if a.Replay == nil || a.SyncPattern == nil {
		return fail("program nevie prehrať záznam")
	}
	pattern := a.SyncPattern(it.Method, it.Path)
	kind, ok := SyncWritable(pattern)
	if !ok || kind != it.Kind {
		return fail("tento záznam sa z telefónu nedá zapísať")
	}
	body, err := remapRefs(it.Body, it.Refs, ids)
	if err != nil {
		return fail(err.Error())
	}
	status, resp := a.Replay(ctx, actor, it.Method, it.Path, body)
	if status < 200 || status >= 300 {
		return fail(apiError(status, resp))
	}
	r.Status = "ok"
	if kind != "" {
		var out struct {
			ID int64 `json:"id"`
		}
		if json.Unmarshal(resp, &out) == nil && out.ID > 0 {
			r.NewID = out.ID
			ids[it.ID] = out.ID
		}
	}
	return r
}

// apiError: text chyby z odpovede API (aj s chybami polí).
func apiError(status int, resp []byte) string {
	var e struct {
		Error  string `json:"error"`
		Fields []struct {
			Message string `json:"message"`
		} `json:"fields"`
	}
	_ = json.Unmarshal(resp, &e)
	msg := e.Error
	if len(e.Fields) > 0 {
		parts := []string{}
		for _, f := range e.Fields {
			parts = append(parts, f.Message)
		}
		msg = strings.TrimSpace(msg + ": " + strings.Join(parts, "; "))
	}
	if msg == "" {
		msg = http.StatusText(status)
	}
	return msg
}

// remapRefs doplní do tela id záznamov, na ktoré záznam odkazuje cez Refs
// (vytvorené skôr na telefóne).
func remapRefs(body json.RawMessage, refs map[string]string, ids map[string]int64) (json.RawMessage, error) {
	if len(refs) == 0 {
		return body, nil
	}
	var v map[string]json.RawMessage
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, errors.New("záznam je poškodený")
	}
	for field, item := range refs {
		id, ok := ids[item]
		if !ok {
			return nil, errors.New("odkazuje na záznam z telefónu, ktorý počítač neprijal")
		}
		v[field] = json.RawMessage(strconv.FormatInt(id, 10))
	}
	return json.Marshal(v)
}

// findRefs: odkazy tela na záznamy, ktoré telefón vytvoril a počítač ich
// ešte neprijal (podľa ich id v kópii na telefóne).
func findRefs(body json.RawMessage, outbox []phoneItem) map[string]string {
	var v map[string]json.RawMessage
	if json.Unmarshal(body, &v) != nil {
		return nil
	}
	var refs map[string]string
	for field, kind := range syncRefs {
		raw, ok := v[field]
		if !ok {
			continue
		}
		id, err := strconv.ParseInt(string(raw), 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		for _, it := range outbox {
			if it.Kind == kind && it.LocalID == id {
				if refs == nil {
					refs = map[string]string{}
				}
				refs[field] = it.ID
			}
		}
	}
	return refs
}

// ---- telefón ----

func (a *App) loadPhone(ctx context.Context) (phoneState, bool, error) {
	var ps phoneState
	if a.SyncState == nil {
		return ps, false, nil
	}
	v, err := a.SyncState.Get(ctx, syncPhoneKey)
	switch {
	case errors.Is(err, store.ErrNotFound) || err == nil && v == "":
		return ps, false, nil
	case err != nil:
		return ps, false, err
	}
	if err := json.Unmarshal([]byte(v), &ps); err != nil {
		return ps, false, err
	}
	return ps, ps.Secret != "", nil
}

func (a *App) savePhone(ctx context.Context, ps phoneState) error {
	b, err := json.Marshal(ps)
	if err != nil {
		return err
	}
	return a.SyncState.Set(ctx, syncPhoneKey, string(b))
}

// SyncIsPhone: je tento Fiškus v prehliadači kópiou z počítača? (Vtedy
// smie zapisovať iba záznamy zo syncWrites.)
func (a *App) SyncIsPhone(ctx context.Context) bool {
	if a.SyncRole() != "phone" {
		return false
	}
	a.syncMem.mu.Lock()
	known := a.syncMem.phone
	a.syncMem.mu.Unlock()
	if known != nil {
		return *known
	}
	_, ok, err := a.loadPhone(ctx)
	if err != nil {
		return true // radšej iba na čítanie
	}
	a.setPhone(ok)
	return ok
}

func (a *App) setPhone(ok bool) {
	a.syncMem.mu.Lock()
	a.syncMem.phone = &ok
	a.syncMem.mu.Unlock()
}

// SyncJoin pripojí Fiškus v prehliadači k počítaču podľa odkazu z QR kódu.
// Údaje v prehliadači nahradí kópia z počítača (pri prvej synchronizácii).
func (a *App) SyncJoin(ctx context.Context, fragment string) (SyncStatus, error) {
	if a.SyncRole() != "phone" {
		return SyncStatus{}, ErrSyncOff
	}
	p, err := relay.ParsePairing(fragment)
	if err != nil {
		return SyncStatus{}, mfaFieldErr("link", err.Error())
	}
	ps := phoneState{Secret: base64.RawURLEncoding.EncodeToString(p.Secret), Desktop: p.Name, Relay: p.Relay,
		Joined: a.now().UTC().Format(time.RFC3339), Outbox: []phoneItem{}, Failed: []SyncResult{}}
	if old, ok, _ := a.loadPhone(ctx); ok && old.Secret == ps.Secret {
		return a.SyncStatus(ctx) // ten istý odkaz znova
	}
	if ps.Desktop == "" {
		ps.Desktop = "Počítač"
	}
	if err := a.savePhone(ctx, ps); err != nil {
		return SyncStatus{}, err
	}
	a.setPhone(true)
	a.SyncKick()
	return a.SyncStatus(ctx)
}

// SyncLeave odpojí telefón od počítača: údaje v prehliadači ostanú ako
// samostatné (už sa nebudú obnovovať), čakajúce záznamy sa zahodia.
func (a *App) SyncLeave(ctx context.Context) error {
	if a.SyncRole() != "phone" {
		return ErrSyncOff
	}
	if err := a.SyncState.Set(ctx, syncPhoneKey, ""); err != nil {
		return err
	}
	a.setPhone(false)
	return nil
}

// SyncCapture zapíše do fronty záznam, ktorý telefón práve uložil vo svojej
// kópii (volá API po úspešnom zápise).
func (a *App) SyncCapture(ctx context.Context, pattern, method, path string, body, resp []byte) error {
	kind, ok := SyncWritable(pattern)
	if !ok {
		return nil
	}
	ps, joined, err := a.loadPhone(ctx)
	if err != nil || !joined {
		return err
	}
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	it := SyncItem{ID: hex.EncodeToString(id), At: a.now().UTC().Format(time.RFC3339), Method: method, Path: path, Kind: kind, Title: syncTitle(kind, pattern, body)}
	if len(bytes.TrimSpace(body)) > 0 {
		if !json.Valid(body) {
			return errors.New("záznam nie je JSON")
		}
		it.Body = json.RawMessage(body)
		it.Refs = findRefs(it.Body, ps.Outbox)
	}
	if kind != "" {
		var out struct {
			ID int64 `json:"id"`
		}
		if json.Unmarshal(resp, &out) == nil {
			it.LocalID = out.ID
		}
	}
	ps.Outbox = append(ps.Outbox, phoneItem{SyncItem: it, Status: "local"})
	if err := a.savePhone(ctx, ps); err != nil {
		return err
	}
	a.SyncKick()
	return nil
}

// syncTitle: popis záznamu vo fronte, s menom alebo sumou z tela, ak sú.
func syncTitle(kind, pattern string, body []byte) string {
	var b struct {
		Name   string `json:"name"`
		Payee  string `json:"payee"`
		Title  string `json:"title"`
		Amount string `json:"amount"`
	}
	_ = json.Unmarshal(body, &b)
	detail := strings.TrimSpace(strings.Join(slicesCompact([]string{b.Name, b.Payee, b.Title}), ", "))
	if b.Amount != "" {
		detail = strings.TrimSpace(detail + " " + strings.Replace(b.Amount, ".", ",", 1) + " €")
	}
	if r := []rune(detail); len(r) > 60 {
		detail = string(r[:60]) + "…"
	}
	base := "Záznam"
	if t, ok := syncTitles[kind]; ok {
		base = t
	} else if strings.HasSuffix(pattern, "/pay") {
		base = "Úhrada záväzku"
	} else if strings.HasSuffix(pattern, "/done") {
		base = "Splnená udalosť"
	}
	if detail != "" {
		return base + ": " + detail
	}
	return base
}

func slicesCompact(in []string) []string {
	out := in[:0]
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// SyncPhoneStart: po načítaní stránky na telefóne — ak sa práve načítala
// nová kópia z počítača, odškrtne záznamy, ktoré počítač už prijal,
// a ostatné zapíše znova do novej kópie (aby ich používateľ videl).
func (a *App) SyncPhoneStart(ctx context.Context) error {
	ps, ok, err := a.loadPhone(ctx)
	if err != nil || !ok {
		a.setPhone(ok && err == nil)
		return err
	}
	a.setPhone(true)
	v, err := a.Store.GetSetting(ctx, SyncSnapshotKey)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	version, _ := strconv.ParseInt(v, 10, 64)
	if ps.Staged != 0 && version != ps.Staged {
		ps.LastError = "nová kópia z počítača sa nenačítala — skúsi sa znova"
	}
	ps.Staged = 0
	if version == ps.Version || version == 0 {
		return a.savePhone(ctx, ps)
	}
	ps.Version = version
	keys, err := a.phoneKeys(ps)
	if err != nil {
		return err
	}
	results, err := a.loadApplied(ctx, keys.Channel)
	if err != nil {
		return err
	}
	byID := map[string]SyncResult{}
	for _, r := range results {
		byID[r.ID] = r
	}
	// záznamy, ktoré počítač ešte neprijal, sa zapíšu do novej kópie znova
	// (dostanú nové id; odkazy medzi nimi idú cez Refs ako v počítači)
	keep := ps.Outbox[:0]
	ids := map[string]int64{}
	actor := auth.Local
	actor.Name = "Telefón"
	for _, it := range ps.Outbox {
		if r, done := byID[it.ID]; done {
			if r.Status == "error" {
				ps.Failed = append(ps.Failed, r)
			}
			continue
		}
		if r := a.replayItem(ctx, actor, it.SyncItem, ids); r.Status == "ok" && r.NewID > 0 {
			it.LocalID = r.NewID // podľa neho sa nájdu odkazy nových záznamov
		}
		keep = append(keep, it)
	}
	ps.Outbox = keep
	if len(ps.Failed) > 50 {
		ps.Failed = ps.Failed[len(ps.Failed)-50:]
	}
	return a.savePhone(ctx, ps)
}

// SyncDismiss skryje záznam, ktorý počítač neprijal.
func (a *App) SyncDismiss(ctx context.Context, id string) (SyncStatus, error) {
	ps, ok, err := a.loadPhone(ctx)
	if err != nil || !ok {
		return SyncStatus{}, ErrSyncOff
	}
	out := ps.Failed[:0]
	for _, r := range ps.Failed {
		if r.ID != id {
			out = append(out, r)
		}
	}
	ps.Failed = out
	if err := a.savePhone(ctx, ps); err != nil {
		return SyncStatus{}, err
	}
	return a.SyncStatus(ctx)
}

func (a *App) phoneKeys(ps phoneState) (relay.Keys, error) {
	secret, err := base64.RawURLEncoding.DecodeString(ps.Secret)
	if err != nil {
		return relay.Keys{}, err
	}
	return relay.Derive(secret)
}

// phoneSync: odošle čakajúce záznamy, stiahne novšiu kópiu (pripraví ju na
// načítanie) a keď je kópia stará, vypýta si čerstvú.
func (a *App) phoneSync(ctx context.Context) error {
	a.syncMem.run.Lock()
	defer a.syncMem.run.Unlock()
	ps, ok, err := a.loadPhone(ctx)
	if err != nil || !ok {
		return err
	}
	cl, err := a.relayClient(ps.Secret, ps.Relay)
	if err != nil {
		return err
	}
	runErr := a.phoneSyncOnce(ctx, cl, &ps)
	ps.LastCheck = a.now().UTC().Format(time.RFC3339)
	ps.LastError = ""
	if runErr != nil {
		ps.LastError = runErr.Error()
	}
	// stav sa mohol medzitým zmeniť novým záznamom — zlúčiť frontu
	cur, ok, err := a.loadPhone(ctx)
	if err == nil && ok && cur.Secret == ps.Secret {
		sent := map[string]string{}
		for _, it := range ps.Outbox {
			sent[it.ID] = it.Status
		}
		for i, it := range cur.Outbox {
			if s, ok := sent[it.ID]; ok {
				cur.Outbox[i].Status = s
			}
		}
		ps.Outbox = cur.Outbox
	} else if err == nil && !ok {
		return runErr // medzitým odpojený
	}
	if err := a.savePhone(ctx, ps); err != nil {
		return err
	}
	return runErr
}

func (a *App) phoneSyncOnce(ctx context.Context, cl relay.Client, ps *phoneState) error {
	for i := range ps.Outbox {
		it := &ps.Outbox[i]
		if it.Status != "local" {
			continue
		}
		plain, err := json.Marshal(it.SyncItem)
		if err != nil {
			return err
		}
		sealed, err := cl.Keys.Seal(relay.KindItem, plain)
		if err != nil {
			return err
		}
		if _, err := cl.PostItem(ctx, sealed); err != nil {
			return err
		}
		it.Status = "sent"
	}
	st, err := cl.State(ctx)
	if err != nil {
		return err
	}
	newest := max(ps.Version, ps.Staged)
	if st.Snapshot != nil && st.Snapshot.Version > newest {
		sealed, version, err := cl.GetSnapshot(ctx)
		if err != nil {
			return err
		}
		if version <= 0 {
			version = st.Snapshot.Version // hlavička verzie chýba (napr. proxy)
		}
		zipped, err := cl.Keys.Open(relay.KindSnapshot, sealed)
		if err != nil {
			return err
		}
		zr, err := gzip.NewReader(bytes.NewReader(zipped))
		if err != nil {
			return relay.ErrSealed
		}
		db, err := io.ReadAll(io.LimitReader(zr, 512<<20))
		if err != nil {
			return relay.ErrSealed
		}
		if err := a.Browser.StageRestore(ctx, db); err != nil {
			if errors.Is(err, store.ErrNewerSchema) {
				return ErrSyncNewer
			}
			return fmt.Errorf("kópia z počítača: %w", err)
		}
		ps.Staged = version
		return nil
	}
	// kópia chýba alebo je staršia ako 10 minút — vypýtať si čerstvú
	// (najviac raz za 10 minút)
	stale := st.Snapshot == nil || a.now().Sub(time.UnixMilli(st.Snapshot.Version)) > 10*time.Minute
	wanted, _ := time.Parse(time.RFC3339, ps.Wanted)
	if stale && st.Want == "" && a.now().Sub(wanted) > 10*time.Minute {
		if err := cl.Want(ctx); err != nil {
			return err
		}
		ps.Wanted = a.now().UTC().Format(time.RFC3339)
	}
	return nil
}
