package household

import (
	"sort"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
)

// Záväzky domácnosti: pôžičky a úvery, dlhy, exekúcie, pravidelné platby
// (nájom, energie, poistky) a peniaze požičané iným. Splátka je pohyb
// domácnosti s odkazom na záväzok (Entry.ObligationID); zaplatené sa
// rozpočíta na splátky v poradí splatnosti.

// ObligationKind je druh záväzku.
type ObligationKind string

const (
	Loan      ObligationKind = "loan"      // pôžička, úver, hypotéka, leasing
	Debt      ObligationKind = "debt"      // dlh voči osobe alebo firme
	Execution ObligationKind = "execution" // exekúcia
	Regular   ObligationKind = "regular"   // pravidelná platba bez dlhu (nájom, energie, poistka)
	Lent      ObligationKind = "lent"      // požičané niekomu — splátky sú príjem
	Fine      ObligationKind = "fine"      // pokuta, poplatok, jednorazová platba (škola, krúžok)
)

// Frequency je opakovanie splátok.
type Frequency string

const (
	Once      Frequency = "once"
	Monthly   Frequency = "monthly"
	Quarterly Frequency = "quarterly"
	HalfYear  Frequency = "halfyear"
	Yearly    Frequency = "yearly"
)

// months je počet mesiacov medzi splátkami.
func (f Frequency) months() int {
	switch f {
	case Monthly:
		return 1
	case Quarterly:
		return 3
	case HalfYear:
		return 6
	case Yearly:
		return 12
	}
	return 0
}

// Obligation je záväzok domácnosti.
type Obligation struct {
	ID        int64          `json:"id"`
	CompanyID int64          `json:"companyId"`
	Kind      ObligationKind `json:"kind"`
	Name      string         `json:"name"`     // napr. Hypotéka VÚB, Exekúcia – JUDr. Novák
	Creditor  string         `json:"creditor"` // komu sa platí (pri požičaných: kto vracia)
	// Principal je celková dlžná suma (pri exekúcii vrátane trov); nula =
	// bez konečnej sumy (pravidelná platba).
	Principal money.Dec `json:"principal"`
	// PaidBefore je suma splatená pred začiatkom evidencie vo Fiškuse.
	PaidBefore  money.Dec    `json:"paidBefore"`
	Installment money.Dec    `json:"installment"` // splátka
	Frequency   Frequency    `json:"frequency"`
	FirstDue    invoice.Date `json:"firstDue"` // splatnosť prvej splátky
	// Count je počet splátok (0 = kým sa nesplatí celá suma, pri pravidelnej
	// platbe bez konca).
	Count     int    `json:"count"`
	Category  string `json:"category"`  // kategória pohybu pri zaplatení
	AccountID int64  `json:"accountId"` // predvolený účet
	Reference string `json:"reference"` // číslo zmluvy, spisová značka
	VS        string `json:"vs"`
	IBAN      string `json:"iban"`
	Address   string `json:"address"` // adresa príjemcu (poštová poukážka, šek)
	// Months sú mesiace (1 – 12), v ktorých sa pri mesačnom opakovaní platí
	// (napr. škola september – jún); prázdne = každý mesiac.
	Months []int `json:"months,omitempty"`
	// RemindDays je koľko dní vopred pripomenúť splátku (0 = podľa
	// nastavenia upozornení, v kalendári v telefóne 3 a 1 deň vopred).
	RemindDays int    `json:"remindDays"`
	Closed     bool   `json:"closed"` // ukončený (splatený, zrušený)
	Note       string `json:"note"`
}

// Income: splátky záväzku sú príjem (požičané niekomu).
func (o Obligation) Income() bool { return o.Kind == Lent }

