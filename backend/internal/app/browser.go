package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/backup"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Fiškus v prehliadači (cmd/fiskus-web): databáza je v úložisku prehliadača
// na počítači používateľa. Zálohuje sa stiahnutím šifrovaného súboru .fbk
// v rovnakom formáte, aký robí nainštalovaný Fiškus — záloha sa dá obnoviť
// v prehliadači aj v nainštalovanom programe a naopak.

// BrowserDB je databáza v úložisku prehliadača (nastavuje cmd/fiskus-web).
type BrowserDB interface {
	// Export vráti konzistentnú kópiu databázy (súbor SQLite).
	Export(ctx context.Context) ([]byte, error)
	// StageRestore overí databázu a pripraví ju na obnovu pri ďalšom
	// spustení (prehliadač po obnove načíta stránku znova).
	StageRestore(ctx context.Context, db []byte) error
}

// BrowserRemindDays: po koľkých dňoch bez zálohy Fiškus pripomenie zálohu.
const BrowserRemindDays = 7

const browserBackupKey = "browser.backup"

// ErrNotBrowser — funkcia je iba vo Fiškuse v prehliadači.
var ErrNotBrowser = errors.New("táto funkcia je iba vo Fiškuse v prehliadači")

// BrowserBackupStatus je stav záloh vo Fiškuse v prehliadači.
type BrowserBackupStatus struct {
	Last   *time.Time `json:"last,omitempty"` // posledná stiahnutá záloha
	Remind bool       `json:"remind"`         // záloha chýba alebo je staršia ako BrowserRemindDays
	Days   int        `json:"days"`           // dni od poslednej zálohy
}

type browserBackupState struct {
	Last time.Time `json:"last"`
}

// BrowserBackupStatus vráti, kedy sa naposledy zálohovalo a či pripomenúť
// zálohu (iba keď je čo zálohovať — aspoň jedna firma).
func (a *App) BrowserBackupStatus(ctx context.Context, now time.Time) (BrowserBackupStatus, error) {
	if a.Browser == nil {
		return BrowserBackupStatus{}, ErrNotBrowser
	}
	var st BrowserBackupStatus
	var s browserBackupState
	v, err := a.Store.GetSetting(ctx, browserBackupKey)
	switch {
	case errors.Is(err, store.ErrNotFound):
	case err != nil:
		return st, err
	default:
		if err := json.Unmarshal([]byte(v), &s); err != nil {
			return st, err
		}
	}
	companies, err := a.Store.ListCompanies(ctx)
	if err != nil {
		return st, err
	}
	if !s.Last.IsZero() {
		last := s.Last
		st.Last = &last
		st.Days = int(now.Sub(s.Last).Hours() / 24)
	}
	st.Remind = len(companies) > 0 && (st.Last == nil || st.Days >= BrowserRemindDays)
	return st, nil
}

// BrowserBackupName je meno súboru zálohy (rovnaké ako v nainštalovanom Fiškuse).
func BrowserBackupName(now time.Time) string {
	return "fiskus-" + now.UTC().Format("20060102-150405") + ".fbk"
}

// BrowserBackup vytvorí šifrovanú zálohu databázy na stiahnutie.
func (a *App) BrowserBackup(ctx context.Context, password string, now time.Time) ([]byte, error) {
	if err := requireSuperAdmin(ctx); err != nil {
		return nil, err
	}
	if a.Browser == nil {
		return nil, ErrNotBrowser
	}
	if n := len([]rune(password)); n < MinBackupPassword {
		return nil, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "password",
			Message: fmt.Sprintf("Heslo musí mať aspoň %d znakov", MinBackupPassword)}}}
	}
	plain, err := a.Browser.Export(ctx)
	if err != nil {
		return nil, fmt.Errorf("kópia databázy: %w", err)
	}
	var buf bytes.Buffer
	if err := backup.Encrypt(&buf, plain, password); err != nil {
		return nil, err
	}
	if back, err := backup.Decrypt(buf.Bytes(), password); err != nil || !bytes.Equal(back, plain) {
		return nil, fmt.Errorf("overenie zálohy zlyhalo: %v", err)
	}
	b, _ := json.Marshal(browserBackupState{Last: now.UTC().Truncate(time.Second)})
	if err := a.Store.SetSetting(ctx, browserBackupKey, string(b)); err != nil {
		return nil, err
	}
	a.record(ctx, 0, audit.CatBackups, "backup", "backup", 0, "Stiahnutá záloha "+BrowserBackupName(now), nil, nil)
	return buf.Bytes(), nil
}

// BrowserRestore dešifruje zálohu (.fbk z prehliadača aj z nainštalovaného
// Fiškusa) a pripraví obnovu; databáza sa nahradí pri ďalšom spustení.
func (a *App) BrowserRestore(ctx context.Context, file []byte, password string) error {
	if err := requireSuperAdmin(ctx); err != nil {
		return err
	}
	if a.Browser == nil {
		return ErrNotBrowser
	}
	plain, err := backup.Decrypt(file, password)
	if err != nil {
		return &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "password",
			Message: "Zálohu sa nepodarilo otvoriť — nesprávne heslo alebo to nie je záloha Fiškusa"}}}
	}
	// databáza z nainštalovaného Fiškusa je v režime WAL; kópia je úplná,
	// takže stačí v hlavičke prepnúť na bežný denník (úložisko prehliadača WAL nepozná)
	if len(plain) > 19 && plain[18] == 2 && plain[19] == 2 {
		plain[18], plain[19] = 1, 1
	}
	if err := a.Browser.StageRestore(ctx, plain); err != nil {
		return &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "file",
			Message: "Záloha je poškodená: " + err.Error()}}}
	}
	a.record(ctx, 0, audit.CatBackups, "restore", "backup", 0, "Obnova zo zálohy pri ďalšom spustení", nil, nil)
	return nil
}
