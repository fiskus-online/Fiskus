package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/household"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Domácnosť: /api/companies/{cid}/household/…

func (s *Server) hhAccounts(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	out, err := s.App.HouseholdAccounts(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) hhSaveAccount(w http.ResponseWriter, r *http.Request) {
	saveEntity(s, w, r, func(a *household.Account, id int64) { a.ID = id }, func(cid int64, a household.Account) (household.Account, error) {
		return s.App.SaveHouseholdAccount(r.Context(), cid, a)
	})
}

func (s *Server) hhDeleteAccount(w http.ResponseWriter, r *http.Request) {
	s.deleteEntity(w, r, func(cid, id int64) error { return s.App.DeleteHouseholdAccount(r.Context(), cid, id) })
}

func (s *Server) hhEntries(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	out, err := s.App.HouseholdEntries(r.Context(), cid, s.yearParam(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) hhSaveEntry(w http.ResponseWriter, r *http.Request) {
	saveEntity(s, w, r, func(e *household.Entry, id int64) { e.ID = id }, func(cid int64, e household.Entry) (household.Entry, error) {
		return s.App.SaveHouseholdEntry(r.Context(), cid, e)
	})
}

func (s *Server) hhDeleteEntry(w http.ResponseWriter, r *http.Request) {
	s.deleteEntity(w, r, func(cid, id int64) error { return s.App.DeleteHouseholdEntry(r.Context(), cid, id) })
}

func (s *Server) hhCategories(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	out, err := s.App.HouseholdCategories(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) hhSaveCategories(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var in []household.Category
	if !decode(w, r, &in) {
		return
	}
	out, err := s.App.SaveHouseholdCategories(r.Context(), cid, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// hhSummary: GET …/household/summary?year=2026&month=10
func (s *Server) hhSummary(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	month, _ := strconv.Atoi(r.URL.Query().Get("month"))
	out, err := s.App.HouseholdSummary(r.Context(), cid, s.yearParam(r), month)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// hhSuggest: GET …/household/suggest?text=LIDL&income=0
func (s *Server) hhSuggest(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	cat, err := s.App.HouseholdSuggest(r.Context(), cid, q.Get("text"), q.Get("income") == "1")
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"category": cat})
}

// hhImportBank: POST …/household/accounts/{id}/import (telo = výpis)
func (s *Server) hhImportBank(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxStatement))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "Súbor je príliš veľký (najviac 20 MB)", nil)
		return
	}
	res, err := s.App.ImportHouseholdBank(r.Context(), cid, id, data)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) hhDraws(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	out, err := s.App.BusinessDraws(r.Context(), cid, s.yearParam(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// hhImportDraws: POST …/household/draws {accountId, year, items: [{companyId, key}]}
func (s *Server) hhImportDraws(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var in struct {
		AccountID int64         `json:"accountId"`
		Year      int           `json:"year"`
		Items     []app.DrawRef `json:"items"`
	}
	if !decode(w, r, &in) {
		return
	}
	res, err := s.App.ImportBusinessDraws(r.Context(), cid, in.AccountID, in.Year, in.Items)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// ---- záväzky ----

func (s *Server) hhObligations(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	out, err := s.App.HouseholdObligations(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) hhObligation(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	out, err := s.App.HouseholdObligation(r.Context(), cid, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) hhSaveObligation(w http.ResponseWriter, r *http.Request) {
	saveEntity(s, w, r, func(o *household.Obligation, id int64) { o.ID = id }, func(cid int64, o household.Obligation) (household.Obligation, error) {
		return s.App.SaveHouseholdObligation(r.Context(), cid, o)
	})
}

func (s *Server) hhDeleteObligation(w http.ResponseWriter, r *http.Request) {
	s.deleteEntity(w, r, func(cid, id int64) error { return s.App.DeleteHouseholdObligation(r.Context(), cid, id) })
}

// hhPay: POST …/household/obligations/{id}/pay {date, accountId, amount, note}
func (s *Server) hhPay(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in app.PayRequest
	if !decode(w, r, &in) {
		return
	}
	out, err := s.App.PayObligation(r.Context(), cid, id, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// ---- bločky ----

// hhReceiptQR: POST …/household/receipt/qr {qr} — návrh pohybu z bločku eKasa.
func (s *Server) hhReceiptQR(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var in struct {
		QR string `json:"qr"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := s.App.HouseholdReceiptQR(r.Context(), cid, in.QR)
	if err != nil {
		failEshop(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// readPhoto prečíta fotku z tela požiadavky (najviac 3 MB).
func readPhoto(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, app.MaxReceiptPhoto+1))
	if err != nil || len(data) > app.MaxReceiptPhoto {
		writeErr(w, http.StatusRequestEntityTooLarge, "Fotka je príliš veľká (najviac 3 MB)", nil)
		return nil, false
	}
	return data, true
}

// hhReceiptPhoto: POST …/household/receipt/photo (telo = fotka) — návrh z fotky cez Claude API.
func (s *Server) hhReceiptPhoto(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	data, ok := readPhoto(w, r)
	if !ok {
		return
	}
	out, err := s.App.HouseholdReceiptPhoto(r.Context(), cid, data)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// hhSavePhoto: PUT …/household/entries/{id}/photo (telo = fotka, Content-Type).
func (s *Server) hhSavePhoto(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	data, ok := readPhoto(w, r)
	if !ok {
		return
	}
	ct := strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0])
	if err := s.App.SaveHouseholdPhoto(r.Context(), cid, id, ct, data); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// hhPhoto: GET …/household/entries/{id}/photo
func (s *Server) hhPhoto(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	p, err := s.App.HouseholdPhoto(r.Context(), cid, id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Pohyb nemá fotku bločku", nil)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", p.ContentType)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	_, _ = w.Write(p.Data)
}
