package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Brána Windows Defender Firewall a mobil v sieti: pri prvom počúvaní sa
// Windows opýta, či Fiškus pustiť do siete. Keď používateľ okno zavrie
// alebo je Wi-Fi nastavená ako verejná sieť, Windows pripojenia z telefónu
// potichu zahodí — telefón sa iba „točí“. Stav sa dá zistiť bez práv
// správcu; povolenie (pravidlo iba pre port mobilu) potrebuje potvrdenie
// v okne UAC.

// FirewallRule je názov pravidla, ktoré pridá Fiškus.
const FirewallRule = "Fiskus - mobil v sieti"

// FirewallInfo je stav brány firewall pre Fiškus.
type FirewallInfo struct {
	Supported bool     `json:"supported"` // iba Windows
	Blocked   bool     `json:"blocked"`   // pravidlo blokuje Fiškus (prichádzajúce)
	Allowed   bool     `json:"allowed"`   // pravidlo povoľuje Fiškus alebo port mobilu
	Public    []string `json:"public"`    // pripojenia nastavené ako verejná sieť
	Error     string   `json:"error,omitempty"`
}

// psQuote vráti reťazec pre PowerShell v jednoduchých úvodzovkách.
func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// FirewallCheckScript zistí stav pravidiel pre program a port (bez práv správcu).
func FirewallCheckScript(exe string, port int) string {
	return fmt.Sprintf(`$ErrorActionPreference = 'SilentlyContinue'
$p = %s
$r = @(Get-NetFirewallApplicationFilter -Program $p | Get-NetFirewallRule | Where-Object { $_.Direction -eq 'Inbound' -and $_.Enabled -eq 'True' })
$n = @(Get-NetFirewallRule -DisplayName %s | Where-Object { $_.Enabled -eq 'True' -and $_.Action -eq 'Allow' })
$port = @(Get-NetFirewallPortFilter -Protocol TCP | Where-Object { $_.LocalPort -eq '%d' } | Get-NetFirewallRule | Where-Object { $_.Direction -eq 'Inbound' -and $_.Enabled -eq 'True' -and $_.Action -eq 'Allow' })
$pub = @(Get-NetConnectionProfile | Where-Object { $_.NetworkCategory -eq 'Public' } | ForEach-Object { $_.InterfaceAlias })
[pscustomobject]@{
  blocked = [bool]($r | Where-Object { $_.Action -eq 'Block' })
  allowed = [bool](($r | Where-Object { $_.Action -eq 'Allow' }) -or $n -or $port)
  public  = $pub
} | ConvertTo-Json -Compress
`, psQuote(exe), psQuote(FirewallRule), port)
}

// FirewallAllowScript odstráni blokujúce pravidlá pre program a povolí port
// mobilu pre program vo všetkých profiloch siete (spúšťa sa ako správca).
func FirewallAllowScript(exe string, port int) string {
	return fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$p = %s
Get-NetFirewallApplicationFilter -Program $p -ErrorAction SilentlyContinue | Get-NetFirewallRule |
  Where-Object { $_.Direction -eq 'Inbound' -and $_.Action -eq 'Block' } | Remove-NetFirewallRule
Get-NetFirewallRule -DisplayName %s -ErrorAction SilentlyContinue | Remove-NetFirewallRule
New-NetFirewallRule -DisplayName %s -Description 'Telefóny spárované s Fiškusom (iba port mobilu v sieti)' `+"`"+`
  -Direction Inbound -Action Allow -Program $p -Protocol TCP -LocalPort %d -Profile Any | Out-Null
`, psQuote(exe), psQuote(FirewallRule), psQuote(FirewallRule), port)
}

func powershell(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "powershell.exe", append([]string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass"}, args...)...)
	hideWindow(cmd)
	return cmd.Output()
}

// FirewallCheck zistí stav brány firewall pre bežiaci program a port.
func FirewallCheck(ctx context.Context, port int) FirewallInfo {
	if runtime.GOOS != "windows" {
		return FirewallInfo{}
	}
	info := FirewallInfo{Supported: true, Public: []string{}}
	exe, err := os.Executable()
	if err != nil {
		info.Error = err.Error()
		return info
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := powershell(ctx, "-Command", FirewallCheckScript(exe, port))
	if err != nil {
		info.Error = "stav brány firewall sa nepodarilo zistiť: " + err.Error()
		return info
	}
	var v struct {
		Blocked bool            `json:"blocked"`
		Allowed bool            `json:"allowed"`
		Public  json.RawMessage `json:"public"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &v); err != nil {
		info.Error = "neznáma odpoveď brány firewall"
		return info
	}
	info.Blocked, info.Allowed = v.Blocked, v.Allowed
	info.Public = parseNames(v.Public)
	return info
}

// parseNames prečíta pole alebo jeden reťazec (ConvertTo-Json z jedného prvku).
func parseNames(raw json.RawMessage) []string {
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return list
	}
	var one string
	if json.Unmarshal(raw, &one) == nil && one != "" {
		return []string{one}
	}
	return []string{}
}

// ErrFirewallCanceled — používateľ v okne UAC povolenie nepotvrdil.
var ErrFirewallCanceled = errors.New("povolenie v bráne firewall sa nepotvrdilo — v okne Windows (Kontrola používateľských kont) kliknite na Áno")

// FirewallAllow povolí Fiškus v bráne firewall (Windows sa opýta na práva správcu).
func FirewallAllow(ctx context.Context, port int) error {
	if runtime.GOOS != "windows" {
		return errors.New("brána firewall sa nastavuje iba vo Windows")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	f, err := os.CreateTemp("", "fiskus-firewall-*.ps1")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	// UTF-8 s BOM, aby Windows PowerShell 5 prečítal diakritiku
	if _, err := f.WriteString("\xef\xbb\xbf" + FirewallAllowScript(exe, port)); err != nil {
		f.Close()
		return err
	}
	f.Close()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	elevate := fmt.Sprintf(`try { $p = Start-Process powershell.exe -Verb RunAs -Wait -PassThru -WindowStyle Hidden -ArgumentList '-NoProfile','-ExecutionPolicy','Bypass','-File',%s; exit $p.ExitCode } catch { exit 1223 }`,
		psQuote(`"`+filepath.Clean(f.Name())+`"`))
	_, err = powershell(ctx, "-Command", elevate)
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1223 {
		return ErrFirewallCanceled
	}
	if err != nil {
		return fmt.Errorf("pravidlo v bráne firewall sa nepridalo: %w", err)
	}
	return nil
}
