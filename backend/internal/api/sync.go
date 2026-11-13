package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/fiskus-online/fiskus/backend/internal/app"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
)

// Telefón mimo domu (app/sync.go): rozhranie na spárovanie a stav,
// obmedzenie zápisov v kópii na telefóne a prehrávanie záznamov z telefónu
// v počítači.

type syncRoutes struct {
	mu  sync.Mutex
	mux *http.ServeMux
	h   http.Handler
}

func (s *Server) routeSync(open func(string, http.HandlerFunc)) {
	open("GET /api/sync", s.syncStatus)
	open("POST /api/sync/run", s.syncRun)
	// počítač
	open("POST /api/sync/phones", s.syncPair)
	open("DELETE /api/sync/phones/{id}", s.syncRemovePhone)
	open("PUT /api/sync/relay", s.syncSetRelay)
	// telefón
	open("POST /api/sync/join", s.syncJoin)
	open("POST /api/sync/leave", s.syncLeave)
	open("DELETE /api/sync/failed/{id}", s.syncDismiss)
}

// syncHandler: na telefóne s kópiou z počítača pustí iba povolené zápisy
// a zapíše ich do fronty; ostatné odmietne. Pripraví prehrávanie záznamov.
func (s *Server) syncHandler(mux *http.ServeMux) http.Handler {
	var h http.Handler = mux
	if s.App.Caps.Browser {
		h = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || !strings.HasPrefix(r.URL.Path, "/api/") || !s.App.SyncIsPhone(r.Context()) {
				mux.ServeHTTP(w, r)
				return
			}
			_, pattern := mux.Handler(r)
			if app.SyncLocalOnly(pattern) {
				mux.ServeHTTP(w, r)
				return
			}
			if _, ok := app.SyncWritable(pattern); !ok {
				writeErr(w, http.StatusConflict, app.ErrPhoneCopy.Error(), nil)
				return
			}
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
			if err != nil {
				writeErr(w, http.StatusRequestEntityTooLarge, "Záznam je priveľký", nil)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, r)
			if rec.Code >= 200 && rec.Code < 300 {
				if err := s.App.SyncCapture(r.Context(), pattern, r.Method, r.URL.RequestURI(), body, rec.Body.Bytes()); err != nil {
					writeErr(w, http.StatusInternalServerError, "Záznam sa uložil iba v telefóne, do fronty pre počítač nie: "+err.Error(), nil)
					return
				}
			}
			for k, v := range rec.Header() {
				w.Header()[k] = v
			}
			w.WriteHeader(rec.Code)
			_, _ = w.Write(rec.Body.Bytes())
		})
	}
	s.sync.mu.Lock()
	s.sync.mux, s.sync.h = mux, h
	s.sync.mu.Unlock()
	s.App.Replay = s.replay
	s.App.SyncPattern = s.pattern
	return h
}

// pattern vráti vzor cesty, ktorý by požiadavku vybavil.
func (s *Server) pattern(method, path string) string {
	s.sync.mu.Lock()
	mux := s.sync.mux
	s.sync.mu.Unlock()
	r, err := http.NewRequest(method, path, nil)
	if err != nil || mux == nil {
		return ""
	}
	_, p := mux.Handler(r)
	return p
}

// replay vykoná požiadavku v mene actora (záznam z telefónu) — rovnako
// ako požiadavka zo spárovaného telefónu v sieti (aj s jej obmedzeniami).
func (s *Server) replay(ctx context.Context, actor auth.Actor, method, path string, body []byte) (int, []byte) {
	s.sync.mu.Lock()
	mux := s.sync.mux
	s.sync.mu.Unlock()
	if mux == nil {
		return http.StatusServiceUnavailable, nil
	}
	r, err := http.NewRequestWithContext(context.WithValue(ctx, lanKey{}, actor), method, path, bytes.NewReader(body))
	if err != nil {
		return http.StatusBadRequest, nil
	}
	if lanDenied(r) {
		return http.StatusForbidden, []byte(`{"error":"Z telefónu sa toto robí na počítači"}`)
	}
	r.Header.Set(CSRFHeader, "1")
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr = "127.0.0.1:0"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	return rec.Code, rec.Body.Bytes()
}

func (s *Server) syncStatus(w http.ResponseWriter, r *http.Request) {
	st, err := s.App.SyncStatus(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) syncRun(w http.ResponseWriter, r *http.Request) {
	runErr := s.App.SyncRun(r.Context())
	st, err := s.App.SyncStatus(r.Context())
	if err != nil {
		syncFail(w, err)
		return
	}
	if runErr != nil && st.Role == "" {
		syncFail(w, runErr)
		return
	}
	writeJSON(w, http.StatusOK, st) // chyba spojenia je v stave (lastError)
}

func (s *Server) syncPair(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	p, err := s.App.SyncPair(r.Context(), in.Name)
	if err != nil {
		syncFail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) syncRemovePhone(w http.ResponseWriter, r *http.Request) {
	st, err := s.App.SyncRemovePhone(r.Context(), r.PathValue("id"))
	if err != nil {
		syncFail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) syncSetRelay(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL string `json:"url"`
	}
	if !decode(w, r, &in) {
		return
	}
	st, err := s.App.SetSyncRelay(r.Context(), in.URL)
	if err != nil {
		syncFail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) syncJoin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Link string `json:"link"`
	}
	if !decode(w, r, &in) {
		return
	}
	link := in.Link
	if _, frag, ok := strings.Cut(link, "#"); ok {
		link = frag
	}
	st, err := s.App.SyncJoin(r.Context(), link)
	if err != nil {
		syncFail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) syncLeave(w http.ResponseWriter, r *http.Request) {
	if err := s.App.SyncLeave(r.Context()); err != nil {
		syncFail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) syncDismiss(w http.ResponseWriter, r *http.Request) {
	st, err := s.App.SyncDismiss(r.Context(), r.PathValue("id"))
	if err != nil {
		syncFail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func syncFail(w http.ResponseWriter, err error) {
	switch {
	case err == app.ErrSyncOff:
		writeErr(w, http.StatusNotFound, err.Error(), nil)
	default:
		fail(w, err)
	}
}
