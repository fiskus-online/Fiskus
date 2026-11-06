package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/app"
)

// Fiškus v prehliadači (cmd/fiskus-web) beží celý v prehliadači používateľa.
// Funkcie, ktoré potrebujú nainštalovaný program (pripojenie na iné služby,
// odosielanie e-mailov, mobil v sieti, zálohy do priečinka…), zostávajú
// v rozhraní, ale API namiesto nich vráti výzvu na inštaláciu (stav 501
// a pole „install“ s názvom funkcie). Rozhodnutie je na serveri, rozhranie
// iba zobrazí dialóg so stiahnutím.

// installOnly: adresy, ktoré vo Fiškuse v prehliadači nie sú → názov funkcie.
var installOnly = map[string]string{
	// zálohy do priečinka a do cloudu (prehliadač zálohuje stiahnutím súboru)
	"POST /api/backups":                                    "Automatické zálohy do priečinka",
	"PUT /api/backups/settings":                            "Automatické zálohy do priečinka",
	"PUT /api/backups/remote":                              "Zálohy do cloudu",
	"GET /api/backups/remote":                              "Zálohy do cloudu",
	"POST /api/backups/remote/{name}/fetch":                "Zálohy do cloudu",
	"POST /api/backups/{name}/restore":                     "Automatické zálohy do priečinka",
	"POST /api/me/keys":                                    "Pripojenie AI asistenta (MCP)",
	"PUT /api/mobile":                                      "Mobil v sieti",
	"PUT /api/mobile/address":                              "Mobil v sieti",
	"POST /api/mobile/pairing":                             "Mobil v sieti",
	"GET /api/registry/{ico}":                              "Vyhľadanie firmy podľa IČO",
	"GET /api/vies/{vatid}":                                "Overenie IČ DPH vo VIES",
	"POST /api/legal/ecb":                                  "Sťahovanie sadzby ECB",
	"POST /api/rates/update":                               "Sťahovanie kurzov ECB",
	"PUT /api/ai/settings":                                 "Čítanie dokladov cez AI",
	"POST /api/companies/{cid}/receipts":                   "Čítanie dokladov cez AI",
	"POST /api/companies/{cid}/purchases/extract":          "Čítanie dokladov cez AI",
	"POST /api/companies/{cid}/household/receipt/photo":    "Čítanie bločkov z fotky",
	"POST /api/companies/{cid}/household/receipt/qr":       "Načítanie bločku z eKasy",
	"PUT /api/companies/{cid}/mail-settings":               "Odosielanie e-mailov",
	"POST /api/companies/{cid}/mail-settings/test":         "Odosielanie e-mailov",
	"POST /api/companies/{cid}/invoices/{id}/email":        "Odosielanie e-mailov",
	"POST /api/companies/{cid}/quotes/{id}/email":          "Odosielanie e-mailov",
	"POST /api/companies/{cid}/reminders/{id}/email":       "Odosielanie e-mailov",
	"POST /api/companies/{cid}/quotes/{id}/share":          "Zdieľanie ponuky odkazom",
	"PUT /api/companies/{cid}/einvoice-settings":           "E-faktúry cez poštára",
	"POST /api/companies/{cid}/einvoice-settings/check":    "E-faktúry cez poštára",
	"POST /api/companies/{cid}/invoices/{id}/einvoice":     "E-faktúry cez poštára",
	"POST /api/companies/{cid}/einvoices/{id}/track":       "E-faktúry cez poštára",
	"POST /api/companies/{cid}/einvoice-inbox/{msg}/take":  "E-faktúry cez poštára",
	"PUT /api/companies/{cid}/eshops":                      "Prepojenie s e-shopom",
	"POST /api/companies/{cid}/eshops/{id}/test":           "Prepojenie s e-shopom",
	"POST /api/companies/{cid}/eshops/{id}/orders":         "Prepojenie s e-shopom",
	"POST /api/companies/{cid}/eshops/{id}/stock":          "Prepojenie s e-shopom",
	"POST /api/companies/{cid}/eshops/{id}/catalog/import": "Prepojenie s e-shopom",
	"POST /api/companies/{cid}/eshops/{id}/catalog/push":   "Prepojenie s e-shopom",
	"POST /api/companies/{cid}/calendar/sync/connect":      "Prepojenie s Google a Outlook kalendárom",
	"POST /api/companies/{cid}/calendar/sync/run":          "Prepojenie s Google a Outlook kalendárom",
	"PUT /api/calendar-sync/config":                        "Prepojenie s Google a Outlook kalendárom",
	"POST /api/me/calendar-feed":                           "Odber kalendára v telefóne",
}

// installBody je odpoveď pre funkciu, ktorá je iba v nainštalovanom Fiškuse.
type installBody struct {
	Error   string `json:"error"`
	Install string `json:"install"`
}

// guard nahradí funkciu, ktorá v prehliadači nie je, výzvou na inštaláciu.
func (s *Server) guard(pattern string, h http.HandlerFunc) http.HandlerFunc {
	feature, ok := installOnly[pattern]
	if !ok || !s.App.Caps.Browser {
		return h
	}
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotImplemented, installBody{
			Error:   feature + " je iba v nainštalovanom Fiškuse.",
			Install: feature,
		})
	}
}

// routeBrowser pridá zálohu a obnovu Fiškusa v prehliadači.
func (s *Server) routeBrowser(open func(string, http.HandlerFunc)) {
	if !s.App.Caps.Browser {
		return
	}
	open("GET /api/browser/backup", s.browserBackupStatus)
	open("POST /api/browser/backup", s.browserBackup)
	open("POST /api/browser/restore", s.browserRestore)
}

func (s *Server) browserBackupStatus(w http.ResponseWriter, r *http.Request) {
	st, err := s.App.BrowserBackupStatus(r.Context(), time.Now())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// browserBackup pošle šifrovanú zálohu (.fbk) na stiahnutie.
func (s *Server) browserBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "Neplatná požiadavka", nil)
		return
	}
	now := time.Now()
	data, err := s.App.BrowserBackup(r.Context(), req.Password, now)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+app.BrowserBackupName(now)+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

// maxRestore je najväčší súbor zálohy na obnovu.
const maxRestore = 512 << 20

// browserRestore prijme súbor zálohy (formulár: file, password).
func (s *Server) browserRestore(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRestore)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "Súbor zálohy je príliš veľký alebo chýba", nil)
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Vyberte súbor zálohy (.fbk)", nil)
		return
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Súbor zálohy sa nedá prečítať", nil)
		return
	}
	if err := s.App.BrowserRestore(r.Context(), data, r.FormValue("password")); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