// normMonths zoradí mesiace bez opakovania; všetkých 12 = prázdne.
func normMonths(ms []int) []int {
	seen := map[int]bool{}
	out := []int{}
	for _, m := range ms {
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	sort.Ints(out)
	if len(out) == 0 || len(out) == 12 {
		return nil
	}
	return out
}

// paysIn: platí sa v mesiaci (1 – 12)?
func (o Obligation) paysIn(month int) bool {
	if len(o.Months) == 0 || o.Frequency != Monthly {
		return true
	}
	for _, m := range o.Months {
		if m == month {
			return true
		}
	}
	return false
}

// Normalize upraví záväzok pred uložením.
func (o *Obligation) Normalize() {
	o.Name = strings.TrimSpace(o.Name)
	o.Creditor = strings.TrimSpace(o.Creditor)
	o.Reference = strings.TrimSpace(o.Reference)
	o.VS = strings.TrimSpace(o.VS)
	o.IBAN = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(o.IBAN), " ", ""))
	o.Note = strings.TrimSpace(o.Note)
	o.Address = strings.TrimSpace(o.Address)
	o.Months = normMonths(o.Months)
	if o.Frequency != "" && o.Frequency != Monthly {
		o.Months = nil
	}
	o.Principal = o.Principal.RoundCents()
	o.PaidBefore = o.PaidBefore.RoundCents()
	o.Installment = o.Installment.RoundCents()
	if o.Frequency == "" {
		o.Frequency = Monthly
	}
	if o.Frequency == Once {
		o.Count = 1
	}
}

// Validate skontroluje záväzok.
func (o Obligation) Validate() error {
	var fe []FieldError
	add := func(f, m string) { fe = append(fe, FieldError{Field: f, Message: m}) }
	switch o.Kind {
	case Loan, Debt, Execution, Regular, Lent, Fine:
	default:
		add("kind", "Neznámy druh záväzku")
	}
	if o.Name == "" {
		add("name", "Zadajte názov")
	}
	switch o.Frequency {
	case Once, Monthly, Quarterly, HalfYear, Yearly:
	default:
		add("frequency", "Neznáme opakovanie")
	}
	if o.FirstDue.IsZero() {
		add("firstDue", "Zadajte splatnosť prvej splátky")
	}
	if o.Installment.Sign() <= 0 {
		add("installment", "Zadajte výšku splátky")
	}
	if o.Principal.Sign() < 0 {
		add("principal", "Suma nemôže byť záporná")
	}
	if o.PaidBefore.Sign() < 0 || (o.Principal.Sign() > 0 && o.PaidBefore.Cmp(o.Principal) > 0) {
		add("paidBefore", "Splatené skôr nemôže byť záporné ani viac ako celá suma")
	}
	if o.Count < 0 || o.Count > 1200 {
		add("count", "Počet splátok je 0 až 1200")
	}
	for _, m := range o.Months {
		if m < 1 || m > 12 {
			add("months", "Mesiace sú 1 až 12")
			break
		}
	}
	if o.RemindDays < 0 || o.RemindDays > 60 {
		add("remindDays", "Pripomenúť sa dá 0 až 60 dní vopred")
	}
	if o.Kind != Regular && o.Principal.IsZero() && o.Count == 0 {
		add("principal", "Zadajte dlžnú sumu alebo počet splátok")
	}
	return verr(fe)
}

// DueStatus je stav splátky.
type DueStatus string

const (
	DuePaid     DueStatus = "paid"     // zaplatená
	DuePartial  DueStatus = "partial"  // zaplatená čiastočne
	DueOverdue  DueStatus = "overdue"  // po splatnosti
	DueUpcoming DueStatus = "upcoming" // ešte nesplatná
)

// Due je splátka v splátkovom kalendári.
type Due struct {
	N      int          `json:"n"` // poradie splátky (od 1)
	Date   invoice.Date `json:"date"`
	Amount money.Dec    `json:"amount"`
	Paid   money.Dec    `json:"paid"` // zaplatené na túto splátku
	Status DueStatus    `json:"status"`
}

// Paid vráti zaplatené na záväzok: splatené pred evidenciou + pohyby so
// záväzkom (výdavky; pri požičaných príjmy).
func Paid(o Obligation, entries []Entry) money.Dec {
	sum := o.PaidBefore
	for _, e := range entries {
		if e.ObligationID != o.ID || e.Transfer() {
			continue
		}
		if o.Income() {
			sum = sum.Add(e.Amount)
		} else {
			sum = sum.Sub(e.Amount)
		}
	}
	return sum
}

