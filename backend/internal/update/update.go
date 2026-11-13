// Package update: automatická aktualizácia nainštalovaného Fiškusa.
//
// Po spustení aplikácie sa zistí vydanie s najvyššou verziou na GitHube
// (nie to, ktoré GitHub označí za posledné: omylom vydaná nižšia značka
// by inak aktualizácie zastavila). Ak je novšie,
// stiahne sa súbor zodpovedajúci spôsobu inštalácie (inštalátor pre
// Windows, .dmg pre macOS, balík deb/rpm alebo samotný program), overí sa
// jeho súčet SHA-256 a program sa vymení. Starý Fiškus sa potom riadne
// ukončí (záverečná záloha) a nový sa spustí; otvorené okno sa samo
// znova načíta.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultURL je zoznam posledných vydaní Fiškusa; vyberie sa najvyššia
// verzia bez predbežných vydaní a konceptov.
const DefaultURL = "https://api.github.com/repos/fiskus-online/fiskus/releases?per_page=30"

// Page je stránka so stiahnutím pre ručnú aktualizáciu.
const Page = "https://fiskus.online/#stiahnut"

// maxAsset obmedzí veľkosť sťahovaného súboru.
const maxAsset = 400 << 20

// Release je vydanie z GitHubu.
type Release struct {
	Version string
	Notes   string
	Page    string
	Assets  []Asset
}

// Asset je súbor vydania.
type Asset struct {
	Name   string
	URL    string
	Size   int64
	Digest string // „sha256:…“ (GitHub ho počíta pri nahraní)
}

// rawRelease je vydanie v odpovedi GitHubu.
type rawRelease struct {
	Tag    string `json:"tag_name"`
	Body   string `json:"body"`
	Page   string `json:"html_url"`
	Draft  bool   `json:"draft"`
	Pre    bool   `json:"prerelease"`
	Assets []struct {
		Name   string `json:"name"`
		URL    string `json:"browser_download_url"`
		Size   int64  `json:"size"`
		Digest string `json:"digest"`
	} `json:"assets"`
}

// Check zistí vydanie s najvyššou verziou. Adresa môže vrátiť zoznam
// vydaní (/releases) aj jedno vydanie (/releases/latest).
func Check(ctx context.Context, client *http.Client, url string) (Release, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := client.Do(req)
	if err != nil {
		return Release{}, errors.New("nepodarilo sa spojiť s GitHubom (internet?)")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub odpovedal %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return Release{}, errors.New("nečitateľná odpoveď GitHubu")
	}
	var list []rawRelease
	if err := json.Unmarshal(body, &list); err != nil {
		var one rawRelease
		if err := json.Unmarshal(body, &one); err != nil {
			return Release{}, errors.New("nečitateľná odpoveď GitHubu")
		}
		list = []rawRelease{one}
	}
	raw, ok := highest(list)
	if !ok {
		return Release{}, errors.New("posledné vydanie nemá platnú verziu")
	}
	r := Release{Version: trimV(raw.Tag), Notes: raw.Body, Page: raw.Page}
	for _, a := range raw.Assets {
		r.Assets = append(r.Assets, Asset{Name: a.Name, URL: a.URL, Size: a.Size, Digest: a.Digest})
	}
	return r, nil
}

// highest vyberie vydanie s najvyššou verziou (bez konceptov, predbežných
// vydaní a značiek v inom tvare ako v1.2.3).
func highest(list []rawRelease) (rawRelease, bool) {
	var best rawRelease
	found := false
	for _, r := range list {
		if r.Draft || r.Pre || parse(r.Tag) == nil {
			continue
		}
		if !found || Newer(r.Tag, best.Tag) {
			best, found = r, true
		}
	}
	return best, found
}

// trimV odstráni predponu „v“ alebo „V“ (značka V0.10.0 je tá istá verzia ako v0.10.0).
func trimV(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "v") || strings.HasPrefix(v, "V") {
		return v[1:]
	}
	return v
}

// parse rozloží „v1.2.3“ (aj „V1.2.3“) na čísla; nil pri inom tvare (aj „dev“, „1.2.3-rc1“).
func parse(v string) []int {
	parts := strings.Split(trimV(v), ".")
	if len(parts) != 3 {
		return nil
	}
	out := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil
		}
		out[i] = n
	}
	return out
}

// Newer: je verzia latest novšia ako current? Vývojová zostava („dev“)
// sa neaktualizuje nikdy.
func Newer(latest, current string) bool {
	l, c := parse(latest), parse(current)
	if l == nil || c == nil {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}
