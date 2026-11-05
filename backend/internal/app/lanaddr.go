package app

import (
	"net"
	"sort"
	"strings"
	"time"
)

// LANAddress je adresa počítača v miestnej sieti na spárovanie telefónu.
type LANAddress struct {
	IP      string `json:"ip"`
	Name    string `json:"name"`    // názov sieťovej karty
	Primary bool   `json:"primary"` // karta, cez ktorú ide počítač do siete
	Virtual bool   `json:"virtual"` // virtuálna karta (WSL, Hyper-V, VPN…) — telefón ju nevidí
}

// virtualNames sú časti názvov virtuálnych a VPN kariet (Windows, macOS,
// Linux). Takú adresu telefón v domácej sieti nedosiahne.
var virtualNames = []string{
	"vethernet", "hyper-v", "wsl", "virtualbox", "vbox", "vmware", "vmnet", "docker", "br-", "veth", "virbr",
	"tailscale", "zerotier", "wireguard", "wg", "tun", "tap", "utun", "openvpn", "nordlynx", "proton", "vpn",
	"npcap", "loopback", "bluetooth", "llw", "awdl", "anpi", "bridge", "parallels", "vnic", "ham",
}

func isVirtual(name string) bool {
	n := strings.ToLower(name)
	for _, v := range virtualNames {
		if strings.HasPrefix(n, v) || strings.Contains(n, " "+v) || strings.Contains(n, "("+v) || strings.Contains(n, v+" ") {
			return true
		}
	}
	return false
}

// primaryIP je adresa karty, cez ktorú by išla správa do internetu
// (UDP „spojenie“ nič neodošle, iba vyberie trasu). Bez siete "".
func primaryIP() string {
	for _, target := range []string{"8.8.8.8:53", "192.168.255.255:9"} {
		c, err := net.DialTimeout("udp4", target, 500*time.Millisecond)
		if err != nil {
			continue
		}
		ip := c.LocalAddr().(*net.UDPAddr).IP.String()
		c.Close()
		return ip
	}
	return ""
}

// ListLANAddresses vráti súkromné IPv4 adresy zoradené tak, aby prvá bola
// tá, ktorú telefón v rovnakej Wi-Fi najskôr dosiahne.
func ListLANAddresses() []LANAddress {
	var out []LANAddress
	ifs, err := net.Interfaces()
	if err != nil {
		return out
	}
	prim := primaryIP()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, ad := range addrs {
			n, ok := ad.(*net.IPNet)
			if !ok || n.IP.To4() == nil || !n.IP.IsPrivate() {
				continue
			}
			ip := n.IP.String()
			out = append(out, LANAddress{IP: ip, Name: i.Name, Primary: ip == prim,
				Virtual: isVirtual(i.Name) || i.Flags&net.FlagPointToPoint != 0})
		}
	}
	sortLAN(out)
	return out
}

// lanRank: hlavná karta, potom skutočné karty (192.168 > 10 > 172), virtuálne nakoniec.
func lanRank(a LANAddress) int {
	r := 0
	switch {
	case a.Primary && !a.Virtual:
		return 0
	case a.Virtual:
		r = 100
	}
	switch {
	case strings.HasPrefix(a.IP, "192.168."):
		r += 10
	case strings.HasPrefix(a.IP, "10."):
		r += 20
	default: // 172.16–31 — často Docker a WSL
		r += 30
	}
	return r
}

func sortLAN(list []LANAddress) {
	sort.SliceStable(list, func(i, j int) bool { return lanRank(list[i]) < lanRank(list[j]) })
}
