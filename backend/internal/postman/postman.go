// Package postman je všeobecné rozhranie na digitálneho poštára (prístupový
// bod siete Peppol), cez ktorého sa od 1. 1. 2027 posielajú a prijímajú
// e-faktúry a hlásia údaje finančnej správe. Konkrétny poskytovateľ sa
// pripojí cez národné rozhranie SAPI-SK, priečinok (synchronizácia aplikáciou
// poskytovateľa) alebo jednoduché HTTP API (adaptér poskytovateľa).
package postman

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Outgoing je odosielaná e-faktúra.
type Outgoing struct {
	Sender   string // identifikátor Peppol odosielateľa, napr. 0245:2120000003
	Receiver string // identifikátor Peppol príjemcu
	FileName string // napr. Faktura-FA20260041.xml
	Data     []byte // UBL XML
}

// Sent je výsledok odoslania.
type Sent struct {
	MessageID string `json:"messageId"`
	Status    string `json:"status"`
}

// Message je prijatá e-faktúra v schránke.
type Message struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Received time.Time `json:"received"`
	Data     []byte    `json:"-"`
}

// Provider je digitálny poštár.
type Provider interface {
	Send(ctx context.Context, o Outgoing) (Sent, error)
	// Inbox vráti prijaté e-faktúry, ktoré ešte neboli potvrdené.
	Inbox(ctx context.Context) ([]Message, error)
	// Ack potvrdí prevzatie (správa zo schránky zmizne).
	Ack(ctx context.Context, id string) error
}

// Checker vie overiť spojenie bez odoslania a prevzatia dokumentov.
type Checker interface {
	Check(ctx context.Context) error
}

// Delivery je stav odoslanej e-faktúry u poštára.
type Delivery struct {
	Status    string // napr. accepted, sent, delivered, failed, rejected, undeliverable
	Delivered string // čas doručenia (RFC 3339), "" = zatiaľ nie
	Error     string // dôvod pri zlyhaní
}

// Tracker vie zistiť stav odoslanej e-faktúry.
type Tracker interface {
	Delivery(ctx context.Context, messageID string) (Delivery, error)
}

// ErrNoTracking — poskytovateľ stav doručenia cez toto rozhranie neposkytuje.
var ErrNoTracking = errors.New("poštár stav doručenia cez toto rozhranie neposkytuje — pozrite ho v jeho portáli")

// Druhy pripojenia.
const (
	ModeOff    = ""       // vypnuté: e-faktúra sa iba stiahne
	ModeFolder = "folder" // priečinky odoslané/prijaté
	ModeHTTP   = "http"   // HTTP API adaptéra poskytovateľa
	ModeSAPI   = "sapi"   // národné rozhranie SAPI-SK priamo u poskytovateľa
)

// Settings je nastavenie pripojenia firmy k poštárovi.
type Settings struct {
	Mode     string `json:"mode"`
	OutDir   string `json:"outDir"`          // folder: sem Fiškus ukladá odosielané e-faktúry
	InDir    string `json:"inDir"`           // folder: odtiaľto číta prijaté e-faktúry
	URL      string `json:"url"`             // http, sapi: základná adresa API
	ClientID string `json:"clientId"`        // sapi: identifikátor klienta API
	Token    string `json:"token,omitempty"` // http: token; sapi: tajomstvo klienta (client_secret)
	TokenSet bool   `json:"tokenSet"`        // iba na zobrazenie — token sa nevracia
	Provider string `json:"provider"`        // názov poskytovateľa (iba na zobrazenie)
	// Participant je 0245:DIČ firmy — dopĺňa ho program, neukladá sa.
	Participant string `json:"-"`
}

// ErrOff — pripojenie k poštárovi nie je nastavené.
var ErrOff = errors.New("pripojenie k digitálnemu poštárovi nie je nastavené (Nastavenia → E-faktúry)")

// Problem je chyba nastavenia pri poli.
type Problem struct {
	Field, Message string
}

// SettingsError zoskupuje chyby nastavenia.
type SettingsError struct{ Problems []Problem }

func (e *SettingsError) Error() string {
	var m []string
	for _, p := range e.Problems {
		m = append(m, p.Message)
	}
	return "neplatné nastavenie e-faktúr: " + strings.Join(m, "; ")
}

// Normalize očistí nastavenie.
func (s *Settings) Normalize() {
	s.Mode = strings.TrimSpace(s.Mode)
	s.OutDir, s.InDir = strings.TrimSpace(s.OutDir), strings.TrimSpace(s.InDir)
	s.URL = strings.TrimRight(strings.TrimSpace(s.URL), "/")
	s.ClientID = strings.TrimSpace(s.ClientID)
	s.Provider = strings.TrimSpace(s.Provider)
}

// Validate skontroluje nastavenie.
func (s Settings) Validate() error {
	var ps []Problem
	switch s.Mode {
	case ModeOff:
	case ModeFolder:
		if s.OutDir == "" {
			ps = append(ps, Problem{"outDir", "Zadajte priečinok na odosielané e-faktúry"})
		}
		if s.InDir == "" {
			ps = append(ps, Problem{"inDir", "Zadajte priečinok s prijatými e-faktúrami"})
		}
		if s.OutDir != "" && s.OutDir == s.InDir {
			ps = append(ps, Problem{"inDir", "Priečinok prijatých musí byť iný ako odosielaných"})
		}
	case ModeHTTP, ModeSAPI:
		if !strings.HasPrefix(s.URL, "https://") && !strings.HasPrefix(s.URL, "http://127.0.0.1") && !strings.HasPrefix(s.URL, "http://localhost") {
			ps = append(ps, Problem{"url", "Adresa API musí začínať https:// (http iba na tomto počítači)"})
		}
		if s.Mode == ModeSAPI && s.ClientID == "" {
			ps = append(ps, Problem{"clientId", "Zadajte Client ID od poskytovateľa"})
		}
		if s.Token == "" {
			msg := "Zadajte prístupový token od poskytovateľa"
			if s.Mode == ModeSAPI {
				msg = "Zadajte Client secret od poskytovateľa"
			}
			ps = append(ps, Problem{"token", msg})
		}
	default:
		ps = append(ps, Problem{"mode", "Neznámy spôsob pripojenia"})
	}
	if len(ps) > 0 {
		return &SettingsError{ps}
	}
	return nil
}

// New vytvorí poštára podľa nastavenia.
func New(s Settings) (Provider, error) {
	switch s.Mode {
	case ModeFolder:
		return Folder{Out: s.OutDir, In: s.InDir}, nil
	case ModeHTTP:
		return HTTP{BaseURL: s.URL, Token: s.Token}, nil
	case ModeSAPI:
		if s.Participant == "" {
			return nil, errors.New("firma nemá DIČ — bez neho nie je adresa v sieti Peppol (0245:DIČ)")
		}
		return SAPI{BaseURL: s.URL, ClientID: s.ClientID, ClientSecret: s.Token, Participant: s.Participant}, nil
	case ModeOff:
		return nil, ErrOff
	}
	return nil, fmt.Errorf("neznámy spôsob pripojenia %q", s.Mode)
}

// PeppolID vráti identifikátor účastníka zo slovenského DIČ (0245:DIČ).
func PeppolID(dic string) string {
	var b strings.Builder
	for _, r := range dic {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return "0245:" + b.String()
}
