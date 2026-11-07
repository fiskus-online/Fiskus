package app

import (
	"context"
	"fmt"
	"sort"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/calendar"
	"github.com/fiskus-online/fiskus/backend/internal/household"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// Záväzky domácnosti: pôžičky, dlhy, exekúcie, pravidelné platby a peniaze
// požičané iným, so splátkovým kalendárom. Splátka je pohyb domácnosti
// s odkazom na záväzok.

// ObligationInfo je záväzok so stavom k dnešku.
type ObligationInfo struct {
	household.Obligation
	State household.ObligationState `json:"state"`
}

// ObligationDetail je záväzok so splátkovým kalendárom a zaplatenými splátkami.
type ObligationDetail struct {
	ObligationInfo
	Schedule []household.Due   `json:"schedule"`
	Payments []household.Entry `json:"payments"`
}

// HouseholdObligations vráti záväzky domácnosti so stavom.
func (a *App) HouseholdObligations(ctx context.Context, companyID int64) ([]ObligationInfo, error) {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerRead); err != nil {
		return nil, err
	}
	os, err := a.Store.ListHHObligations(ctx, companyID)
	if err != nil {
		return nil, err
	}
	es, err := a.Store.ListHHEntries(ctx, companyID, 0)
	if err != nil {
		return nil, err
	}
	today := a.Today()
	out := make([]ObligationInfo, 0, len(os))
	for _, o := range os {
		out = append(out, ObligationInfo{Obligation: o, State: household.State(o, es, today)})
	}
	return out, nil
}

// HouseholdObligation vráti záväzok so splátkovým kalendárom (pri
// pravidelnej platbe bez konca rok dopredu).
func (a *App) HouseholdObligation(ctx context.Context, companyID, id int64) (ObligationDetail, error) {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerRead); err != nil {
		return ObligationDetail{}, err
	}
	o, err := a.obligation(ctx, companyID, id)
	if err != nil {
		return ObligationDetail{}, err
	}
	es, err := a.Store.ListHHEntries(ctx, companyID, 0)
	if err != nil {
		return ObligationDetail{}, err
	}
	today := a.Today()
	d := ObligationDetail{ObligationInfo: ObligationInfo{Obligation: o, State: household.State(o, es, today)}, Payments: []household.Entry{}}
	d.Schedule = household.Schedule(o, household.Paid(o, es), today, today.AddMonths(12))
	for _, e := range es {
		if e.ObligationID == id {
			d.Payments = append(d.Payments, e)
		}
	}
	return d, nil
}

func (a *App) obligation(ctx context.Context, companyID, id int64) (household.Obligation, error) {
	os, err := a.Store.ListHHObligations(ctx, companyID)
	if err != nil {
		return household.Obligation{}, err
	}
	for _, o := range os {
		if o.ID == id {
			return o, nil
		}
	}
	return household.Obligation{}, store.ErrNotFound
}

// defaultObligationCategory: kategória splátky podľa druhu záväzku.
func defaultObligationCategory(o household.Obligation, cats []household.Category) string {
	want := map[household.ObligationKind]string{household.Loan: "Splátky a dlhy", household.Debt: "Splátky a dlhy",
		household.Execution: "Splátky a dlhy", household.Regular: "Bývanie", household.Lent: "Iné príjmy", household.Fine: "Iné výdavky"}[o.Kind]
	for _, c := range cats {
		if c.Name == want && c.Income == o.Income() {
			return want
		}
	}
	return fallbackCategory(cats, o.Income())
}

// SaveHouseholdObligation uloží záväzok (ID 0 = nový).
func (a *App) SaveHouseholdObligation(ctx context.Context, companyID int64, o household.Obligation) (household.Obligation, error) {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return o, err
	}
	o.Normalize()
	if err := o.Validate(); err != nil {
		return o, err
	}
	var before *household.Obligation
	if o.ID != 0 {
		old, err := a.obligation(ctx, companyID, o.ID)
		if err != nil {
			return o, err
		}
		before = &old
	}
	s, err := a.householdSettings(ctx, companyID)
	if err != nil {
		return o, err
	}
	if o.Category == "" {
		o.Category = defaultObligationCategory(o, s.Categories)
	}
	found := false
	for _, c := range s.Categories {
		found = found || (c.Name == o.Category && c.Income == o.Income())
	}
	if !found {
		what := "výdavkov"
		if o.Income() {
			what = "príjmov"
		}
		return o, vErr("category", "Vyberte kategóriu "+what)
	}
	if o.AccountID != 0 {
		accs, err := a.Store.ListHHAccounts(ctx, companyID)
		if err != nil {
			return o, err
		}
		ok := false
		for _, x := range accs {
			ok = ok || x.ID == o.AccountID
		}
		if !ok {
			return o, vErr("accountId", "Účet neexistuje")
		}
	}
	if err := a.Store.SaveHHObligation(ctx, companyID, &o); err != nil {
		return o, err
	}
	a.record(ctx, companyID, audit.CatLedger, "save", "household_obligation", o.ID, "Domácnosť: záväzok "+o.Name, before, o)
	return o, nil
}

