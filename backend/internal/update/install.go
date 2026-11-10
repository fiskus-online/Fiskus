package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Relaunch sú parametre nového Fiškusa po aktualizácii: aplikácia, bez
// otvorenia nového okna (otvorené okno sa samo znova načíta).
var Relaunch = []string{"-app", "-open=false"}

// Install vymení program podľa spôsobu inštalácie a vráti, čo sa spustí
// po ukončení starého Fiškusa.
func Install(t Target, file string) (func() error, error) {
	switch t.Kind {
	case WindowsSetup:
		// inštalátor (Inno Setup) beží po ukončení Fiškusa: tichá inštalácia,
		// /aktualizacia=1 ho nechá Fiškus na konci znova spustiť (fiskus.iss)
		return func() error {
			return start(file, "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/SP-", "/aktualizacia=1")
		}, nil
	case WindowsPortable:
		if err := swapWindows(t.Exe, file); err != nil {
			return nil, err
		}
	case Binary:
		if strings.HasSuffix(strings.ToLower(t.Exe), ".exe") {
			if err := swapWindows(t.Exe, file); err != nil {
				return nil, err
			}
		} else if err := swapUnix(t.Exe, file); err != nil {
			return nil, err
		}
	case MacApp:
		if err := swapMacApp(t.App, file); err != nil {
			return nil, err
		}
		return func() error {
			return start("open", append([]string{"-n", t.App, "--args"}, Relaunch...)...)
		}, nil
	case Deb, RPM:
		args := []string{"dpkg", "-i", file}
		if t.Kind == RPM {
			args = []string{"rpm", "-U", "--replacepkgs", file}
		}
		// pkexec si vypýta heslo správcu v okne systému
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "pkexec", args...).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("inštalácia balíka zlyhala (%v): %s", err, lastLine(out))
		}
	default:
		return nil, errors.New("neznámy spôsob inštalácie")
	}
	exe := t.Exe
	return func() error { return start(exe, Relaunch...) }, nil
}

// start spustí program samostatne (beží ďalej aj po skončení Fiškusa).
func start(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// swapUnix: nový program vedľa starého a premenovanie naraz (bežiaci
// program to nevadí).
func swapUnix(exe, file string) error {
	tmp := exe + ".new"
	if err := copyFile(file, tmp, 0o755); err != nil {
		return fmt.Errorf("program sa nepodarilo zapísať: %w", err)
	}
	if err := os.Rename(tmp, exe); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("program sa nepodarilo vymeniť: %w", err)
	}
	return nil
}

// swapWindows: bežiaci .exe sa nedá prepísať, ale dá sa premenovať; starý
// ostane ako .old a zmaže sa pri ďalšom spustení.
func swapWindows(exe, file string) error {
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("program sa nepodarilo vymeniť: %w", err)
	}
	if err := copyFile(file, exe, 0o755); err != nil {
		_ = os.Remove(exe)
		_ = os.Rename(old, exe)
		return fmt.Errorf("program sa nepodarilo zapísať: %w", err)
	}
	return nil
}

// swapMacApp pripojí .dmg, skopíruje z neho Fiškus.app vedľa pôvodného
// a vymení ich.
func swapMacApp(app, dmg string) error {
	mnt, err := os.MkdirTemp("", "fiskus-dmg")
	if err != nil {
		return err
	}
	defer os.Remove(mnt)
	if out, err := exec.Command("hdiutil", "attach", "-nobrowse", "-readonly", "-noautoopen", "-mountpoint", mnt, dmg).CombinedOutput(); err != nil {
		return fmt.Errorf("obraz .dmg sa nepodarilo otvoriť: %s", lastLine(out))
	}
	defer exec.Command("hdiutil", "detach", "-force", mnt).Run() //nolint:errcheck
	src, err := findApp(mnt)
	if err != nil {
		return err
	}
	tmp := app + ".new"
	_ = os.RemoveAll(tmp)
	if out, err := exec.Command("ditto", src, tmp).CombinedOutput(); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("Fiškus.app sa nepodarilo skopírovať: %s", lastLine(out))
	}
	old := app + ".old"
	_ = os.RemoveAll(old)
	if err := os.Rename(app, old); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("Fiškus.app sa nepodarilo vymeniť: %w", err)
	}
	if err := os.Rename(tmp, app); err != nil {
		_ = os.Rename(old, app)
		return fmt.Errorf("Fiškus.app sa nepodarilo vymeniť: %w", err)
	}
	_ = os.RemoveAll(old)
	return nil
}

func findApp(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".app") {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", errors.New("v obraze .dmg nie je Fiškus.app")
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func lastLine(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
