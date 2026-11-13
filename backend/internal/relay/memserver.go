package relay

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MemServer je schránka v pamäti s rovnakým rozhraním ako sync-relay/
// (Cloudflare Worker). Slúži na testy a na skúšanie bez internetu
// (fiskus -sync-relay-local).
type MemServer struct {
	mu       sync.Mutex
	channels map[string]*memChannel
	Now      func() time.Time
}

type memChannel struct {
	auth     [32]byte
	snapshot []byte
	snapInfo *Snapshot
	items    map[string][]byte
	info     map[string]Item
	next     int64
	want     string
}

// NewMemServer vytvorí prázdnu schránku.
func NewMemServer() *MemServer { return &MemServer{channels: map[string]*memChannel{}} }

var (
	channelRe = regexp.MustCompile(`^/v1/c/([0-9a-f]{32})(/.*)?$`)
	itemRe    = regexp.MustCompile(`^/inbox/([0-9]{12})$`)
)

func (s *MemServer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func relayErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (s *MemServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Expose-Headers", "X-Version") // prehliadač inak hlavičku verzie neukáže
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, POST, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Version")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	m := channelRe.FindStringSubmatch(r.URL.Path)
	if m == nil {
		relayErr(w, http.StatusNotFound, "neznáma adresa")
		return
	}
	auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if len(auth) < 32 {
		relayErr(w, http.StatusUnauthorized, "chýba prístupový kľúč")
		return
	}
	sum := sha256.Sum256([]byte(auth))
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := s.channels[m[1]]
	if ch == nil {
		if r.Method == http.MethodDelete && m[2] == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// prvý prístup schránku založí a zapamätá si odtlačok kľúča
		ch = &memChannel{auth: sum, items: map[string][]byte{}, info: map[string]Item{}}
		s.channels[m[1]] = ch
	} else if subtle.ConstantTimeCompare(ch.auth[:], sum[:]) != 1 {
		relayErr(w, http.StatusUnauthorized, "nesprávny prístupový kľúč")
		return
	}
	path := m[2]
	at := s.now().UTC().Format(time.RFC3339)
	body := func(limit int) ([]byte, bool) {
		b, err := io.ReadAll(io.LimitReader(r.Body, int64(limit)+1))
		if err != nil {
			relayErr(w, http.StatusBadRequest, "údaje sa neprečítali")
			return nil, false
		}
		if len(b) > limit {
			relayErr(w, http.StatusRequestEntityTooLarge, "priveľké údaje")
			return nil, false
		}
		return b, true
	}
	switch {
	case r.Method == http.MethodGet && path == "/state":
		st := State{Snapshot: ch.snapInfo, Inbox: []Item{}, Want: ch.want}
		for _, it := range ch.info {
			st.Inbox = append(st.Inbox, it)
		}
		sort.Slice(st.Inbox, func(i, j int) bool { return st.Inbox[i].ID < st.Inbox[j].ID })
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(st)
	case r.Method == http.MethodPut && path == "/snapshot":
		v, err := strconv.ParseInt(r.Header.Get("X-Version"), 10, 64)
		if err != nil || v <= 0 {
			relayErr(w, http.StatusBadRequest, "chýba verzia kópie")
			return
		}
		b, ok := body(MaxSnapshot)
		if !ok {
			return
		}
		ch.snapshot, ch.snapInfo, ch.want = b, &Snapshot{Version: v, Size: len(b), At: at}, ""
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet && path == "/snapshot":
		if ch.snapInfo == nil {
			relayErr(w, http.StatusNotFound, "kópia ešte nie je")
			return
		}
		w.Header().Set("X-Version", strconv.FormatInt(ch.snapInfo.Version, 10))
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(ch.snapshot)
	case r.Method == http.MethodPost && path == "/inbox":
		if len(ch.items) >= MaxInbox {
			relayErr(w, http.StatusTooManyRequests, "fronta je plná — zapnite Fiškus v počítači")
			return
		}
		b, ok := body(MaxItem)
		if !ok {
			return
		}
		ch.next++
		id := fmt.Sprintf("%012d", ch.next)
		ch.items[id] = b
		ch.info[id] = Item{ID: id, Size: len(b), At: at}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
	case r.Method == http.MethodPost && path == "/want":
		ch.want = at
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodDelete && path == "":
		delete(s.channels, m[1])
		w.WriteHeader(http.StatusNoContent)
	default:
		im := itemRe.FindStringSubmatch(path)
		if im == nil {
			relayErr(w, http.StatusNotFound, "neznáma adresa")
			return
		}
		b, ok := ch.items[im[1]]
		switch {
		case !ok && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent) // už zmazaný
		case !ok:
			relayErr(w, http.StatusNotFound, "záznam nie je")
		case r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(b)
		case r.Method == http.MethodDelete:
			delete(ch.items, im[1])
			delete(ch.info, im[1])
			w.WriteHeader(http.StatusNoContent)
		default:
			relayErr(w, http.StatusMethodNotAllowed, "nepovolená metóda")
		}
	}
}
