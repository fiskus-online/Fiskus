// Package job: zákazky — jednoduché CRM/ERP. Zákazka má číslo, zákazníka,
// stav (dopyt → ponuka → realizácia → dokončená, alebo stratená), termíny
// a plánovanú cenu. Doklady (ponuky, faktúry, prijaté faktúry, dodacie
// listy…) sa k nej priraďujú prepojením (internal/link).
package job

import (
	"fmt"
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
	// Rate je predvolená hodinová sadzba pre výkaz práce (bez DPH u platiteľa).
	Rate money.Dec `json:"rate"`
	// Work je výkaz práce, Material materiál spotrebovaný na zákazke.
	Work     []WorkEntry     `json:"work"`
	Material []MaterialEntry `json:"material"`
	// Kind je druh zákazky; podľa neho je vyplnené Vehicle, Site alebo Device.
	Kind    Kind     `json:"kind"`
	Vehicle *Vehicle `json:"vehicle,omitempty"`
	Site    *Site    `json:"site,omitempty"`
	Device  *Device  `json:"device,omitempty"`
}

// WorkEntry je záznam vo výkaze práce.
type WorkEntry struct {
	ID          int64        `json:"id"` // poradie v rámci zákazky
	Date        invoice.Date `json:"date"`
	Person      string       `json:"person"`
	Description string       `json:"description"`
	Hours       money.Dec    `json:"hours"`
	Rate        money.Dec    `json:"rate"`     // predajná sadzba za hodinu
	CostRate    money.Dec    `json:"costRate"` // náklad za hodinu (mzda, subdodávka); 0 = vlastná práca
	// Total je predajná hodnota (dopĺňa aplikácia na zobrazenie).
	Total money.Dec `json:"total"`
	Billing
}

// MaterialEntry je materiál spotrebovaný na zákazke. Položka vedená na
// sklade (ItemID) sa vydá zo skladu výdajkou zákazky a náklad je z ocenenia
// skladu; inak je náklad množstvo × UnitCost.
type MaterialEntry struct {
	ID        int64        `json:"id"`
	Date      invoice.Date `json:"date"`
	ItemID    int64        `json:"itemId,omitempty"`
	Name      string       `json:"name"`
	Unit      string       `json:"unit"`
	Quantity  money.Dec    `json:"quantity"`
	UnitPrice money.Dec    `json:"unitPrice"` // predajná cena za jednotku
	UnitCost  money.Dec    `json:"unitCost"`  // nákupná cena (iba mimo skladu)
	// Cost je náklad riadku (dopĺňa aplikácia, zo skladu alebo UnitCost).
	Cost  money.Dec `json:"cost"`
	Total money.Dec `json:"total"`           // predajná hodnota (dopĺňa aplikácia)
	Stock bool      `json:"stock,omitempty"` // vydané zo skladu (dopĺňa aplikácia)
	Billing
}

// Billing: na ktorej faktúre je záznam vyfakturovaný (0 = ešte nie).
type Billing struct {
	InvoiceID     int64  `json:"invoiceId,omitempty"`
	InvoiceNumber string `json:"invoiceNumber,omitempty"`
}

// Billed: záznam je už na faktúre.
func (b Billing) Billed() bool { return b.InvoiceID != 0 }

// Amount je predajná hodnota záznamu práce.
func (w WorkEntry) Amount() money.Dec { return w.Hours.Mul(w.Rate).RoundCents() }

// Cost je náklad záznamu práce.
func (w WorkEntry) Cost() money.Dec { return w.Hours.Mul(w.CostRate).RoundCents() }

// Amount je predajná hodnota materiálu.
func (m MaterialEntry) Amount() money.Dec { return m.Quantity.Mul(m.UnitPrice).RoundCents() }

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
	j.normalizeKind()
	for i := range j.Work {
		w := &j.Work[i]
		w.Person, w.Description = strings.TrimSpace(w.Person), strings.TrimSpace(w.Description)
	}
	for i := range j.Material {
		m := &j.Material[i]
		m.Name, m.Unit = strings.TrimSpace(m.Name), strings.TrimSpace(m.Unit)
		if m.Unit == "" {
			m.Unit = "ks"
		}
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
	out = append(out, j.validateKind()...)
	if j.Budget.Sign() < 0 {
		out = append(out, FieldError{"budget", "Plánovaná cena nemôže byť záporná"})
	}
	if j.Rate.Sign() < 0 {
		out = append(out, FieldError{"rate", "Sadzba nemôže byť záporná"})
	}
	for i, w := range j.Work {
		f := func(s string) string { return fmt.Sprintf("work[%d].%s", i, s) }
		if w.Date.IsZero() {
			out = append(out, FieldError{f("date"), "Zadajte dátum"})
		}
		if w.Description == "" {
			out = append(out, FieldError{f("description"), "Popíšte prácu"})
		}
		if w.Hours.Sign() <= 0 {
			out = append(out, FieldError{f("hours"), "Zadajte počet hodín"})
		} else if w.Hours.Cmp(money.MustParse("24")) > 0 {
			out = append(out, FieldError{f("hours"), "Najviac 24 hodín v jednom zázname"})
		}
		if w.Rate.Sign() < 0 || w.CostRate.Sign() < 0 {
			out = append(out, FieldError{f("rate"), "Sadzba nemôže byť záporná"})
		}
	}
	for i, m := range j.Material {
		f := func(s string) string { return fmt.Sprintf("material[%d].%s", i, s) }
		if m.Date.IsZero() {
			out = append(out, FieldError{f("date"), "Zadajte dátum"})
		}
		if m.Name == "" && m.ItemID == 0 {
			out = append(out, FieldError{f("name"), "Zadajte názov materiálu"})
		}
		if m.Quantity.Sign() <= 0 {
			out = append(out, FieldError{f("quantity"), "Zadajte množstvo"})
		}
		if m.UnitPrice.Sign() < 0 || m.UnitCost.Sign() < 0 {
			out = append(out, FieldError{f("unitPrice"), "Cena nemôže byť záporná"})
		}
	}
	return out
}

