//go:build sqlite && !js

package sqlite

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func TestPhoneSnapshot(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	c, err := s.CreateCompany(ctx, store.Company{Name: "Alfa"})
	if err != nil {
		t.Fatal(err)
	}
	secrets := map[string]string{
		"ai:anthropic": "sk-tajny-kluc-ai", "mail:1": `{"password":"tajne-smtp"}`, "eshops.1": "tajny-eshop",
		"bank.fio.1": "tajny-fio-token", "desktop.pin": "tajny-pin", "sync.desktop": "tajne-sparovanie",
	}
	for k, v := range secrets {
		if err := s.SetSetting(ctx, k, v); err != nil {
			t.Fatal(err)
		}
	}
	_ = s.SetSetting(ctx, "household.1", "ponechat")
	before, _ := s.Changes(ctx)

	b, err := s.PhoneSnapshot(ctx, 1234)
	if err != nil {
		t.Fatal(err)
	}
	if after, _ := s.Changes(ctx); after != before {
		t.Fatalf("kópia nesmie meniť údaje: %d → %d", before, after)
	}
	for _, v := range secrets {
		if bytes.Contains(b, []byte(v)) {
			t.Errorf("v kópii ostalo tajomstvo %q", v)
		}
	}
	path := filepath.Join(t.TempDir(), "telefon.db")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Verify(ctx, path); err != nil {
		t.Fatal(err)
	}
	p, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if v, _ := p.GetSetting(ctx, "sync.snapshot"); v != "1234" {
		t.Fatalf("verzia kópie: %q", v)
	}
	if v, _ := p.GetSetting(ctx, "household.1"); v != "ponechat" {
		t.Fatalf("bežné nastavenie: %q", v)
	}
	if got, err := p.GetCompany(ctx, c.ID); err != nil || got.Name != "Alfa" {
		t.Fatalf("firma v kópii: %+v %v", got, err)
	}
	if _, err := p.GetSetting(ctx, "ai:anthropic"); err == nil {
		t.Fatal("kľúč AI v kópii")
	}

	kv, err := OpenKV(filepath.Join(t.TempDir(), "stav.db"))
	if err != nil {
		t.Fatal(err)
	}
	if v, err := kv.Get(ctx, "phone"); v != "" || err != nil {
		t.Fatalf("prázdne úložisko: %q %v", v, err)
	}
	if err := kv.Set(ctx, "phone", "x"); err != nil {
		t.Fatal(err)
	}
	if v, _ := kv.Get(ctx, "phone"); v != "x" {
		t.Fatalf("úložisko: %q", v)
	}
}
