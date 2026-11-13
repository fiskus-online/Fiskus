// Package relay: synchronizácia telefónu s Fiškusom v počítači cez schránku
// na internete. Schránka (sync-relay/, Cloudflare Worker) iba drží
// zašifrované dáta: kópiu databázy z počítača a frontu záznamov z telefónu.
// Kľúč pozná iba počítač a telefón — je v odkaze z párovacieho QR kódu
// (za znakom #, takže ho prehliadač neposiela na server).
//
// Z jedného 32-bajtového tajomstva sa odvodí:
//   - kanál (verejné meno schránky, 32 hex znakov),
//   - prístupový kľúč ku schránke (server si pamätá iba jeho odtlačok),
//   - kľúč AES-256-GCM na šifrovanie obsahu.
package relay

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// SecretSize je dĺžka tajomstva spárovania v bajtoch.
const SecretSize = 32

// Druhy zašifrovaného obsahu (sú súčasťou overovaných údajov, takže kópiu
// databázy nemožno podstrčiť ako záznam a naopak).
const (
	KindSnapshot = "snapshot"
	KindItem     = "item"
)

const sealVersion = 1

// ErrSealed: obsah sa nedá rozšifrovať (iný kľúč, poškodené dáta).
var ErrSealed = errors.New("údaje zo schránky sa nedajú rozšifrovať — spárujte telefón znova")

// Keys sú kľúče jedného spárovania.
type Keys struct {
	Channel string // meno schránky
	Auth    string // prístupový kľúč ku schránke
	aead    cipher.AEAD
}

// NewSecret vytvorí náhodné tajomstvo spárovania.
func NewSecret() ([]byte, error) {
	s := make([]byte, SecretSize)
	_, err := rand.Read(s)
	return s, err
}

func derive(secret []byte, label string) []byte {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte("fiskus-sync/" + label))
	return m.Sum(nil)
}

// Derive odvodí kľúče z tajomstva.
func Derive(secret []byte) (Keys, error) {
	if len(secret) != SecretSize {
		return Keys{}, fmt.Errorf("tajomstvo spárovania má mať %d bajtov", SecretSize)
	}
	block, err := aes.NewCipher(derive(secret, "key"))
	if err != nil {
		return Keys{}, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return Keys{}, err
	}
	return Keys{
		Channel: hex.EncodeToString(derive(secret, "channel"))[:32],
		Auth:    base64.RawURLEncoding.EncodeToString(derive(secret, "auth")),
		aead:    aead,
	}, nil
}

func (k Keys) aad(kind string) []byte {
	return []byte("fiskus-sync/v1/" + kind + "/" + k.Channel)
}

// Seal zašifruje obsah: verzia (1 bajt) | nonce (12) | šifra s tagom.
func (k Keys) Seal(kind string, plain []byte) ([]byte, error) {
	if k.aead == nil {
		return nil, errors.New("kľúče spárovania nie sú pripravené")
	}
	out := make([]byte, 1+k.aead.NonceSize(), 1+k.aead.NonceSize()+len(plain)+k.aead.Overhead())
	out[0] = sealVersion
	if _, err := rand.Read(out[1:]); err != nil {
		return nil, err
	}
	return k.aead.Seal(out, out[1:], plain, k.aad(kind)), nil
}

// Open rozšifruje obsah zo Seal.
func (k Keys) Open(kind string, sealed []byte) ([]byte, error) {
	if k.aead == nil {
		return nil, errors.New("kľúče spárovania nie sú pripravené")
	}
	n := k.aead.NonceSize()
	if len(sealed) < 1+n+k.aead.Overhead() || sealed[0] != sealVersion {
		return nil, ErrSealed
	}
	plain, err := k.aead.Open(nil, sealed[1:1+n], sealed[1+n:], k.aad(kind))
	if err != nil {
		return nil, ErrSealed
	}
	return plain, nil
}

// LinkParam je parameter v časti # odkazu na pripojenie telefónu.
const LinkParam = "pripojit"

// PairLink je odkaz, ktorý telefón otvorí z QR kódu: adresa Fiškusa
// v prehliadači a za # tajomstvo, meno počítača a (voliteľne) adresa vlastnej
// schránky. Časť za # sa na server neposiela.
func PairLink(app string, secret []byte, name, relayURL string) string {
	v := url.Values{}
	v.Set("k", base64.RawURLEncoding.EncodeToString(secret))
	v.Set("n", name)
	if relayURL != "" {
		v.Set("s", relayURL)
	}
	return strings.TrimRight(app, "/") + "/#" + LinkParam + "&" + v.Encode()
}

// Pairing sú údaje z odkazu na pripojenie.
type Pairing struct {
	Secret []byte
	Name   string // meno počítača
	Relay  string // vlastná schránka (prázdne = predvolená)
}

// ParsePairing prečíta údaje z časti odkazu za # (aj s úvodným „pripojit&“).
func ParsePairing(fragment string) (Pairing, error) {
	fragment = strings.TrimPrefix(strings.TrimPrefix(fragment, "#"), LinkParam+"&")
	v, err := url.ParseQuery(fragment)
	if err != nil {
		return Pairing{}, errors.New("odkaz na pripojenie je poškodený")
	}
	secret, err := base64.RawURLEncoding.DecodeString(v.Get("k"))
	if err != nil || len(secret) != SecretSize {
		return Pairing{}, errors.New("odkaz na pripojenie je neúplný — naskenujte QR kód znova")
	}
	p := Pairing{Secret: secret, Name: strings.TrimSpace(v.Get("n")), Relay: strings.TrimSpace(v.Get("s"))}
	if p.Relay != "" {
		if err := ValidURL(p.Relay); err != nil {
			return Pairing{}, err
		}
	}
	if r := []rune(p.Name); len(r) > 60 {
		p.Name = string(r[:60])
	}
	return p, nil
}

// ValidURL overí adresu schránky (https, výnimka iba pre tento počítač pri skúšaní).
func ValidURL(s string) error {
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("adresa schránky nie je platná (https://…)")
	}
	local := u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost"
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return errors.New("schránka musí mať adresu https://")
	}
	return nil
}
