package update

import (
	"path/filepath"
	"strings"
)

// Kind je spôsob, akým je Fiškus nainštalovaný (z toho plynie, čo sa stiahne).
type Kind string

const (
	WindowsSetup    Kind = "windows-setup"    // inštalátor (Programs\Fiskus)
	WindowsPortable Kind = "windows-portable" // „bez inštalácie“ (okno, bez konzoly)
	MacApp          Kind = "macos-app"        // Fiškus.app z .dmg
	Binary          Kind = "binary"           // samotný program (Linux, macOS, Windows z príkazového riadka)
	Deb             Kind = "deb"              // balík Debian/Ubuntu
	RPM             Kind = "rpm"              // balík Fedora/openSUSE
)

// Target: čo a kde sa vymení.
type Target struct {
	Kind Kind
	Exe  string // bežiaci program
	App  string // MacApp: priečinok …/Fiškus.app
	// Manual: prečo sa nedá aktualizovať samo (prázdne = dá sa).
	Manual string
}

// Env sú vlastnosti počítača (v testoch podvrhnuté).
type Env struct {
	GOOS, GOARCH string
	Exe          string
	Exists       func(path string) bool
	Writable     func(dir string) bool
	LookPath     func(name string) bool
	GUI          bool // Windows: program je zostavený bez konzoly (-H windowsgui)
}

// Detect zistí spôsob inštalácie podľa umiestnenia programu.
func Detect(e Env) Target {
	t := Target{Exe: e.Exe}
	dir := filepath.Dir(e.Exe)
	switch e.GOOS {
	case "windows":
		switch {
		case e.Exists(filepath.Join(dir, "unins000.exe")):
			t.Kind = WindowsSetup
		case e.GUI:
			t.Kind = WindowsPortable
		default:
			t.Kind = Binary
		}
		if t.Kind != WindowsSetup && !e.Writable(dir) {
			t.Manual = "Do priečinka s programom sa nedá zapisovať."
		}
	case "darwin":
		// …/Fiškus.app/Contents/MacOS/fiskus
		app := filepath.Dir(filepath.Dir(dir))
		if strings.HasSuffix(app, ".app") && filepath.Base(filepath.Dir(dir)) == "Contents" {
			t.Kind, t.App = MacApp, app
			if !e.Writable(filepath.Dir(app)) {
				t.Manual = "Do priečinka Aplikácie sa nedá zapisovať bez správcu."
			}
		} else {
			t.Kind = Binary
			if !e.Writable(dir) {
				t.Manual = "Do priečinka s programom sa nedá zapisovať."
			}
		}
	default: // linux a ďalšie
		switch {
		case e.Writable(dir):
			t.Kind = Binary
		case e.LookPath("dpkg"):
			t.Kind = Deb
		case e.LookPath("rpm"):
			t.Kind = RPM
		default:
			t.Kind = Binary
			t.Manual = "Do priečinka s programom sa nedá zapisovať."
		}
		if (t.Kind == Deb || t.Kind == RPM) && !e.LookPath("pkexec") {
			t.Manual = "Inštalácia balíka potrebuje heslo správcu a chýba pkexec."
		}
	}
	return t
}

// Pick vyberie súbor vydania pre daný spôsob inštalácie.
func Pick(t Target, goos, goarch string, assets []Asset) (Asset, bool) {
	debArch := map[string]string{"amd64": "amd64", "arm64": "arm64"}[goarch]
	rpmArch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[goarch]
	match := func(n string) bool {
		switch t.Kind {
		case WindowsSetup:
			return strings.HasSuffix(n, "-windows-setup.exe")
		case WindowsPortable:
			return strings.HasSuffix(n, "-windows-bez-instalacie.exe")
		case MacApp:
			return strings.HasSuffix(n, "-macos.dmg")
		case Deb:
			return debArch != "" && strings.HasPrefix(n, "fiskus_") && strings.HasSuffix(n, "_"+debArch+".deb")
		case RPM:
			return rpmArch != "" && strings.HasPrefix(n, "fiskus-") && strings.HasSuffix(n, "."+rpmArch+".rpm")
		default:
			suffix := "-" + goos + "-" + goarch
			if goos == "windows" {
				suffix += ".exe"
			}
			return (strings.HasPrefix(n, "fiskus-v") || strings.HasPrefix(n, "fiskus-V")) && strings.HasSuffix(n, suffix)
		}
	}
	for _, a := range assets {
		if match(a.Name) {
			return a, true
		}
	}
	return Asset{}, false
}
