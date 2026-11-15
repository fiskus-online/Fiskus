//go:build sqlite

package sqlite

import (
	"context"
	"path/filepath"
	"testing"
)

func TestReadSetting(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "kopia.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.SetSetting(ctx, "sync.applied.x", `[{"id":"a"}]`); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ key, want string }{
		{"sync.applied.x", `[{"id":"a"}]`},
		{"nie.je", ""},
	}
	for _, c := range cases {
		got, err := ReadSetting(ctx, path, c.key)
		if err != nil || got != c.want {
			t.Errorf("%s: %q %v", c.key, got, err)
		}
	}
}