// DeleteHouseholdObligation zmaže záväzok bez splátok (so splátkami ho treba ukončiť).
func (a *App) DeleteHouseholdObligation(ctx context.Context, companyID, id int64) error {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return err
	}
	o, err := a.obligation(ctx, companyID, id)
	if err != nil {
		return err
	}
	es, err := a.Store.ListHHEntries(ctx, companyID, 0)
	if err != nil {
		return err
	}
	for _, e := range es {
		if e.ObligationID == id {
			return vErr("id", "Záväzok má zaplatené splátky — namiesto zmazania ho označte ako ukončený")
		}
	}
	if err := a.Store.DeleteHHObligation(ctx, companyID, id); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatLedger, "delete", "household_obligation", id, "Domácnosť: zmazaný záväzok "+o.Name, o, nil)
	return nil
}

// PayRequest je zaplatenie splátky.
type PayRequest struct {
	Date      invoice.Date `json:"date"`
	AccountID int64        `json:"accountId"`
	Amount    money.Dec    `json:"amount"` // kladná suma; nula = najbližšia splátka
	Note      string       `json:"note"`
}

// PayObligation zapíše splátku záväzku ako pohyb domácnosti (výdavok, pri
// požičaných príjem) s kategóriou a účtom záväzku.
func (a *App) PayObligation(ctx context.Context, companyID, id int64, p PayRequest) (household.Entry, error) {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return household.Entry{}, err
	}
	o, err := a.obligation(ctx, companyID, id)
	if err != nil {
		return household.Entry{}, err
	}
	amt := p.Amount.RoundCents()
	if amt.IsZero() {
		es, err := a.Store.ListHHEntries(ctx, companyID, 0)
		if err != nil {
			return household.Entry{}, err
		}
		amt = household.State(o, es, a.Today()).NextAmount
	}
	if amt.Sign() <= 0 {
		return household.Entry{}, vErr("amount", "Zadajte sumu splátky")
	}
	if p.Date.IsZero() {
		p.Date = a.Today()
	}
	if p.AccountID == 0 {
		p.AccountID = o.AccountID
	}
	if !o.Income() {
		amt = amt.Neg()
	}
	note := p.Note
	if note == "" {
		note = "Splátka: " + o.Name
	}
	return a.SaveHouseholdEntry(ctx, companyID, household.Entry{Date: p.Date, AccountID: p.AccountID, Amount: amt, Category: o.Category,
		Payee: o.Creditor, Note: note, ObligationID: o.ID})
}

// UpcomingDue je splátka záväzku v prehľade a kalendári.
type UpcomingDue struct {
	ObligationID int64                    `json:"obligationId"`
	Name         string                   `json:"name"`
	Kind         household.ObligationKind `json:"kind"`
	Creditor     string                   `json:"creditor"`
	AccountID    int64                    `json:"accountId"`
	IBAN         string                   `json:"iban,omitempty"`
	VS           string                   `json:"vs,omitempty"`
	Address      string                   `json:"address,omitempty"`
	RemindDays   int                      `json:"remindDays,omitempty"`
	household.Due
	Rest money.Dec `json:"rest"` // zostáva zaplatiť z tejto splátky
}

// obligationDues vráti nezaplatené splátky otvorených záväzkov do dátumu to
// (aj staršie po splatnosti).
func (a *App) obligationDues(ctx context.Context, companyID int64, to invoice.Date) ([]UpcomingDue, error) {
	os, err := a.Store.ListHHObligations(ctx, companyID)
	if err != nil {
		return nil, err
	}
	es, err := a.Store.ListHHEntries(ctx, companyID, 0)
	if err != nil {
		return nil, err
	}
	today := a.Today()
	out := []UpcomingDue{}
	for _, o := range os {
		if o.Closed {
			continue
		}
		for _, d := range household.Schedule(o, household.Paid(o, es), today, to) {
			if d.Status == household.DuePaid || d.Date.After(to) {
				continue
			}
			out = append(out, UpcomingDue{ObligationID: o.ID, Name: o.Name, Kind: o.Kind, Creditor: o.Creditor, AccountID: o.AccountID,
				IBAN: o.IBAN, VS: o.VS, Address: o.Address, RemindDays: o.RemindDays, Due: d, Rest: d.Amount.Sub(d.Paid)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out, nil
}

// householdEvents: splátky záväzkov domácnosti v kalendári.
func (a *App) householdEvents(ctx context.Context, companyID int64, to invoice.Date) ([]calendar.Event, error) {
	dues, err := a.obligationDues(ctx, companyID, to)
	if err != nil {
		return nil, err
	}
	out := make([]calendar.Event, 0, len(dues))
	for _, d := range dues {
		amt := d.Rest
		kind, title := calendar.KindPay, "Splátka: "+d.Name
		switch d.Kind {
		case household.Lent:
			kind, title = calendar.KindReceive, "Má vrátiť: "+d.Name
		case household.Regular, household.Fine:
			title = "Platba: " + d.Name
		}
		out = append(out, calendar.Event{ID: fmt.Sprintf("obl-%d-%d", d.ObligationID, d.N), Date: d.Date, Kind: kind, Title: title,
			Amount: &amt, Curr: "EUR", Link: fmt.Sprintf("/domacnost/zavazky/%d", d.ObligationID), Overdue: d.Status == household.DueOverdue,
			Detail: d.Creditor, RemindDays: d.RemindDays})
	}
	return out, nil
}
