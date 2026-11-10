package app

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/job"
	"github.com/fiskus-online/fiskus/backend/internal/link"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/quote"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// Zákazky (CRM): evidencia a ekonomika z prepojených dokladov. Nie je to
// analytická zákazka v účtovom rozvrhu (chart.Job, kód pri zaúčtovaní).

// jobSeries je číselný rad zákaziek (Z2026001).
const jobSeries = "ZK"

// JobSummary je ekonomika zákazky v eurách. U platiteľa DPH sú ponuky,
// fakturácia a náklady bez DPH (Net), úhrady vždy s DPH.
type JobSummary struct {
	Net      bool      `json:"net"`
	Offered  money.Dec `json:"offered"`  // ponuky (vystavené, prijaté, vyfakturované)
	Invoiced money.Dec `json:"invoiced"` // faktúry a dobropisy (bez zálohových)
	Gross    money.Dec `json:"gross"`    // fakturované s DPH
	Paid     money.Dec `json:"paid"`     // uhradené
	Unpaid   money.Dec `json:"unpaid"`   // na úhradu
	Costs    money.Dec `json:"costs"`    // prijaté faktúry, práca a materiál
	Profit   money.Dec `json:"profit"`   // fakturované − náklady
	// Hours je odpracovaný čas, Unbilled predajná hodnota nevyfakturovanej
	// práce a materiálu; WorkCost a MaterialCost sú časti nákladov.
	Hours        money.Dec `json:"hours"`
	Unbilled     money.Dec `json:"unbilled"`
	WorkCost     money.Dec `json:"workCost"`
	MaterialCost money.Dec `json:"materialCost"`
	// Margin je zisk v % z fakturovaného (nil, kým nie je fakturácia).
	Margin *int `json:"margin,omitempty"`
	Docs   int  `json:"docs"`
}

// JobView je zákazka s ekonomikou (a v detaile s dokladmi).
type JobView struct {
	job.Job
	Late    bool        `json:"late"`
	Summary JobSummary  `json:"summary"`
	Docs    []LinkedDoc `json:"docs,omitempty"`
}

func jobErr(errs []job.FieldError) error {
	if len(errs) == 0 {
		return nil
	}
	fe := make([]invoice.FieldError, len(errs))
	for i, e := range errs {
		fe[i] = invoice.FieldError{Field: e.Field, Message: e.Message}
	}
	return &invoice.ValidationError{Fields: fe}
}

