// Package job: zákazky — jednoduché CRM/ERP. Zákazka má číslo, zákazníka,
// stav (dopyt → ponuka → realizácia → dokončená, alebo stratená), termíny
// a plánovanú cenu. Doklady (ponuky, faktúry, prijaté faktúry, dodacie
// listy…) sa k nej priraďujú prepojením (internal/link).
package job

import (
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
)

// Status je stav zákazky.
type Status string

const (
	Lead   Status = "lead"   // dopyt
	Offer  Status = "offer"  // ponuka odoslaná
	Active Status = "active" // v realizácii
	Done   Status = "done"   // dokončená
	Lost   Status = "lost"   // stratená / zrušená
)

// Statuses v poradí priebehu.
var Statuses = []Status{Lead, Offer, Active, Done, Lost}

// Known: je stav platný?
func (s Status) Known() bool {
	for _, x := range Statuses {
		if x == s {
			return true
		}
	}
	return false
}

// Open: zákazka ešte beží (nie dokončená ani stratená).
func (s Status) Open() bool { return s == Lead || s == Offer || s == Active }

// Job je zákazka.
type Job struct {
	ID        int64        `json:"id"`
	CompanyID int64        `json:"companyId"`
	Number    string       `json:"number"`
	Name      string       `json:"name"`
	ContactID int64        `json:"contactId"`
	Customer  string       `json:"customer"` // názov zákazníka (z adresára)
	Status    Status       `json:"status"`
	Start     invoice.Date `json:"start"`
	Due       invoice.Date `json:"due"`    // termín dokončenia
	Closed    invoice.Date `json:"closed"` // kedy bola dokončená alebo stratená
	Budget    money.Dec    `json:"budget"` // plánovaná cena (bez DPH u platiteľa)
	Note      string       `json:"note"`
	Created   invoice.Date `json:"created"`
}

// FieldError je chyba poľa.
type FieldError struct {
	Field   string
	Message string
}

// Normalize oreže texty a doplní stav.
func (j *Job) Normalize() {
	j.Name, j.Customer, j.Note = strings.TrimSpace(j.Name), strings.TrimSpace(j.Customer), strings.TrimSpace(j.Note)
	if j.Status == "" {
		j.Status = Lead
	}
}

// Validate overí zákazku.
func (j *Job) Validate() []FieldError {
	var out []FieldError
	if j.Name == "" {
		out = append(out, FieldError{"name", "Zadajte názov zákazky"})
	} else if len([]rune(j.Name)) > 200 {
		out = append(out, FieldError{"name", "Názov je pridlhý (najviac 200 znakov)"})
	}
	if !j.Status.Known() {
		out = append(out, FieldError{"status", "Neznámy stav zákazky"})
	}
	if !j.Start.IsZero() && !j.Due.IsZero() && j.Due.Before(j.Start) {
		out = append(out, FieldError{"due", "Termín nemôže byť pred začiatkom"})
	}
	if j.Budget.Sign() < 0 {
		out = append(out, FieldError{"budget", "Plánovaná cena nemôže byť záporná"})
	}
	return out
}

// SetStatus zmení stav; pri dokončení alebo strate zapíše dátum, pri
// opätovnom otvorení ho zmaže.
func (j *Job) SetStatus(s Status, today invoice.Date) {
	if s == j.Status {
		return
	}
	j.Status = s
	if s.Open() {
		j.Closed = invoice.Date{}
	} else if j.Closed.IsZero() {
		j.Closed = today
	}
}

// Late: otvorená zákazka po termíne.
func (j *Job) Late(today invoice.Date) bool {
	return j.Status.Open() && !j.Due.IsZero() && j.Due.Before(today)
}
