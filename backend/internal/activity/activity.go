// Package activity: CRM aktivity — poznámky, telefonáty, e-maily, stretnutia
// a úlohy s termínom pri kontakte, zákazke alebo cenovej ponuke. Úloha
// s termínom sa ukáže v kalendári, kým nie je splnená.
package activity

import (
	"strings"
	"time"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
)

// Kind je druh aktivity.
type Kind string

const (
	Note    Kind = "note"    // poznámka
	Call    Kind = "call"    // telefonát
	Email   Kind = "email"   // e-mail
	Meeting Kind = "meeting" // stretnutie
	Task    Kind = "task"    // úloha s termínom
)

// Kinds v poradí ponuky.
var Kinds = []Kind{Note, Call, Email, Meeting, Task}

// Known: je druh platný?
func (k Kind) Known() bool {
	for _, x := range Kinds {
		if x == k {
			return true
		}
	}
	return false
}

// Activity je záznam CRM.
type Activity struct {
	ID        int64        `json:"id"`
	CompanyID int64        `json:"companyId"`
	Kind      Kind         `json:"kind"`
	Text      string       `json:"text"`
	Date      invoice.Date `json:"date"` // kedy sa stalo (pri úlohe kedy vznikla)
	Due       invoice.Date `json:"due"`  // termín úlohy
	Done      bool         `json:"done"`
	DoneDate  invoice.Date `json:"doneDate"`
	ContactID int64        `json:"contactId,omitempty"`
	JobID     int64        `json:"jobId,omitempty"`
	QuoteID   int64        `json:"quoteId,omitempty"`
	Author    string       `json:"author"`
	Created   time.Time    `json:"created"`
}

// FieldError je chyba poľa (rovnaký tvar ako pri faktúre).
type FieldError = invoice.FieldError

// Normalize oreže text a doplní druh.
func (a *Activity) Normalize() {
	a.Text = strings.TrimSpace(a.Text)
	if a.Kind == "" {
		a.Kind = Note
	}
	if a.Kind != Task {
		a.Due, a.Done, a.DoneDate = invoice.Date{}, false, invoice.Date{}
	}
	if !a.Done {
		a.DoneDate = invoice.Date{}
	}
}

// Validate overí aktivitu.
func (a *Activity) Validate() []FieldError {
	var out []FieldError
	if !a.Kind.Known() {
		out = append(out, FieldError{Field: "kind", Message: "Neznámy druh záznamu"})
	}
	if a.Text == "" {
		out = append(out, FieldError{Field: "text", Message: "Napíšte, o čo ide"})
	} else if len([]rune(a.Text)) > 4000 {
		out = append(out, FieldError{Field: "text", Message: "Text je pridlhý (najviac 4000 znakov)"})
	}
	if a.Date.IsZero() {
		out = append(out, FieldError{Field: "date", Message: "Zadajte dátum"})
	}
	if a.Kind == Task && a.Due.IsZero() {
		out = append(out, FieldError{Field: "due", Message: "Zadajte termín úlohy"})
	}
	if a.ContactID == 0 && a.JobID == 0 && a.QuoteID == 0 {
		out = append(out, FieldError{Field: "contactId", Message: "Záznam patrí ku kontaktu, zákazke alebo ponuke"})
	}
	return out
}

// Open: nesplnená úloha.
func (a Activity) Open() bool { return a.Kind == Task && !a.Done }

// Late: nesplnená úloha po termíne.
func (a Activity) Late(today invoice.Date) bool { return a.Open() && a.Due.Before(today) }

// SetDone označí úlohu ako splnenú (alebo nesplnenú).
func (a *Activity) SetDone(done bool, today invoice.Date) {
	if a.Kind != Task {
		return
	}
	a.Done = done
	if done {
		a.DoneDate = today
	} else {
		a.DoneDate = invoice.Date{}
	}
}