// jobSummary spočíta ekonomiku z prepojených dokladov, výkazu a materiálu
// (a doplní záznamom predajnú hodnotu).
func (a *App) jobSummary(ctx context.Context, companyID int64, j *job.Job, vatPayer bool) (JobSummary, error) {
	s := JobSummary{Net: vatPayer}
	amount := func(sum, base money.Dec) money.Dec {
		if vatPayer {
			return base
		}
		return sum
	}
	self := link.Ref{Kind: link.Job, ID: j.ID}
	links, err := a.Store.ListDocLinks(ctx, companyID, self)
	if err != nil {
		return s, err
	}
	for _, l := range links {
		o := l.Other(self)
		switch o.Kind {
		case link.Quote:
			q, err := a.Store.GetQuote(ctx, companyID, o.ID)
			if err != nil {
				continue
			}
			s.Docs++
			if q.Status != quote.StatusDraft && q.Status != quote.StatusRejected && (q.Currency == "" || q.Currency == "EUR") {
				s.Offered = s.Offered.Add(amount(q.Summary.Total, q.Summary.Base))
			}
		case link.Invoice:
			inv, err := a.Store.GetInvoice(ctx, companyID, o.ID)
			if err != nil {
				continue
			}
			s.Docs++
			if inv.Status == invoice.StatusDraft || (inv.Kind != invoice.KindInvoice && inv.Kind != invoice.KindCreditNote) {
				continue // koncept, zálohová faktúra a doklad k platbe sa do fakturácie neratajú
			}
			ts, ok := inv.TaxSummary()
			if !ok {
				continue
			}
			s.Invoiced = s.Invoiced.Add(amount(ts.Total, ts.Base))
			s.Gross = s.Gross.Add(ts.Total)
			s.Paid = s.Paid.Add(paidEUR(&inv, ts.Total))
		case link.Purchase:
			p, err := a.Store.GetPurchase(ctx, companyID, o.ID)
			if err != nil {
				continue
			}
			s.Docs++
			t, _ := p.Tax()
			s.Costs = s.Costs.Add(amount(t.Summary.Total, t.Summary.Base))
		default:
			if _, err := a.docInfo(ctx, companyID, o); err == nil {
				s.Docs++
			}
		}
	}
	for i := range j.Work {
		w := &j.Work[i]
		w.Total = w.Amount()
		s.Hours = s.Hours.Add(w.Hours)
		s.WorkCost = s.WorkCost.Add(w.Cost())
		if !w.Billed() {
			s.Unbilled = s.Unbilled.Add(w.Amount())
		}
	}
	for i := range j.Material {
		m := &j.Material[i]
		m.Total = m.Amount()
		s.MaterialCost = s.MaterialCost.Add(m.Cost)
		if !m.Billed() {
			s.Unbilled = s.Unbilled.Add(m.Amount())
		}
	}
	s.Costs = s.Costs.Add(s.WorkCost).Add(s.MaterialCost)
	s.Unpaid = s.Gross.Sub(s.Paid)
	s.Profit = s.Invoiced.Sub(s.Costs)
	if s.Invoiced.Sign() > 0 {
		r := new(big.Rat).Quo(s.Profit.Rat(), s.Invoiced.Rat())
		f, _ := r.Mul(r, big.NewRat(100, 1)).Float64()
		m := int(f + 0.5)
		if f < 0 {
			m = int(f - 0.5)
		}
		s.Margin = &m
	}
	return s, nil
}

// paidEUR: uhradená časť faktúry v eurách (pri cudzej mene pomerne).
func paidEUR(inv *invoice.Invoice, totalEUR money.Dec) money.Dec {
	paid, total := inv.Paid(), inv.Summary.Total
	if !inv.Foreign() {
		return paid
	}
	if total.IsZero() {
		return money.Zero
	}
	r := new(big.Rat).Quo(paid.Rat(), total.Rat())
	v, err := money.FromRat(r.Mul(r, totalEUR.Rat()))
	if err != nil {
		return money.Zero
	}
	return v.RoundCents()
}

func (a *App) jobView(ctx context.Context, companyID int64, j job.Job, vatPayer bool) (JobView, error) {
	if a.releaseJob(ctx, companyID, &j) {
		if err := a.Store.SaveJob(ctx, companyID, &j); err != nil {
			return JobView{}, err
		}
	}
	a.jobMaterialCosts(ctx, companyID, &j)
	s, err := a.jobSummary(ctx, companyID, &j, vatPayer)
	return JobView{Job: j, Late: j.Late(a.Today()), Summary: s}, err
}

// CrmJobs vráti zákazky s ekonomikou.
func (a *App) CrmJobs(ctx context.Context, companyID int64) ([]JobView, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return nil, err
	}
	c, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return nil, err
	}
	list, err := a.Store.ListJobs(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := make([]JobView, 0, len(list))
	for _, j := range list {
		v, err := a.jobView(ctx, companyID, j, c.VATPayer)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// CrmJob vráti zákazku s ekonomikou a dokladmi.
func (a *App) CrmJob(ctx context.Context, companyID, id int64) (JobView, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return JobView{}, err
	}
	c, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return JobView{}, err
	}
	j, err := a.Store.GetJob(ctx, companyID, id)
	if err != nil {
		return JobView{}, err
	}
	v, err := a.jobView(ctx, companyID, j, c.VATPayer)
	if err != nil {
		return v, err
	}
	v.Docs, err = a.DocLinks(ctx, companyID, link.Ref{Kind: link.Job, ID: id})
	return v, err
}

