package app

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/job"
	"github.com/fiskus-online/fiskus/backend/internal/link"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/quote"
	"github.com/fiskus-online/fiskus/backend/internal/stock"
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
	// GoodsCost je nákupná cena tovaru zo skladu predaného faktúrami zákazky
	// (výdajky faktúr mínus vrátenia z dobropisov).
	GoodsCost money.Dec `json:"goodsCost"`
	// Margin je zisk v % z fakturovaného (nil, kým nie je fakturácia).
	Margin *int `json:"margin,omitempty"`
	Docs   int  `json:"docs"`
}

// JobView je zákazka s ekonomikou (a v detaile s dokladmi).
type JobView struct {
	job.Job
	Late    bool        `json:"late"`
	Subject string      `json:"subject"` // predmet podľa druhu (EČV, zariadenie, miesto)
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
	soldFrom := map[int64]bool{} // faktúry zákazky (tovar zo skladu = náklad)
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
			if inv.Status == invoice.StatusDraft || inv.Status == invoice.StatusCancelled || (inv.Kind != invoice.KindInvoice && inv.Kind != invoice.KindCreditNote) {
				continue // koncept, storno, zálohová faktúra a doklad k platbe sa do fakturácie neratajú
			}
			soldFrom[inv.ID] = true
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
	s.GoodsCost = a.goodsCost(ctx, companyID, soldFrom)
	s.Costs = s.Costs.Add(s.WorkCost).Add(s.MaterialCost).Add(s.GoodsCost)
	s.Unpaid = s.Gross.Sub(s.Paid)
	s.Profit = s.Invoiced.Sub(s.Costs)
	s.Margin = percent(s.Profit, s.Invoiced)
	return s, nil
}

// goodsCost spočíta ocenenie výdajok (mínus vrátenia) vystavených faktúr.
func (a *App) goodsCost(ctx context.Context, companyID int64, invoices map[int64]bool) money.Dec {
	total := money.Zero
	if len(invoices) == 0 {
		return total
	}
	docs, err := a.Store.ListStockDocs(ctx, companyID)
	if err != nil {
		return total
	}
	found := false
	for _, d := range docs {
		if d.Auto && d.LinkKind == "invoice" && invoices[d.LinkID] {
			found = true
			break
		}
	}
	if !found {
		return total
	}
	cfg, err := a.loadStock(ctx, companyID)
	if err != nil {
		return total
	}
	_, costed, _ := stock.Valuate(docs, cfg.Method)
	for _, d := range costed {
		if !d.Auto || d.LinkKind != "invoice" || !invoices[d.LinkID] {
			continue
		}
		if d.Kind == stock.Receipt {
			total = total.Sub(d.Total)
		} else {
			total = total.Add(d.Total)
		}
	}
	return total
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
	return JobView{Job: j, Late: j.Late(a.Today()), Subject: j.Subject(), Summary: s}, err
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

// JobStatsRow je súhrn zákaziek jedného zákazníka.
type JobStatsRow struct {
	ContactID int64     `json:"contactId,omitempty"`
	Customer  string    `json:"customer"`
	Jobs      int       `json:"jobs"`
	Open      int       `json:"open"`
	Done      int       `json:"done"`
	Lost      int       `json:"lost"`
	Budget    money.Dec `json:"budget"`
	Invoiced  money.Dec `json:"invoiced"`
	Costs     money.Dec `json:"costs"`
	Profit    money.Dec `json:"profit"`
	Unbilled  money.Dec `json:"unbilled"`
	Margin    *int      `json:"margin,omitempty"`
}

// JobStats je ziskovosť zákaziek podľa zákazníka za rok (0 = všetky).
type JobStats struct {
	Year  int           `json:"year"`
	Net   bool          `json:"net"`
	Rows  []JobStatsRow `json:"rows"`
	Total JobStatsRow   `json:"total"`
	// WinRate: podiel dokončených zo zákaziek, ktoré sa uzavreli (dokončené + stratené).
	WinRate *int  `json:"winRate,omitempty"`
	Years   []int `json:"years"`
}

// jobYear: rok zákazky — uzavretej podľa uzavretia, otvorenej podľa vzniku.
func jobYear(j job.Job) int {
	if !j.Closed.IsZero() {
		return j.Closed.Year()
	}
	if !j.Start.IsZero() {
		return j.Start.Year()
	}
	return j.Created.Year()
}

// percent vráti zaokrúhlené percento a z b (nil pri b = 0).
func percent(a, b money.Dec) *int {
	if b.Sign() == 0 {
		return nil
	}
	r := new(big.Rat).Quo(a.Rat(), b.Rat())
	f, _ := r.Mul(r, big.NewRat(100, 1)).Float64()
	m := int(f + 0.5)
	if f < 0 {
		m = int(f - 0.5)
	}
	return &m
}

// CrmJobStats spočíta ziskovosť zákaziek podľa zákazníka.
func (a *App) CrmJobStats(ctx context.Context, companyID int64, year int) (JobStats, error) {
	views, err := a.CrmJobs(ctx, companyID)
	if err != nil {
		return JobStats{}, err
	}
	st := JobStats{Year: year, Rows: []JobStatsRow{}, Years: []int{}}
	seenYear := map[int]bool{}
	idx := map[string]int{}
	for _, v := range views {
		st.Net = v.Summary.Net
		y := jobYear(v.Job)
		if !seenYear[y] {
			seenYear[y] = true
			st.Years = append(st.Years, y)
		}
		if year != 0 && y != year {
			continue
		}
		key := fmt.Sprintf("c%d", v.ContactID)
		name := v.Customer
		if v.ContactID == 0 {
			key = "n" + strings.ToLower(strings.TrimSpace(v.Customer))
			if name == "" {
				name = "Bez zákazníka"
			}
		}
		i, ok := idx[key]
		if !ok {
			i = len(st.Rows)
			idx[key] = i
			st.Rows = append(st.Rows, JobStatsRow{ContactID: v.ContactID, Customer: name})
		}
		for _, r := range []*JobStatsRow{&st.Rows[i], &st.Total} {
			r.Jobs++
			switch {
			case v.Status.Open():
				r.Open++
			case v.Status == job.Done:
				r.Done++
			case v.Status == job.Lost:
				r.Lost++
			}
			r.Budget = r.Budget.Add(v.Budget)
			r.Invoiced = r.Invoiced.Add(v.Summary.Invoiced)
			r.Costs = r.Costs.Add(v.Summary.Costs)
			r.Profit = r.Profit.Add(v.Summary.Profit)
			r.Unbilled = r.Unbilled.Add(v.Summary.Unbilled)
		}
	}
	for i := range st.Rows {
		st.Rows[i].Margin = percent(st.Rows[i].Profit, st.Rows[i].Invoiced)
	}
	st.Total.Customer = "Spolu"
	st.Total.Margin = percent(st.Total.Profit, st.Total.Invoiced)
	if closed := st.Total.Done + st.Total.Lost; closed > 0 {
		st.WinRate = percent(money.FromInt(int64(st.Total.Done)), money.FromInt(int64(closed)))
	}
	sort.Slice(st.Rows, func(i, k int) bool {
		if c := st.Rows[i].Profit.Cmp(st.Rows[k].Profit); c != 0 {
			return c > 0
		}
		return st.Rows[i].Customer < st.Rows[k].Customer
	})
	sort.Sort(sort.Reverse(sort.IntSlice(st.Years)))
	return st, nil
}
