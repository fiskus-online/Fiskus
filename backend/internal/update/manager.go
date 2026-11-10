package update

import (
	"context"
	"crypto/sha256"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Stavy aktualizácie (pole State).
const (
	Idle        = ""            // ešte sa nekontrolovalo
	Checking    = "checking"    // zisťuje sa posledné vydanie
	Current     = "current"     // Fiškus je aktuálny
	Available   = "available"   // je novšia verzia (čaká na potvrdenie alebo ručnú inštaláciu)
	Downloading = "downloading" // sťahuje sa
	Installing  = "installing"  // vymieňa sa program
	Restarting  = "restarting"  // starý sa ukončuje, nový sa spustí
	Failed      = "error"       // posledný pokus zlyhal (Error)
)

// Status je stav pre rozhranie.
type Status struct {
	Current  string  `json:"current"`
	Latest   string  `json:"latest,omitempty"`
	Notes    string  `json:"notes,omitempty"`
	Page     string  `json:"page,omitempty"`
	State    string  `json:"state"`
	Progress float64 `json:"progress"`
	Error    string  `json:"error,omitempty"`
	// Manual: prečo sa Fiškus nedá aktualizovať sám (stiahnuť ručne z Page).
	Manual string `json:"manual,omitempty"`
	Auto   bool   `json:"auto"` // aktualizovať automaticky po spustení
	Kind   Kind   `json:"kind,omitempty"`
}

// Manager riadi kontrolu, stiahnutie a výmenu programu.
type Manager struct {
	Version      string
	URL          string
	Client       *http.Client
	Target       Target
	GOOS, GOARCH string
	Dir          string // priečinok na stiahnutie a nastavenie
	// Prepare vymení program (alebo pripraví inštalátor) a vráti, čo sa
	// spustí po ukončení starého Fiškusa. Predvolene Install.
	Prepare func(t Target, file string) (after func() error, err error)
	// Restart požiada program o riadne ukončenie a potom zavolá after.
	Restart func(after func() error)

	mu     sync.Mutex
	st     Status
	rel    Release
	cancel context.CancelFunc
}

type settings struct {
	Auto bool `json:"auto"`
}

func (m *Manager) settingsPath() string { return filepath.Join(m.Dir, "nastavenie.json") }

// Init načíta nastavenie a uprace zvyšky predošlej aktualizácie.
func (m *Manager) Init() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.st = Status{Current: strings.TrimPrefix(m.Version, "v"), Auto: true, Kind: m.Target.Kind, Page: Page}
	if b, err := os.ReadFile(m.settingsPath()); err == nil {
		var s settings
		if json.Unmarshal(b, &s) == nil {
			m.st.Auto = s.Auto
		}
	}
	if parse(m.Version) == nil {
		m.st.Manual = "Vývojová zostava sa neaktualizuje."
	} else {
		m.st.Manual = m.Target.Manual
	}
	// stiahnuté súbory a pôvodný program po výmene
	if entries, err := os.ReadDir(m.Dir); err == nil {
		for _, e := range entries {
			if e.Name() != "nastavenie.json" {
				_ = os.RemoveAll(filepath.Join(m.Dir, e.Name()))
			}
		}
	}
	if m.Target.Exe != "" {
		_ = os.Remove(m.Target.Exe + ".old")
	}
	if m.Target.App != "" {
		_ = os.RemoveAll(m.Target.App + ".old")
	}
}

// Status vráti aktuálny stav.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.st
}

// SetAuto zapne alebo vypne automatickú aktualizáciu po spustení.
func (m *Manager) SetAuto(on bool) (Status, error) {
	b, _ := json.Marshal(settings{Auto: on})
	if err := os.MkdirAll(m.Dir, 0o700); err != nil {
		return m.Status(), err
	}
	if err := os.WriteFile(m.settingsPath(), b, 0o600); err != nil {
		return m.Status(), err
	}
	m.mu.Lock()
	m.st.Auto = on
	m.mu.Unlock()
	return m.Status(), nil
}

func (m *Manager) set(f func(s *Status)) {
	m.mu.Lock()
	f(&m.st)
	m.mu.Unlock()
}

// Check zistí posledné vydanie (počas sťahovania alebo výmeny nič nerobí).
func (m *Manager) Check(ctx context.Context) Status {
	m.mu.Lock()
	switch m.st.State {
	case Checking, Downloading, Installing, Restarting:
		defer m.mu.Unlock()
		return m.st
	}
	m.st.State, m.st.Error = Checking, ""
	m.mu.Unlock()

	rel, err := Check(ctx, m.Client, m.URL)
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.st.State, m.st.Error = Failed, err.Error()
		return m.st
	}
	m.rel = rel
	m.st.Latest, m.st.Notes = rel.Version, rel.Notes
	if !Newer(rel.Version, m.Version) {
		m.st.State = Current
		return m.st
	}
	m.st.State = Available
	if m.st.Manual == "" {
		if _, ok := Pick(m.Target, m.GOOS, m.GOARCH, rel.Assets); !ok {
			m.st.Manual = "Vo vydaní chýba súbor pre tento počítač."
		}
	}
	return m.st
}

