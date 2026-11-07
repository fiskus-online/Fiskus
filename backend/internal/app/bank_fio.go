package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/bank"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// Napojenie na Fio banku cez jej API: token z internetbankingu (Nastavenia →
// API) dovolí iba čítať pohyby. Token je tajný — nikdy sa nevracia z API,
// nezapisuje do auditu ani do chybových hlásení (je súčasťou adresy).

// fioGap je najkratší čas medzi dvoma volaniami Fio API (limit banky).
const fioGap = 30 * time.Second

// fioHistory je najstaršie obdobie pri prvom stiahnutí (Fio bez ďalšej
// autorizácie vydá pohyby za posledných 90 dní).
const fioHistory = 89

type fioAccount struct {
	ID       int64     `json:"id"`
	Label    string    `json:"label"`
	Token    string    `json:"token"`
	Account  string    `json:"account"`  // IBAN z posledného stiahnutia
	LastSync string    `json:"lastSync"` // posledný stiahnutý deň
	LastCall time.Time `json:"lastCall"`
}

// FioAccount je napojenie na účet vo Fio banke (bez tokenu).
type FioAccount struct {
	ID       int64  `json:"id"`
	Label    string `json:"label"`
	Account  string `json:"account"`
	LastSync string `json:"lastSync"`
	TokenSet bool   `json:"tokenSet"`
}

// FioAccountInput je zadanie napojenia; prázdny token ponechá uložený.
type FioAccountInput struct {
	ID    int64  `json:"id"`
	Label string `json:"label"`
	Token string `json:"token"`
}

var reFioToken = regexp.MustCompile(`^[A-Za-z0-9]{20,128}$`)

func fioKey(companyID int64) string { return fmt.Sprintf("bank.fio.%d", companyID) }

