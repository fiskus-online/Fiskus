package app

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/backup"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/store/memstore"
)

// fakeBrowserDB nahrádza úložisko prehliadača.
type fakeBrowserDB struct {
	db     []byte
	staged []byte
	bad    bool // StageRestore odmietne databázu
}

func (f *fakeBrowserDB) Export(context.Context) ([]byte, error) {
	return append([]byte(nil), f.db...), nil
}
func (f *fakeBrowserDB) StageRestore(_ context.Context, b []byte) error {
	if f.bad {
		return errors.New("integrity_check: chyba")
	}
	f.staged = b
	return nil
}

func TestBrowserBackup(t *testing.T) {
	ctx := auth.WithActor(context.Background(), auth.Local)
	a := New(memstore.New(), edition.ForBrowser())
	db := &fakeBrowserDB{db: append([]byte("SQLite format 3\x00"), make([]byte, 100)...)}
	db.db[18], db.db[19] = 1, 1
	a.Browser = db
	day := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)

	// bez firmy nie je čo zálohovať
	if st, err := a.BrowserBackupStatus(ctx, day); err != nil || st.Remind || st.Last != nil {
		t.Fatalf("prázdne: %+v %v", st, err)
	}
	if _, err := a.CreateCompany(ctx, store.Company{Name: "Moja firma"}); err != nil {
		t.Fatal(err)
	}
	if st, _ := a.BrowserBackupStatus(ctx, day); !st.Remind {
		t.Fatal("firma bez zálohy: pripomenúť")
	}

	var ve *invoice.ValidationError
	if _, err := a.BrowserBackup(ctx, "kratke", day); !errors.As(err, &ve) {
		t.Fatalf("krátke heslo: %v", err)
	}
	fbk, err := a.BrowserBackup(ctx, "dlhe-heslo-zalohy", day)
	if err != nil {
		t.Fatal(err)
	}
	if plain, err := backup.Decrypt(fbk, "dlhe-heslo-zalohy"); err != nil || !bytes.Equal(plain, db.db) {
		t.Fatalf("záloha sa nedá otvoriť: %v", err)
	}
	if got := BrowserBackupName(day); got != "fiskus-20261106-100000.fbk" {
		t.Errorf("meno zálohy %s", got)
	}

	tests := []struct {
		name   string
		at     time.Time
		remind bool
		days   int
	}{
		{"hneď po zálohe", day, false, 0},
		{"o 6 dní", day.AddDate(0, 0, 6), false, 6},
		{"o 7 dní", day.AddDate(0, 0, 7), true, 7},
	}
	for _, tt := range tests {
		st, err := a.BrowserBackupStatus(ctx, tt.at)
		if err != nil || st.Remind != tt.remind || st.Days != tt.days || st.Last == nil || !st.Last.Equal(day) {
			t.Errorf("%s: %+v %v", tt.name, st, err)
		}
	}

	// obnova: zlé heslo, poškodená databáza, záloha z nainštalovaného Fiškusa (WAL)
	if err := a.BrowserRestore(ctx, fbk, "zle-heslo-zalohy"); !errors.As(err, &ve) || ve.Fields[0].Field != "password" {
		t.Fatalf("zlé heslo: %v", err)
	}
	wal := append([]byte(nil), db.db...)
	wal[18], wal[19] = 2, 2
	var buf bytes.Buffer
	if err := backup.Encrypt(&buf, wal, "dlhe-heslo-zalohy"); err != nil {
		t.Fatal(err)
	}
	if err := a.BrowserRestore(ctx, buf.Bytes(), "dlhe-heslo-zalohy"); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(db.staged, db.db) {
		t.Error("databáza z WAL sa mala prepnúť na bežný denník")
	}
	db.bad = true
	if err := a.BrowserRestore(ctx, fbk, "dlhe-heslo-zalohy"); !errors.As(err, &ve) || ve.Fields[0].Field != "file" {
		t.Fatalf("poškodená: %v", err)
	}

	// mimo prehliadača
	desk, _ := newApp(t, edition.Desktop)
	if _, err := desk.BrowserBackupStatus(ctx, day); !errors.Is(err, ErrNotBrowser) {
		t.Fatalf("desktop: %v", err)
	}
}