// KeepBilled prevezme z pôvodnej zákazky vyfakturované záznamy (tie sa
// nemenia ani nemažú, kým je faktúra platná) a novým záznamom pridelí ID.
// Klient nemôže záznam označiť za vyfakturovaný. Vráti chybu, ak klient
// vyfakturovaný záznam zmazal alebo zmenil.
func (j *Job) KeepBilled(old Job) []FieldError {
	var out []FieldError
	next := int64(0)
	for _, w := range old.Work {
		next = max(next, w.ID)
	}
	for _, m := range old.Material {
		next = max(next, m.ID)
	}
	oldWork := map[int64]WorkEntry{}
	for _, w := range old.Work {
		oldWork[w.ID] = w
	}
	seen := map[int64]bool{}
	for i := range j.Work {
		w := &j.Work[i]
		o, ok := oldWork[w.ID]
		switch {
		case ok && o.Billed():
			if !sameWork(*w, o) {
				out = append(out, FieldError{fmt.Sprintf("work[%d].description", i), "Práca je vyfakturovaná na " + o.InvoiceNumber + ", nemení sa"})
			}
			*w = o
		case ok && !seen[w.ID]:
			w.Billing = Billing{}
		default:
			next++
			w.ID, w.Billing = next, Billing{}
		}
		seen[w.ID] = true
	}
	for _, o := range old.Work {
		if o.Billed() && !seen[o.ID] {
			out = append(out, FieldError{"work", "Práca z " + o.Date.Display() + " je vyfakturovaná na " + o.InvoiceNumber + ", nedá sa zmazať"})
		}
	}
	oldMat := map[int64]MaterialEntry{}
	for _, m := range old.Material {
		oldMat[m.ID] = m
	}
	seen = map[int64]bool{}
	for i := range j.Material {
		m := &j.Material[i]
		o, ok := oldMat[m.ID]
		switch {
		case ok && o.Billed():
			if !sameMaterial(*m, o) {
				out = append(out, FieldError{fmt.Sprintf("material[%d].name", i), "Materiál je vyfakturovaný na " + o.InvoiceNumber + ", nemení sa"})
			}
			*m = o
		case ok && !seen[m.ID]:
			m.Billing = Billing{}
		default:
			next++
			m.ID, m.Billing = next, Billing{}
		}
		seen[m.ID] = true
	}
	for _, o := range old.Material {
		if o.Billed() && !seen[o.ID] {
			out = append(out, FieldError{"material", o.Name + " je vyfakturovaný na " + o.InvoiceNumber + ", nedá sa zmazať"})
		}
	}
	return out
}

func sameWork(a, b WorkEntry) bool {
	return a.Date == b.Date && a.Person == b.Person && a.Description == b.Description &&
		a.Hours.Cmp(b.Hours) == 0 && a.Rate.Cmp(b.Rate) == 0 && a.CostRate.Cmp(b.CostRate) == 0
}

func sameMaterial(a, b MaterialEntry) bool {
	return a.Date == b.Date && a.ItemID == b.ItemID && (a.ItemID != 0 || a.Name == b.Name) && a.Unit == b.Unit &&
		a.Quantity.Cmp(b.Quantity) == 0 && a.UnitPrice.Cmp(b.UnitPrice) == 0 && a.UnitCost.Cmp(b.UnitCost) == 0
}

// Release zruší označenie vyfakturovania pri záznamoch, ktorých faktúra
// už neplatí (zmazaný koncept, storno). Vráti true, ak sa niečo zmenilo.
func (j *Job) Release(valid func(invoiceID int64) bool) bool {
	changed := false
	for i := range j.Work {
		if b := &j.Work[i].Billing; b.Billed() && !valid(b.InvoiceID) {
			*b, changed = Billing{}, true
		}
	}
	for i := range j.Material {
		if b := &j.Material[i].Billing; b.Billed() && !valid(b.InvoiceID) {
			*b, changed = Billing{}, true
		}
	}
	return changed
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
