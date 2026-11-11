// Package edition určuje, ktoré funkcie sú povolené v danom režime behu.
//
// Rozhoduje backend — frontend sa len pýta /api/capabilities a podľa toho
// zobrazí alebo skryje prvky. Obísť sa to teda nedá úpravou rozhrania.
//
// Fiškus nemá licencie ani obmedzenie počtu firiem.
// Režimy sa líšia iba tým, kde program beží (desktop, server, prehliadač).
package edition

import "fmt"

// Mode je režim behu programu.
type Mode string

const (
	Desktop Mode = "desktop" // lokálna inštalácia
	Server  Mode = "server"  // beh na serveri, prístup cez prehliadač
)

// Capabilities sú povolené funkcie.
type Capabilities struct {
	Mode         Mode `json:"mode"`
	MultiCompany bool `json:"multiCompany"` // dajú sa zakladať ďalšie firmy (vždy)
	// Browser: Fiškus beží v prehliadači (údaje v úložisku prehliadača).
	// Je to desktopová verzia bez funkcií, ktoré potrebujú inštaláciu.
	Browser bool `json:"browser,omitempty"`
	// FioProxy: v prehliadači je dostupné sťahovanie z Fio cez funkciu na
	// adrese aplikácie (desktop a server sa pripájajú priamo).
	FioProxy bool `json:"fioProxy,omitempty"`
	// Demo: prázdna inštalácia ponúkne ukážkovú firmu na vyskúšanie.
	Demo bool `json:"demo,omitempty"`
}

// For vráti schopnosti pre daný režim.
func For(m Mode) (Capabilities, error) {
	switch m {
	case Desktop, Server:
		return Capabilities{Mode: m, MultiCompany: true}, nil
	}
	return Capabilities{}, fmt.Errorf("neznámy režim %q (desktop | server)", m)
}

// ForBrowser vráti schopnosti Fiškusa v prehliadači.
func ForBrowser() Capabilities {
	c, _ := For(Desktop)
	c.Browser = true
	return c
}
