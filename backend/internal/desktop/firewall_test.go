package desktop

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

func TestFirewallScripts(t *testing.T) {
	exe := `C:\Users\Ján O'Neil\AppData\Local\Programs\Fiskus\fiskus.exe`
	check := FirewallCheckScript(exe, 48081)
	allow := FirewallAllowScript(exe, 48081)
	for _, s := range []string{check, allow} {
		if !strings.Contains(s, `'C:\Users\Ján O''Neil\AppData\Local\Programs\Fiskus\fiskus.exe'`) {
			t.Errorf("cesta v úvodzovkách:\n%s", s)
		}
	}
	if !strings.Contains(check, "LocalPort -eq '48081'") || !strings.Contains(check, "ConvertTo-Json") {
		t.Error("kontrola portu")
	}
	for _, want := range []string{"Remove-NetFirewallRule", "-Action Allow", "-LocalPort 48081", "-Profile Any", "Action -eq 'Block'"} {
		if !strings.Contains(allow, want) {
			t.Errorf("povolenie: chýba %q", want)
		}
	}
	tests := []struct {
		raw  string
		want []string
	}{
		{`["Wi-Fi","Ethernet"]`, []string{"Wi-Fi", "Ethernet"}},
		{`"Wi-Fi"`, []string{"Wi-Fi"}},
		{`null`, []string{}},
	}
	for _, tt := range tests {
		got := parseNames(json.RawMessage(tt.raw))
		if strings.Join(got, ",") != strings.Join(tt.want, ",") {
			t.Errorf("%s → %v", tt.raw, got)
		}
	}
	if runtime.GOOS != "windows" {
		if FirewallCheck(context.Background(), 48081).Supported {
			t.Error("mimo Windows nie je")
		}
		if FirewallAllow(context.Background(), 48081) == nil {
			t.Error("mimo Windows chyba")
		}
	}
}
