package app

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/auth"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestIsVirtual(t *testing.T) {
	for _, tt := range []struct {
		name string
		want bool
	}{
		{"Wi-Fi", false}, {"Ethernet", false}, {"en0", false}, {"wlan0", false}, {"eth0", false}, {"enp3s0", false},
		{"vEthernet (WSL)", true}, {"vEthernet (Default Switch)", true}, {"VirtualBox Host-Only Network", true},
		{"VMware Network Adapter VMnet8", true}, {"docker0", true}, {"br-1a2b3c", true}, {"utun3", true},
		{"Tailscale", true}, {"NordLynx", true}, {"OpenVPN TAP-Windows6", true}, {"wg0", true}, {"bridge100", true},
	} {
		if got := isVirtual(tt.name); got != tt.want {
			t.Errorf("%q: %v", tt.name, got)
		}
	}
}

func TestSortLAN(t *testing.T) {
	list := []LANAddress{
		{IP: "172.24.80.1", Name: "vEthernet (WSL)", Virtual: true},
		{IP: "10.8.0.2", Name: "OpenVPN", Virtual: true},
		{IP: "172.17.0.1", Name: "docker0", Virtual: true},
		{IP: "10.0.0.5", Name: "Ethernet 2"},
		{IP: "192.168.1.20", Name: "Wi-Fi", Primary: true},
		{IP: "192.168.56.1", Name: "Ethernet 3"},
	}
	sortLAN(list)
	want := []string{"192.168.1.20", "192.168.56.1", "10.0.0.5", "10.8.0.2", "172.24.80.1", "172.17.0.1"}
	for i, w := range want {
		if list[i].IP != w {
			t.Fatalf("poradie %v", list)
		}
	}
}

func TestMobileAddress(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	a.LANAddresses = func() []string { return []string{"172.24.80.1", "192.168.1.20"} }
	ctx := auth.WithActor(ctx, auth.Local)
	if _, err := a.SetMobile(ctx, true, 0); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		in, field, first string
	}{
		{"192.168.1.20", "", "http://192.168.1.20:"},
		{" 192.168.0.50 ", "", "http://192.168.0.50:"}, // adresa, ktorú Fiškus nenašiel (pevná v routeri)
		{"8.8.8.8", "address", ""},
		{"pocitac.local", "address", ""},
		{"", "", "http://172.24.80.1:"}, // automaticky = prvá nájdená
	} {
		st, err := a.SetMobileAddress(ctx, tt.in)
		if tt.field != "" {
			if !fieldErr(err, tt.field) {
				t.Errorf("%q: %v", tt.in, err)
			}
			continue
		}
		if err != nil || len(st.URLs) == 0 || st.URLs[0][:len(tt.first)] != tt.first || len(st.Addresses) != 2 {
			t.Errorf("%q: %+v %v", tt.in, st, err)
		}
	}
	// QR kód párovania ide na vybranú adresu
	_, _ = a.SetMobileAddress(ctx, "192.168.1.20")
	p, err := a.NewMobilePairing(ctx)
	if err != nil || len(p.URLs) != 2 || p.URLs[0][:len("http://192.168.1.20:")] != "http://192.168.1.20:" {
		t.Errorf("párovanie %+v %v", p, err)
	}
}
