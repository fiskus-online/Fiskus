//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
)

// Kópia databázy pre telefón mimo domu (app/sync.go): všetky údaje firiem
// a domácností, ale bez hesiel a prístupových kľúčov k iným službám
// (e-mail, AI, e-shopy, banka, kalendáre, poštár), bez API kľúčov, relácií
// a spárovaní. Zmazané údaje sa prepíšu (secure_delete) a súbor sa zhutní,
// aby v ňom neostali ani vo voľných stránkach.

// phoneSecrets: nastavenia, ktoré do kópie pre telefón nejdú (vzory LIKE).
var phoneSecrets = []string{
	"ai:%", "mail:%", "einvoice.%", "eshops.%", "bank.fio.%", "calsync.%", "quote.share.%",
	"oauth.secret", "desktop.pin", "mobile", "sync.desktop",
}

// PhoneSnapshot vytvorí kópiu databázy pre telefón s verziou v nastavení
// sync.snapshot (app.SyncSnapshotKey).
func (s *Store) PhoneSnapshot(ctx context.Context, version int64) ([]byte, error) {
	dir, err := os.MkdirTemp("", "fiskus-telefon-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	raw := filepath.Join(dir, "kopia.db")
	if err := s.Snapshot(ctx, raw); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+raw)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	stmts := []string{"PRAGMA secure_delete = ON"}
	for _, k := range phoneSecrets {
		stmts = append(stmts, "DELETE FROM settings WHERE key LIKE '"+k+"'")
	}
	stmts = append(stmts,
		"DELETE FROM api_keys",
		"DELETE FROM sessions",
		"UPDATE users SET password_hash = '', mfa_json = ''",
		"INSERT OR REPLACE INTO settings (key, value) VALUES ('sync.snapshot', '"+strconv.FormatInt(version, 10)+"')",
	)
	for _, q := range stmts {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return nil, err
		}
	}
	out := filepath.Join(dir, "telefon.db")
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", out); err != nil {
		return nil, err
	}
	db.Close()
	return os.ReadFile(out)
}

// Changes rastie s každou zmenou údajov (počet zmien spojenia od jeho otvorenia).
func (s *Store) Changes(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, "SELECT total_changes()").Scan(&n)
	return n, err
}

// KV je malé úložisko kľúč–hodnota v samostatnom súbore (stav telefónu
// mimo domu, ktorý nesmie zaniknúť s kópiou databázy).
type KV struct{ db *sql.DB }

// OpenKV otvorí (alebo vytvorí) úložisko.
func OpenKV(path string) (*KV, error) {
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS kv (key TEXT PRIMARY KEY, value TEXT NOT NULL)"); err != nil {
		db.Close()
		return nil, err
	}
	return &KV{db: db}, nil
}

// Get vráti hodnotu (prázdnu, keď kľúč nie je).
func (k *KV) Get(ctx context.Context, key string) (string, error) {
	var v string
	err := k.db.QueryRowContext(ctx, "SELECT value FROM kv WHERE key = ?", key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// Set uloží hodnotu.
func (k *KV) Set(ctx context.Context, key, value string) error {
	_, err := k.db.ExecContext(ctx, "INSERT OR REPLACE INTO kv (key, value) VALUES (?, ?)", key, value)
	return err
}
