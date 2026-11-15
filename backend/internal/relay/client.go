package relay

import (
	"bytes"
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

// DefaultURL je schránka Fiškusa (sync-relay/ nasadený na Cloudflare).
const DefaultURL = "https://sync.fiskus.online"

// Limity schránky (rovnaké v sync-relay/src/core.js).
const (
	MaxSnapshot = 32 << 20   // kópia databázy (zašifrovaná)
	MaxItem     = 1536 << 10 // jeden záznam z telefónu (aj bloček s fotkou)
	MaxInbox    = 500        // záznamov čakajúcich na počítač
)

// Chyby schránky.
var (
	ErrUnauthorized = errors.New("schránka odmietla prístup — spárujte telefón znova")
	ErrNoSnapshot   = errors.New("počítač ešte neposlal kópiu údajov")
	ErrBusy         = errors.New("schránka je dnes preťažená — synchronizácia sa zopakuje neskôr")
)

// Snapshot je údaj o kópii databázy v schránke.
type Snapshot struct {
	Version int64  `json:"version"` // čas vytvorenia v ms (rastie)
	Size    int    `json:"size"`
	At      string `json:"at"`
}

// Item je záznam vo fronte schránky.
type Item struct {
	ID   string `json:"id"` // poradie (rastie)
	Size int    `json:"size"`
	At   string `json:"at"`
}

// State je stav schránky.
type State struct {
	Snapshot *Snapshot `json:"snapshot"`
	Inbox    []Item    `json:"inbox"`
	Want     string    `json:"want,omitempty"` // telefón si pýta čerstvú kópiu
}

// Client volá schránku jedného spárovania.
type Client struct {
	Base string
	Keys Keys
	HTTP *http.Client
}

func (c Client) do(ctx context.Context, method, path string, body []byte, hdr map[string]string) (*http.Response, error) {
	base := c.Base
	if base == "" {
		base = DefaultURL
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(base, "/")+"/v1/c/"+c.Keys.Channel+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Keys.Auth)
	if body != nil {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 2 * time.Minute}
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("schránka nie je dostupná: %w", err)
	}
	if res.StatusCode >= 300 {
		defer res.Body.Close()
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&e)
		switch {
		case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
			return nil, ErrUnauthorized
		case res.StatusCode == http.StatusNotFound && path == "/snapshot":
			return nil, ErrNoSnapshot
		case res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500 && e.Error == "":
			return nil, ErrBusy
		case e.Error != "":
			return nil, fmt.Errorf("schránka: %s", e.Error)
		}
		return nil, fmt.Errorf("schránka vrátila %s", res.Status)
	}
	return res, nil
}

func (c Client) json(ctx context.Context, method, path string, body []byte, hdr map[string]string, out any) error {
	res, err := c.do(ctx, method, path, body, hdr)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
		return nil
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out)
}

// State vráti stav schránky.
func (c Client) State(ctx context.Context) (State, error) {
	var st State
	err := c.json(ctx, http.MethodGet, "/state", nil, nil, &st)
	return st, err
}

// PutSnapshot uloží zašifrovanú kópiu databázy.
func (c Client) PutSnapshot(ctx context.Context, version int64, sealed []byte) error {
	if len(sealed) > MaxSnapshot {
		return fmt.Errorf("kópia údajov má %d MB, schránka prijme najviac %d MB", len(sealed)>>20, MaxSnapshot>>20)
	}
	return c.json(ctx, http.MethodPut, "/snapshot", sealed, map[string]string{"X-Version": strconv.FormatInt(version, 10)}, nil)
}

// GetSnapshot stiahne zašifrovanú kópiu databázy a jej verziu.
func (c Client) GetSnapshot(ctx context.Context) ([]byte, int64, error) {
	res, err := c.do(ctx, http.MethodGet, "/snapshot", nil, nil)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, MaxSnapshot+1))
	if err != nil {
		return nil, 0, err
	}
	if len(b) > MaxSnapshot {
		return nil, 0, errors.New("kópia údajov v schránke je priveľká")
	}
	v, _ := strconv.ParseInt(res.Header.Get("X-Version"), 10, 64)
	return b, v, nil
}

// PostItem pridá zašifrovaný záznam do fronty a vráti jeho id.
func (c Client) PostItem(ctx context.Context, sealed []byte) (string, error) {
	if len(sealed) > MaxItem {
		return "", fmt.Errorf("záznam je priveľký (%d kB, najviac %d kB)", len(sealed)>>10, MaxItem>>10)
	}
	var out struct {
		ID string `json:"id"`
	}
	err := c.json(ctx, http.MethodPost, "/inbox", sealed, nil, &out)
	return out.ID, err
}

// GetItem stiahne záznam z fronty.
func (c Client) GetItem(ctx context.Context, id string) ([]byte, error) {
	res, err := c.do(ctx, http.MethodGet, "/inbox/"+id, nil, nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return io.ReadAll(io.LimitReader(res.Body, MaxItem+1))
}

// DeleteItem odstráni spracovaný záznam z fronty.
func (c Client) DeleteItem(ctx context.Context, id string) error {
	return c.json(ctx, http.MethodDelete, "/inbox/"+id, nil, nil, nil)
}

// Want: telefón si pýta čerstvú kópiu (počítač ju pošle pri ďalšej kontrole).
func (c Client) Want(ctx context.Context) error {
	return c.json(ctx, http.MethodPost, "/want", nil, nil, nil)
}

// Wipe zmaže celú schránku (zrušenie spárovania).
func (c Client) Wipe(ctx context.Context) error {
	return c.json(ctx, http.MethodDelete, "", nil, nil, nil)
}