// Start stiahne novú verziu a vymení program (beží na pozadí).
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.st.State != Available && m.st.State != Failed {
		return errors.New("nie je čo aktualizovať")
	}
	if m.st.Manual != "" {
		return errors.New(m.st.Manual)
	}
	asset, ok := Pick(m.Target, m.GOOS, m.GOARCH, m.rel.Assets)
	if !ok || !Newer(m.rel.Version, m.Version) {
		return errors.New("nie je čo aktualizovať")
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.st.State, m.st.Progress, m.st.Error = Downloading, 0, ""
	go m.run(ctx, asset)
	return nil
}

// Postpone zastaví sťahovanie; aktualizácia počká na ďalšie spustenie.
func (m *Manager) Postpone() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.st.State == Downloading && m.cancel != nil {
		m.cancel()
		m.st.State, m.st.Progress = Available, 0
	}
	return m.st
}

func (m *Manager) run(ctx context.Context, asset Asset) {
	file, err := m.download(ctx, asset)
	if ctx.Err() != nil {
		_ = os.Remove(file)
		return // odložené
	}
	if err != nil {
		m.fail(err)
		return
	}
	m.set(func(s *Status) { s.State, s.Progress = Installing, 1 })
	prepare := m.Prepare
	if prepare == nil {
		prepare = Install
	}
	after, err := prepare(m.Target, file)
	if err != nil {
		m.fail(err)
		return
	}
	m.set(func(s *Status) { s.State = Restarting })
	log.Printf("aktualizácia: %s → %s, reštartujem", m.Version, asset.Name)
	if m.Restart != nil {
		m.Restart(after)
	}
}

func (m *Manager) fail(err error) {
	log.Printf("aktualizácia zlyhala: %v", err)
	m.set(func(s *Status) { s.State, s.Error, s.Progress = Failed, err.Error(), 0 })
}

// download stiahne súbor do Dir a overí veľkosť a súčet SHA-256.
func (m *Manager) download(ctx context.Context, a Asset) (string, error) {
	if err := os.MkdirAll(m.Dir, 0o700); err != nil {
		return "", err
	}
	if a.Size > maxAsset {
		return "", errors.New("súbor aktualizácie je príliš veľký")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return "", err
	}
	res, err := m.Client.Do(req)
	if err != nil {
		return "", errors.New("sťahovanie sa prerušilo (internet?)")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("sťahovanie: server odpovedal %d", res.StatusCode)
	}
	total := a.Size
	if total <= 0 {
		total = res.ContentLength
	}
	path := filepath.Join(m.Dir, filepath.Base(a.Name))
	f, err := os.OpenFile(path+".part", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	var got int64
	buf := make([]byte, 256<<10)
	body := io.LimitReader(res.Body, maxAsset+1)
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				return path + ".part", werr
			}
			h.Write(buf[:n])
			got += int64(n)
			if total > 0 {
				p := float64(got) / float64(total)
				m.set(func(s *Status) { s.Progress = min(p, 0.99) })
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return path + ".part", errors.New("sťahovanie sa prerušilo (internet?)")
		}
	}
	if err := f.Close(); err != nil {
		return path + ".part", err
	}
	if got > maxAsset || (a.Size > 0 && got != a.Size) {
		return path + ".part", errors.New("stiahnutý súbor je neúplný")
	}
	if alg, want, ok := strings.Cut(a.Digest, ":"); ok && alg == "sha256" {
		if hex.EncodeToString(h.Sum(nil)) != strings.ToLower(want) {
			return path + ".part", errors.New("stiahnutý súbor nesedí so súčtom vydania — aktualizácia sa nenainštaluje")
		}
	}
	if err := os.Rename(path+".part", path); err != nil {
		return path + ".part", err
	}
	return path, nil
}

// Startup: po spustení aplikácie skontroluje vydanie a pri zapnutej
// automatickej aktualizácii ju hneď spustí.
func (m *Manager) Startup(ctx context.Context, delay time.Duration) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(delay):
	}
	st := m.Check(ctx)
	if st.State == Available && st.Auto && st.Manual == "" {
		if err := m.Start(); err != nil {
			log.Printf("aktualizácia: %v", err)
		}
	}
}

// IsGUI: je program pre Windows zostavený bez konzoly (-H windowsgui)?
func IsGUI(exe string) bool {
	f, err := pe.Open(exe)
	if err != nil {
		return false
	}
	defer f.Close()
	switch h := f.OptionalHeader.(type) {
	case *pe.OptionalHeader64:
		return h.Subsystem == pe.IMAGE_SUBSYSTEM_WINDOWS_GUI
	case *pe.OptionalHeader32:
		return h.Subsystem == pe.IMAGE_SUBSYSTEM_WINDOWS_GUI
	}
	return false
}