// SaveCrmJob založí (ID 0) alebo upraví zákazku. Nová dostane číslo z radu
// zákaziek (Z2026001); zákazník sa doplní z adresára.
func (a *App) SaveCrmJob(ctx context.Context, companyID int64, j job.Job) (JobView, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return JobView{}, err
	}
	j.Normalize()
	today := a.Today()
	var before *job.Job
	if j.ID != 0 {
		old, err := a.Store.GetJob(ctx, companyID, j.ID)
		if err != nil {
			return JobView{}, err
		}
		before = &old
		a.releaseJob(ctx, companyID, &old)
		if err := jobErr(j.KeepBilled(old)); err != nil {
			return JobView{}, err
		}
		status := j.Status
		j.Number, j.Created, j.Closed, j.Status = old.Number, old.Created, old.Closed, old.Status
		j.SetStatus(status, today)
	} else {
		j.KeepBilled(job.Job{}) // pridelí ID záznamom
		j.Created, j.Closed = today, invoice.Date{}
		if !j.Status.Open() {
			j.Closed = today
		}
	}
	if j.ContactID != 0 {
		ct, err := a.Store.GetContact(ctx, companyID, j.ContactID)
		if errors.Is(err, store.ErrNotFound) {
			return JobView{}, jobErr([]job.FieldError{{Field: "contactId", Message: "Kontakt sa nenašiel"}})
		} else if err != nil {
			return JobView{}, err
		}
		j.Customer = ct.Name
	}
	if err := jobErr(j.Validate()); err != nil {
		return JobView{}, err
	}
	if err := a.prepareJobMaterial(ctx, companyID, &j); err != nil {
		return JobView{}, err
	}
	if before == nil {
		seq, err := a.Store.NextSequence(ctx, companyID, jobSeries, today.Year())
		if err != nil {
			return JobView{}, err
		}
		j.Number = fmt.Sprintf("Z%04d%03d", today.Year(), seq)
	}
	if err := a.Store.SaveJob(ctx, companyID, &j); err != nil {
		return JobView{}, err
	}
	action, what := "create", "Nová zákazka "
	if before != nil {
		action, what = "update", "Úprava zákazky "
	}
	a.record(ctx, companyID, audit.CatInvoices, action, "job", j.ID, what+j.Number+" "+j.Name, before, j)
	if err := a.syncJobStock(ctx, companyID, j); err != nil {
		return JobView{}, err
	}
	return a.CrmJob(ctx, companyID, j.ID)
}

// DeleteCrmJob zmaže zákazku a jej prepojenia (doklady ostanú).
func (a *App) DeleteCrmJob(ctx context.Context, companyID, id int64) error {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return err
	}
	j, err := a.Store.GetJob(ctx, companyID, id)
	if err != nil {
		return err
	}
	a.releaseJob(ctx, companyID, &j)
	for _, w := range j.Work {
		if w.Billed() {
			return vErr("work", "Zákazka má prácu vyfakturovanú na "+w.InvoiceNumber+", nedá sa zmazať")
		}
	}
	for _, m := range j.Material {
		if m.Billed() {
			return vErr("material", "Zákazka má materiál vyfakturovaný na "+m.InvoiceNumber+", nedá sa zmazať")
		}
	}
	links, err := a.Store.ListDocLinks(ctx, companyID, link.Ref{Kind: link.Job, ID: id})
	if err != nil {
		return err
	}
	for _, l := range links {
		if err := a.Store.DeleteDocLink(ctx, companyID, l.ID); err != nil {
			return err
		}
	}
	j.Material = nil
	if err := a.syncJobStock(ctx, companyID, j); err != nil { // výdajky zákazky sa zrušia
		return err
	}
	if err := a.Store.DeleteJob(ctx, companyID, id); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatInvoices, "delete", "job", id, "Zmazaná zákazka "+j.Number+" "+j.Name, j, nil)
	return nil
}