// Schedule zostaví splátkový kalendár od prvej splátky v evidencii: splátky
// do konca dlhu (bez sumy splatenej pred evidenciou) alebo počtu splátok,
// pri pravidelnej platbe bez konca do dátumu until. Zaplatená suma sa
// priradí splátkam v poradí; posledná splátka dlhu je zvyšok.
func Schedule(o Obligation, paid money.Dec, today, until invoice.Date) []Due {
	out := []Due{}
	if o.Installment.Sign() <= 0 || o.FirstDue.IsZero() {
		return out
	}
	step := o.Frequency.months()
	left := paid.Sub(o.PaidBefore)
	principal := o.Principal.Sub(o.PaidBefore)
	total := money.Zero // naplánované spolu
	k := -1             // poradie obdobia od prvej splátky (aj vynechané mesiace)
	for n := 1; n <= 1200; n++ {
		if o.Count > 0 && n > o.Count {
			break
		}
		if step == 0 && n > 1 {
			break
		}
		amt := o.Installment
		if o.Principal.Sign() > 0 {
			rest := principal.Sub(total)
			if rest.Sign() <= 0 {
				break
			}
			if amt.Cmp(rest) > 0 || (o.Count > 0 && n == o.Count) {
				amt = rest // posledná splátka doplatí zvyšok
			}
		}
		d, ok := invoice.Date{}, false
		for tries := 0; tries < 12 && !ok; tries++ {
			k++
			d = o.FirstDue.AddMonths(k * step)
			ok = o.paysIn(d.Month())
		}
		if !ok {
			break
		}
		unbounded := o.Count == 0 && o.Principal.IsZero()
		if unbounded && d.After(until) && left.Sign() <= 0 {
			break
		}
		due := Due{N: n, Date: d, Amount: amt}
		switch {
		case left.Cmp(amt) >= 0:
			due.Paid, due.Status = amt, DuePaid
			left = left.Sub(amt)
		case left.Sign() > 0:
			due.Paid, due.Status = left, DuePartial
			left = money.Zero
		default:
			due.Status = DueUpcoming
		}
		if due.Status != DuePaid && d.Before(today) {
			due.Status = DueOverdue
			// pravidelná platba bez konca: staré nezaplatené obdobia (viac ako
			// 2 mesiace) sa už neupomínajú — evidencia začala neskôr
			if unbounded && d.Before(today.AddMonths(-2)) {
				total = total.Add(amt)
				continue
			}
		}
		out = append(out, due)
		total = total.Add(amt)
	}
	return out
}

// ObligationState je stav záväzku k dnešku.
type ObligationState struct {
	Paid          money.Dec    `json:"paid"`
	Remaining     money.Dec    `json:"remaining"` // zostáva splatiť (nula pri pravidelnej platbe)
	NextDue       invoice.Date `json:"nextDue,omitzero"`
	NextAmount    money.Dec    `json:"nextAmount"` // čo treba zaplatiť pri najbližšej splátke
	Overdue       int          `json:"overdue"`    // počet splátok po splatnosti
	OverdueAmount money.Dec    `json:"overdueAmount"`
	PaidOff       bool         `json:"paidOff"`
	Left          int          `json:"left"` // zostávajúci počet splátok (0 = bez konca)
}

// State vypočíta stav záväzku z pohybov.
func State(o Obligation, entries []Entry, today invoice.Date) ObligationState {
	paid := Paid(o, entries)
	st := ObligationState{Paid: paid}
	if o.Principal.Sign() > 0 {
		st.Remaining = o.Principal.Sub(paid)
		if st.Remaining.Sign() < 0 {
			st.Remaining = money.Zero
		}
	}
	sched := Schedule(o, paid, today, today.AddMonths(13))
	open := 0
	for _, d := range sched {
		if d.Status == DuePaid {
			continue
		}
		open++
		rest := d.Amount.Sub(d.Paid)
		if d.Status == DueOverdue {
			st.Overdue++
			st.OverdueAmount = st.OverdueAmount.Add(rest)
		}
		if st.NextDue.IsZero() {
			st.NextDue, st.NextAmount = d.Date, rest
		}
	}
	bounded := o.Principal.Sign() > 0 || o.Count > 0
	st.PaidOff = bounded && open == 0
	if bounded {
		st.Left = open
	}
	return st
}