func (a *App) loadFio(ctx context.Context, companyID int64) ([]fioAccount, error) {
	out := []fioAccount{}
	raw, err := a.Store.GetSetting(ctx, fioKey(companyID))
	if errors.Is(err, store.ErrNotFound) || (err == nil && raw == "") {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	return out, json.Unmarshal([]byte(raw), &out)
}

func (a *App) saveFio(ctx context.Context, companyID int64, list []fioAccount) error {
	raw, _ := json.Marshal(list)
	return a.Store.SetSetting(ctx, fioKey(companyID), string(raw))
}

func fioView(list []fioAccount) []FioAccount {
	out := make([]FioAccount, 0, len(list))
	for _, f := range list {
		out = append(out, FioAccount{ID: f.ID, Label: f.Label, Account: f.Account, LastSync: f.LastSync, TokenSet: f.Token != ""})
	}
	return out
}

func (a *App) fioAvailable() error {
	if a.Caps.Browser {
		return vErr("fio", "Fiškus v prehliadači sa na Fio API nepripojí (banka to z prehliadača nedovolí) — použite nainštalovaný Fiškus alebo import výpisu")
	}
	return nil
}

// FioAccounts vráti napojenia na Fio banku.
func (a *App) FioAccounts(ctx context.Context, companyID int64) ([]FioAccount, error) {
	if err := a.Require(ctx, companyID, auth.PermBankRead); err != nil {
		return nil, err
	}
	list, err := a.loadFio(ctx, companyID)
	return fioView(list), err
}

// SaveFioAccount pridá alebo upraví napojenie na účet vo Fio banke.
func (a *App) SaveFioAccount(ctx context.Context, companyID int64, in FioAccountInput) ([]FioAccount, error) {
	if err := a.Require(ctx, companyID, auth.PermBankWrite); err != nil {
		return nil, err
	}
	if err := a.fioAvailable(); err != nil {
		return nil, err
	}
	list, err := a.loadFio(ctx, companyID)
	if err != nil {
		return nil, err
	}
	in.Label, in.Token = strings.TrimSpace(in.Label), strings.TrimSpace(in.Token)
	if in.Label == "" {
		in.Label = "Fio banka"
	}
	idx := -1
	var maxID int64
	for i, f := range list {
		maxID = max(maxID, f.ID)
		if in.ID != 0 && f.ID == in.ID {
			idx = i
		}
	}
	if in.ID != 0 && idx < 0 {
		return nil, store.ErrNotFound
	}
	if in.Token != "" && !reFioToken.MatchString(in.Token) {
		return nil, vErr("token", "Token Fio API sú písmená a číslice (64 znakov) — skopírujte ho celý z internetbankingu")
	}
	if idx < 0 {
		if in.Token == "" {
			return nil, vErr("token", "Zadajte token Fio API")
		}
		list = append(list, fioAccount{ID: maxID + 1, Label: in.Label, Token: in.Token})
	} else {
		list[idx].Label = in.Label
		if in.Token != "" {
			list[idx].Token = in.Token
		}
	}
	if err := a.saveFio(ctx, companyID, list); err != nil {
		return nil, err
	}
	a.record(ctx, companyID, audit.CatBank, "update", "fio", 0, "Napojenie na Fio banku: "+in.Label, nil,
		map[string]any{"label": in.Label, "tokenChanged": in.Token != ""})
	return fioView(list), nil
}

// DeleteFioAccount zruší napojenie (token sa zmaže).
func (a *App) DeleteFioAccount(ctx context.Context, companyID, id int64) ([]FioAccount, error) {
	if err := a.Require(ctx, companyID, auth.PermBankWrite); err != nil {
		return nil, err
	}
	list, err := a.loadFio(ctx, companyID)
	if err != nil {
		return nil, err
	}
	rest := list[:0]
	var label string
	for _, f := range list {
		if f.ID == id {
			label = f.Label
			continue
		}
		rest = append(rest, f)
	}
	if label == "" {
		return nil, store.ErrNotFound
	}
	if err := a.saveFio(ctx, companyID, rest); err != nil {
		return nil, err
	}
	a.record(ctx, companyID, audit.CatBank, "delete", "fio", 0, "Zrušené napojenie na Fio banku: "+label, nil, nil)
	return fioView(rest), nil
}

// SyncFio stiahne nové pohyby z Fio banky a spáruje ich ako pri výpise.
// Obdobie: od posledného stiahnutého dňa (deň pred ním pre neskoré
// zaúčtovanie), prvýkrát posledných 90 dní; už stiahnuté sa preskočia.
func (a *App) SyncFio(ctx context.Context, companyID, id int64) (ImportResult, error) {
	res := ImportResult{Warnings: []string{}, Format: "Fio API"}
	if err := a.Require(ctx, companyID, auth.PermBankWrite); err != nil {
		return res, err
	}
	if err := a.fioAvailable(); err != nil {
		return res, err
	}
	list, err := a.loadFio(ctx, companyID)
	if err != nil {
		return res, err
	}
	idx := -1
	for i, f := range list {
		if f.ID == id {
			idx = i
		}
	}
	if idx < 0 {
		return res, store.ErrNotFound
	}
	f := &list[idx]
	now := a.now()
	if wait := fioGap - now.Sub(f.LastCall); wait > 0 {
		return res, vErr("fio", fmt.Sprintf("Fio banka dovolí sťahovať pohyby raz za 30 sekúnd — skúste znova o %d s", int(wait.Seconds())+1))
	}
	today := a.Today()
	from := today.AddDays(-fioHistory)
	if last, err := invoice.ParseDate(f.LastSync); err == nil && from.Before(last) {
		from = last.AddDays(-1)
	}
	f.LastCall = now
	if err := a.saveFio(ctx, companyID, list); err != nil {
		return res, err
	}
	data, err := a.fetchFio(ctx, f.Token, from, today)
	if err != nil {
		return res, err
	}
	txs, err := bank.ParseFio(data)
	if err != nil {
		return res, vErr("fio", "Fio banka vrátila nečitateľnú odpoveď — skúste neskôr")
	}
	company, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return res, err
	}
	own := strings.ToUpper(strings.ReplaceAll(company.IBAN, " ", ""))
	res, err = a.importTransactions(ctx, companyID, own, "z Fio banky ("+f.Label+")", txs, res)
	if err != nil {
		return res, err
	}
	if len(txs) > 0 {
		f.Account = txs[0].Account
	}
	f.LastSync = today.String()
	return res, a.saveFio(ctx, companyID, list)
}

// fetchFio stiahne pohyby. Chyba nikdy neobsahuje adresu (je v nej token).
func (a *App) fetchFio(ctx context.Context, token string, from, to invoice.Date) ([]byte, error) {
	base := a.FioURL
	if base == "" {
		base = bank.FioURL
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bank.FioPeriodURL(base, token, from, to), nil)
	if err != nil {
		return nil, vErr("token", "Neplatný token Fio API")
	}
	client := a.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // bez adresy s tokenom
		}
		return nil, fmt.Errorf("spojenie s Fio bankou zlyhalo: %v", err)
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusConflict:
		return nil, vErr("fio", "Fio banka dovolí sťahovať pohyby raz za 30 sekúnd — skúste o chvíľu")
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusNotFound:
		return nil, vErr("token", "Fio banka token neprijala — skontrolujte ho v internetbankingu (Nastavenia → API) a zadajte znova")
	case res.StatusCode == http.StatusRequestEntityTooLarge || res.StatusCode == http.StatusInternalServerError:
		return nil, vErr("fio", "Fio banka odmietla požiadavku (neplatný token alebo pohyby staršie ako 90 dní bez dodatočnej autorizácie)")
	case res.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("Fio banka vrátila chybu %d", res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, 32<<20))
}
