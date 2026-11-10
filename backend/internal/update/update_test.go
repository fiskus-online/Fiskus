package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	for _, tc := range []struct {
		latest, current string
		want            bool
	}{
		{"0.8.0", "0.7.0", true},
		{"v0.7.1", "v0.7.0", true},
		{"1.0.0", "0.99.9", true},
		{"0.10.0", "0.9.0", true},
		{"0.7.0", "0.7.0", false},
		{"0.6.9", "0.7.0", false},
		{"0.8.0", "dev", false},
		{"0.8.0-rc1", "0.7.0", false},
		{"0.8", "0.7.0", false},
	} {
		if got := Newer(tc.latest, tc.current); got != tc.want {
			t.Errorf("Newer(%q, %q) = %v", tc.latest, tc.current, got)
		}
	}
}

var assets07 = []Asset{
	{Name: "fiskus-0.7.0-1.aarch64.rpm"}, {Name: "fiskus-0.7.0-1.x86_64.rpm"},
	{Name: "fiskus-0.7.0-macos.dmg"}, {Name: "fiskus-0.7.0-windows-setup.exe"},
	{Name: "fiskus-mcp-v0.7.0-darwin-arm64"}, {Name: "fiskus-mcp-v0.7.0-linux-amd64"},
	{Name: "fiskus-mcp-v0.7.0-windows-amd64.exe"},
	{Name: "fiskus-v0.7.0-darwin-arm64"}, {Name: "fiskus-v0.7.0-linux-amd64"}, {Name: "fiskus-v0.7.0-linux-arm64"},
	{Name: "fiskus-v0.7.0-windows-amd64.exe"}, {Name: "fiskus-v0.7.0-windows-bez-instalacie.exe"},
	{Name: "fiskus_0.7.0_amd64.deb"}, {Name: "fiskus_0.7.0_arm64.deb"},
}

func TestDetectPick(t *testing.T) {
	yes := func(string) bool { return true }
	no := func(string) bool { return false }
	only := func(names ...string) func(string) bool {
		return func(s string) bool {
			for _, n := range names {
				if strings.HasSuffix(s, n) {
					return true
				}
			}
			return false
		}
	}
	for _, tc := range []struct {
		name   string
		env    Env
		kind   Kind
		asset  string
		manual bool
	}{
		{"Windows inštalátor", Env{GOOS: "windows", GOARCH: "amd64", Exe: `C:\U\AppData\Local\Programs\Fiskus\fiskus.exe`, Exists: only("unins000.exe"), Writable: yes, LookPath: no, GUI: true},
			WindowsSetup, "fiskus-0.7.0-windows-setup.exe", false},
		{"Windows bez inštalácie", Env{GOOS: "windows", GOARCH: "amd64", Exe: `C:\U\Downloads\fiskus.exe`, Exists: no, Writable: yes, LookPath: no, GUI: true},
			WindowsPortable, "fiskus-v0.7.0-windows-bez-instalacie.exe", false},
		{"Windows príkazový riadok", Env{GOOS: "windows", GOARCH: "amd64", Exe: `C:\nastroje\fiskus.exe`, Exists: no, Writable: yes, LookPath: no},
			Binary, "fiskus-v0.7.0-windows-amd64.exe", false},
		{"Windows bez zápisu", Env{GOOS: "windows", GOARCH: "amd64", Exe: `C:\Program Files\fiskus.exe`, Exists: no, Writable: no, LookPath: no, GUI: true},
			WindowsPortable, "fiskus-v0.7.0-windows-bez-instalacie.exe", true},
		{"macOS aplikácia", Env{GOOS: "darwin", GOARCH: "arm64", Exe: "/Applications/Fiškus.app/Contents/MacOS/fiskus", Exists: no, Writable: yes, LookPath: no},
			MacApp, "fiskus-0.7.0-macos.dmg", false},
		{"macOS Aplikácie bez zápisu", Env{GOOS: "darwin", GOARCH: "arm64", Exe: "/Applications/Fiškus.app/Contents/MacOS/fiskus", Exists: no, Writable: no, LookPath: no},
			MacApp, "fiskus-0.7.0-macos.dmg", true},
		{"macOS program", Env{GOOS: "darwin", GOARCH: "arm64", Exe: "/Users/p/bin/fiskus", Exists: no, Writable: yes, LookPath: no},
			Binary, "fiskus-v0.7.0-darwin-arm64", false},
		{"Linux deb", Env{GOOS: "linux", GOARCH: "amd64", Exe: "/usr/bin/fiskus", Exists: no, Writable: no, LookPath: only("dpkg", "pkexec")},
			Deb, "fiskus_0.7.0_amd64.deb", false},
		{"Linux rpm arm", Env{GOOS: "linux", GOARCH: "arm64", Exe: "/usr/bin/fiskus", Exists: no, Writable: no, LookPath: only("rpm", "pkexec")},
			RPM, "fiskus-0.7.0-1.aarch64.rpm", false},
		{"Linux deb bez pkexec", Env{GOOS: "linux", GOARCH: "amd64", Exe: "/usr/bin/fiskus", Exists: no, Writable: no, LookPath: only("dpkg")},
			Deb, "fiskus_0.7.0_amd64.deb", true},
		{"Linux program", Env{GOOS: "linux", GOARCH: "arm64", Exe: "/home/p/fiskus", Exists: no, Writable: yes, LookPath: no},
			Binary, "fiskus-v0.7.0-linux-arm64", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tg := Detect(tc.env)
			if tg.Kind != tc.kind || (tg.Manual != "") != tc.manual {
				t.Fatalf("Detect = %+v", tg)
			}
			a, ok := Pick(tg, tc.env.GOOS, tc.env.GOARCH, assets07)
			if !ok || a.Name != tc.asset {
				t.Fatalf("Pick = %q %v", a.Name, ok)
			}
		})
	}
	if _, ok := Pick(Target{Kind: Binary}, "freebsd", "amd64", assets07); ok {
		t.Fatal("súbor pre nepodporovaný systém")
	}
}

// vydanie na podvrhnutom GitHube
func fakeGitHub(t *testing.T, tag string, payload []byte, digest string) (*httptest.Server, *atomic.Int32) {
	var downloads atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tag_name": tag, "body": "- novinky", "html_url": srv.URL + "/v",
				"assets": []map[string]any{
					{"name": "fiskus-v" + strings.TrimPrefix(tag, "v") + "-linux-amd64", "browser_download_url": srv.URL + "/bin", "size": len(payload), "digest": digest},
				},
			})
		case "/bin":
			downloads.Add(1)
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &downloads
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}

func newManager(t *testing.T, url string) (*Manager, chan func() error) {
	restarted := make(chan func() error, 1)
	m := &Manager{
		Version: "0.7.0", URL: url, Client: http.DefaultClient, GOOS: "linux", GOARCH: "amd64",
		Target:  Target{Kind: Binary, Exe: filepath.Join(t.TempDir(), "fiskus")},
		Dir:     filepath.Join(t.TempDir(), "aktualizacia"),
		Restart: func(after func() error) { restarted <- after },
	}
	m.Init()
	return m, restarted
}

func waitState(t *testing.T, m *Manager, states ...string) Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		st := m.Status()
		for _, s := range states {
			if st.State == s {
				return st
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("stav %q, čakal som %v", m.Status().State, states)
	return Status{}
}

func TestManagerFlow(t *testing.T) {
	payload := []byte("#!/bin/sh\necho nový Fiškus\n")
	srv, downloads := fakeGitHub(t, "v0.8.0", payload, sum(payload))
	m, restarted := newManager(t, srv.URL+"/latest")
	if err := os.WriteFile(m.Target.Exe, []byte("starý"), 0o755); err != nil {
		t.Fatal(err)
	}

	m.Startup(context.Background(), 0)
	after := <-restarted
	if st := m.Status(); st.State != Restarting || st.Latest != "0.8.0" || st.Notes != "- novinky" || st.Current != "0.7.0" {
		t.Fatalf("stav: %+v", st)
	}
	if b, _ := os.ReadFile(m.Target.Exe); string(b) != string(payload) {
		t.Fatalf("program nevymenený: %q", b)
	}
	if after == nil || downloads.Load() != 1 {
		t.Fatalf("po ukončení: %v, stiahnutí %d", after != nil, downloads.Load())
	}
}

func TestManagerCases(t *testing.T) {
	payload := []byte("nový")
	t.Run("aktuálny", func(t *testing.T) {
		srv, dl := fakeGitHub(t, "v0.7.0", payload, sum(payload))
		m, _ := newManager(t, srv.URL+"/latest")
		if st := m.Check(context.Background()); st.State != Current || st.Latest != "0.7.0" {
			t.Fatalf("%+v", st)
		}
		if m.Start() == nil || dl.Load() != 0 {
			t.Fatal("aktualizácia bez novej verzie")
		}
	})
	t.Run("zlý súčet", func(t *testing.T) {
		srv, _ := fakeGitHub(t, "v0.8.0", payload, "sha256:"+strings.Repeat("0", 64))
		m, restarted := newManager(t, srv.URL+"/latest")
		_ = os.WriteFile(m.Target.Exe, []byte("starý"), 0o755)
		m.Check(context.Background())
		if err := m.Start(); err != nil {
			t.Fatal(err)
		}
		st := waitState(t, m, Failed)
		if !strings.Contains(st.Error, "súčtom") || len(restarted) != 0 {
			t.Fatalf("%+v", st)
		}
		if b, _ := os.ReadFile(m.Target.Exe); string(b) != "starý" {
			t.Fatal("program vymenený napriek zlému súčtu")
		}
	})
	t.Run("vypnuté automaticky", func(t *testing.T) {
		srv, dl := fakeGitHub(t, "v0.8.0", payload, sum(payload))
		m, _ := newManager(t, srv.URL+"/latest")
		if _, err := m.SetAuto(false); err != nil {
			t.Fatal(err)
		}
		m.Init() // nastavenie zostane po reštarte
		m.Startup(context.Background(), 0)
		if st := m.Status(); st.State != Available || st.Auto || dl.Load() != 0 {
			t.Fatalf("%+v, stiahnutí %d", st, dl.Load())
		}
	})
	t.Run("vývojová zostava", func(t *testing.T) {
		srv, dl := fakeGitHub(t, "v0.8.0", payload, sum(payload))
		m, _ := newManager(t, srv.URL+"/latest")
		m.Version = "dev"
		m.Init()
		m.Startup(context.Background(), 0)
		if st := m.Status(); st.Manual == "" || dl.Load() != 0 {
			t.Fatalf("%+v", st)
		}
	})
	t.Run("ručná inštalácia", func(t *testing.T) {
		srv, dl := fakeGitHub(t, "v0.8.0", payload, sum(payload))
		m, _ := newManager(t, srv.URL+"/latest")
		m.Target.Manual = "Do priečinka s programom sa nedá zapisovať."
		m.Init()
		m.Startup(context.Background(), 0)
		if st := m.Status(); st.State != Available || st.Manual == "" || dl.Load() != 0 || m.Start() == nil {
			t.Fatalf("%+v", st)
		}
	})
	t.Run("GitHub nedostupný", func(t *testing.T) {
		m, _ := newManager(t, "http://127.0.0.1:1/latest")
		if st := m.Check(context.Background()); st.State != Failed || st.Error == "" {
			t.Fatalf("%+v", st)
		}
	})
	t.Run("odložiť", func(t *testing.T) {
		block := make(chan struct{})
		var srv *httptest.Server
		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/bin" {
				w.Header().Set("Content-Length", "1000")
				_, _ = w.Write([]byte("x"))
				w.(http.Flusher).Flush()
				select {
				case <-block:
				case <-r.Context().Done():
				}
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v0.8.0", "assets": []map[string]any{{"name": "fiskus-v0.8.0-linux-amd64", "browser_download_url": srv.URL + "/bin", "size": 1000}}})
		}))
		defer srv.Close()
		defer close(block)
		m, restarted := newManager(t, srv.URL+"/latest")
		m.Check(context.Background())
		_ = m.Start()
		waitState(t, m, Downloading)
		if st := m.Postpone(); st.State != Available {
			t.Fatalf("%+v", st)
		}
		time.Sleep(50 * time.Millisecond)
		if st := m.Status(); st.State != Available || len(restarted) != 0 {
			t.Fatalf("po odložení: %+v", st)
		}
	})
	t.Run("výmena zlyhá", func(t *testing.T) {
		srv, _ := fakeGitHub(t, "v0.8.0", payload, sum(payload))
		m, restarted := newManager(t, srv.URL+"/latest")
		m.Prepare = func(Target, string) (func() error, error) { return nil, errors.New("disk plný") }
		m.Check(context.Background())
		_ = m.Start()
		if st := waitState(t, m, Failed); st.Error != "disk plný" || len(restarted) != 0 {
			t.Fatalf("%+v", st)
		}
	})
	t.Run("upratanie po aktualizácii", func(t *testing.T) {
		m, _ := newManager(t, "http://127.0.0.1:1/latest")
		_ = os.WriteFile(filepath.Join(m.Dir, "fiskus-v0.8.0-linux-amd64"), payload, 0o600)
		_ = os.WriteFile(m.Target.Exe+".old", payload, 0o600)
		_, _ = m.SetAuto(true)
		m.Init()
		if _, err := os.Stat(m.Target.Exe + ".old"); !os.IsNotExist(err) {
			t.Fatal("starý program ostal")
		}
		left, _ := os.ReadDir(m.Dir)
		if len(left) != 1 || left[0].Name() != "nastavenie.json" {
			t.Fatalf("zvyšky: %v", left)
		}
	})
}

func TestSwap(t *testing.T) {
	dir := t.TempDir()
	exe, file := filepath.Join(dir, "fiskus.exe"), filepath.Join(dir, "novy")
	_ = os.WriteFile(exe, []byte("starý"), 0o755)
	_ = os.WriteFile(file, []byte("nový"), 0o600)
	if err := swapWindows(exe, file); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "nový" {
		t.Fatal("nový program")
	}
	if b, _ := os.ReadFile(exe + ".old"); string(b) != "starý" {
		t.Fatal("starý program ako .old")
	}
	if err := swapWindows(filepath.Join(dir, "nie.exe"), file); err == nil {
		t.Fatal("chýbajúci program")
	}
	unix := filepath.Join(dir, "fiskus")
	_ = os.WriteFile(unix, []byte("starý"), 0o755)
	if err := swapUnix(unix, file); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(unix); fi.Mode().Perm()&0o100 == 0 {
		t.Fatal("nový program nie je spustiteľný")
	}
	if IsGUI(file) {
		t.Fatal("IsGUI na inom ako .exe")
	}
}
