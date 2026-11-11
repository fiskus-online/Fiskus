package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/calendar"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/journal"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/purchase"
	"github.com/fiskus-online/fiskus/backend/internal/quote"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// Prehľad firmy na úvodnej stránke: peniaze, pohľadávky a záväzky,
// príjmy a výdavky po mesiacoch, najbližšie termíny a rozpracovaná práca.
// Každá časť je iba pre toho, kto má na ňu právo (inak chýba). Všetky sumy
// sú v eurách; u platiteľa DPH sú príjmy a výdavky bez DPH.

// Dashboard je prehľad firmy za rok.
type Dashboard struct {
	Year  int          `json:"year"`
	Today invoice.Date `json:"today"`
	Net   bool         `json:"net"` // príjmy a výdavky bez DPH (platiteľ)
	// Money sú zostatky peňazí (pokladnice a bankové účty) ku koncu roka.
	Money       *DashMoney `json:"money,omitempty"`
	Receivables *DashOpen  `json:"receivables,omitempty"`
	Payables    *DashOpen  `json:"payables,omitempty"`
	// Months sú príjmy (vydané faktúry) a výdavky (prijaté faktúry) po mesiacoch.
	Months  []DashMonth `json:"months,omitempty"`
	Income  money.Dec   `json:"income"`
	Expense money.Dec   `json:"expense"`
	Result  money.Dec   `json:"result"`
	// Events sú nesplnené termíny na najbližších 14 dní, Overdue zmeškané.
	Events  []calendar.Event `json:"events"`
	Overdue []calendar.Event `json:"overdue"`
	Work    *DashWork        `json:"work,omitempty"`
}

// DashMoney sú peniaze firmy.
type DashMoney struct {
	Total money.Dec       `json:"total"`
	Items []DashMoneyItem `json:"items"`
}

// DashMoneyItem je pokladnica alebo bankový účet.
type DashMoneyItem struct {
	Kind    string    `json:"kind"` // cash | bank
	Label   string    `json:"label"`
	Account string    `json:"account,omitempty"`
	Amount  money.Dec `json:"amount"`
}

// DashOpen sú neuhradené doklady (pohľadávky alebo záväzky).
type DashOpen struct {
	Total        money.Dec          `json:"total"`
	Count        int                `json:"count"`
	Overdue      money.Dec          `json:"overdue"`
	OverdueCount int                `json:"overdueCount"`
	Top          []journal.OpenItem `json:"top"` // najstaršie po splatnosti, inak najbližšie splatné
}

// DashMonth sú príjmy a výdavky mesiaca.
type DashMonth struct {
	Month   int       `json:"month"`
	Income  money.Dec `json:"income"`
	Expense money.Dec `json:"expense"`
}

// DashWork je rozpracovaná práca: zákazky, objednávky, ponuky, sklad.
type DashWork struct {
	Jobs        int       `json:"jobs"`     // bežiace zákazky
	JobsLate    int       `json:"jobsLate"` // po termíne
	Pipeline    money.Dec `json:"pipeline"` // plánovaná cena bežiacich
	Orders      int       `json:"orders"`   // prijaté objednávky na dodanie
	OrdersLate  int       `json:"ordersLate"`
	Quotes      int       `json:"quotes"` // platné vystavené ponuky
	QuotesTotal money.Dec `json:"quotesTotal"`
	StockLow    int       `json:"stockLow"` // položky na objednanie
	Drafts      int       `json:"drafts"`   // koncepty faktúr
}

const dashTop = 5

// Dashboard zostaví prehľad firmy za rok (0 = tento rok).
func (a *App) Dashboard(ctx context.Context, companyID int64, year int) (Dashboard, error) {
	if err := a.Require(ctx, companyID, ""); err != nil {
		return Dashboard{}, err
	}
	today := a.Today()
	if year == 0 {
		year = today.Year()
	}
	co, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return Dashboard{}, err
	}
	d := Dashboard{Year: year, Today: today, Net: co.VATPayer, Events: []calendar.Event{}, Overdue: []calendar.Event{}}
	can := func(p auth.Perm) bool { return a.Require(ctx, companyID, p) == nil }

	if can(auth.PermLedgerRead) {
		if d.Money, err = a.dashMoney(ctx, companyID, co, year); err != nil {
			return d, err
		}
		at := today
		if year < today.Year() {
			at = invoice.MustDate(fmt.Sprintf("%04d-12-31", year))
		}
		open, err := a.OpenItemsAt(ctx, companyID, at)
		if err != nil {
			return d, err
		}
		d.Receivables, d.Payables = dashOpen(open.Receivables), dashOpen(open.Payables)
	}
	if can(auth.PermInvoicesRead) || can(auth.PermPurchasesRead) {
		d.Months = make([]DashMonth, 12)
		for i := range d.Months {
			d.Months[i].Month = i + 1
		}
		pick := func(s money.Dec, b money.Dec) money.Dec {
			if co.VATPayer {
				return b
			}
			return s
		}
		if can(auth.PermInvoicesRead) {
			invs, err := a.Store.ListInvoices(ctx, companyID, store.InvoiceFilter{})
			if err != nil {
				return d, err
			}
			for i := range invs {
				inv := &invs[i]
				if inv.Status == invoice.StatusDraft || inv.Status == invoice.StatusCancelled ||
					(inv.Kind != invoice.KindInvoice && inv.Kind != invoice.KindCreditNote) || inv.DeliveryDate.Year() != year {
					continue
				}
				ts, ok := inv.TaxSummary()
				if !ok {
					continue
				}
				m := &d.Months[int(inv.DeliveryDate.Month())-1]
				m.Income = m.Income.Add(pick(ts.Total, ts.Base))
			}
		}
		if can(auth.PermPurchasesRead) {
			ps, err := a.Store.ListPurchases(ctx, companyID, store.PurchaseFilter{})
			if err != nil {
				return d, err
			}
			for i := range ps {
				p := &ps[i]
				if (p.Status != purchase.StatusBooked && p.Status != purchase.StatusPaid) || p.DeliveryDate.Year() != year {
					continue
				}
				t, ok := p.Tax()
				if !ok {
					continue
				}
				m := &d.Months[int(p.DeliveryDate.Month())-1]
				m.Expense = m.Expense.Add(pick(t.Summary.Total, t.Summary.Base))
			}
		}
		for _, m := range d.Months {
			d.Income, d.Expense = d.Income.Add(m.Income), d.Expense.Add(m.Expense)
		}
		d.Result = d.Income.Sub(d.Expense)
	}

	cal, err := a.Calendar(ctx, companyID, today, today.AddDays(14))
	if err != nil {
		return d, err
	}
	for _, e := range cal.Events {
		if !e.Done && e.Kind != calendar.KindHoliday {
			d.Events = append(d.Events, e)
		}
	}
	d.Overdue = append(d.Overdue, cal.Overdue...)
	if len(d.Overdue) > dashTop {
		d.Overdue = d.Overdue[len(d.Overdue)-dashTop:] // najnovšie zmeškané
	}

	if can(auth.PermInvoicesRead) {
		w, err := a.dashWork(ctx, companyID, today, can(auth.PermPurchasesRead))
		if err != nil {
			return d, err
		}
		d.Work = &w
	}
	return d, nil
}

// dashOpen zhrnie neuhradené doklady: spolu, po splatnosti a pár najdôležitejších.
func dashOpen(items []journal.OpenItem) *DashOpen {
	o := &DashOpen{Top: []journal.OpenItem{}}
	for _, it := range items {
		if it.RemainEUR.Sign() <= 0 {
			continue
		}
		o.Total, o.Count = o.Total.Add(it.RemainEUR), o.Count+1
		if it.DaysLate > 0 {
			o.Overdue, o.OverdueCount = o.Overdue.Add(it.RemainEUR), o.OverdueCount+1
		}
		o.Top = append(o.Top, it)
	}
	sort.SliceStable(o.Top, func(i, k int) bool {
		a, b := o.Top[i], o.Top[k]
		if a.DaysLate != b.DaysLate {
			return a.DaysLate > b.DaysLate
		}
		return a.DueDate.Before(b.DueDate)
	})
	if len(o.Top) > dashTop {
		o.Top = o.Top[:dashTop]
	}
	return o
}

// dashMoney: zostatky pokladníc a účtov — z peňažného denníka
// (jednoduché účtovníctvo) alebo z hlavnej knihy (účty 211 a 221).
func (a *App) dashMoney(ctx context.Context, companyID int64, co store.Company, year int) (*DashMoney, error) {
	m := &DashMoney{Items: []DashMoneyItem{}}
	if co.Bookkeeping == store.BookkeepingDouble {
		lb, err := a.Ledger(ctx, companyID, year)
		if err != nil {
			return nil, err
		}
		analytic := map[string]bool{}
		for _, t := range lb.Accounts {
			if len(t.Account) > 3 {
				analytic[t.Account[:3]] = true
			}
		}
		for _, t := range lb.Accounts {
			syn := t.Account
			if len(syn) > 3 {
				syn = syn[:3]
			}
			if (syn != "211" && syn != "221") || (len(t.Account) == 3 && analytic[syn]) {
				continue
			}
			kind := "bank"
			if syn == "211" {
				kind = "cash"
			}
			m.Items = append(m.Items, DashMoneyItem{Kind: kind, Label: firstNonBlank(t.Name, t.Account), Account: t.Account, Amount: t.Closing})
		}
	} else {
		jb, err := a.Journal(ctx, companyID, year)
		if err != nil {
			return nil, err
		}
		for _, b := range jb.Balances {
			kind := string(b.Means)
			if kind != "cash" && kind != "bank" {
				continue
			}
			label := b.Account
			if label == "" {
				label = map[string]string{"cash": "Pokladnica", "bank": "Bankový účet"}[kind]
			}
			m.Items = append(m.Items, DashMoneyItem{Kind: kind, Label: label, Amount: b.Closing})
		}
	}
	sort.SliceStable(m.Items, func(i, k int) bool {
		if m.Items[i].Kind != m.Items[k].Kind {
			return m.Items[i].Kind == "bank"
		}
		return strings.ToLower(m.Items[i].Label) < strings.ToLower(m.Items[k].Label)
	})
	for _, it := range m.Items {
		m.Total = m.Total.Add(it.Amount)
	}
	return m, nil
}

// dashWork spočíta rozpracovanú prácu.
func (a *App) dashWork(ctx context.Context, companyID int64, today invoice.Date, stock bool) (DashWork, error) {
	var w DashWork
	jobs, err := a.CrmJobs(ctx, companyID)
	if err != nil {
		return w, err
	}
	for _, j := range jobs {
		if !j.Status.Open() {
			continue
		}
		w.Jobs++
		w.Pipeline = w.Pipeline.Add(j.Budget)
		if j.Late {
			w.JobsLate++
		}
	}
	orders, err := a.salesOrders(ctx, companyID)
	if err != nil {
		return w, err
	}
	for _, o := range orders {
		if !o.Status.Active() {
			continue
		}
		w.Orders++
		if !o.Due.IsZero() && o.Due.Before(today) {
			w.OrdersLate++
		}
	}
	qs, err := a.Store.ListQuotes(ctx, companyID, 0)
	if err != nil {
		return w, err
	}
	for _, q := range qs {
		if (q.Status == quote.StatusIssued || q.Status == quote.StatusAccepted) && !q.ValidUntil.Before(today) && (q.Currency == "" || q.Currency == "EUR") {
			w.Quotes++
			w.QuotesTotal = w.QuotesTotal.Add(q.Summary.Total)
		}
	}
	invs, err := a.Store.ListInvoices(ctx, companyID, store.InvoiceFilter{Status: invoice.StatusDraft})
	if err != nil {
		return w, err
	}
	w.Drafts = len(invs)
	if stock {
		rows, err := a.Reorder(ctx, companyID)
		if err != nil {
			return w, err
		}
		w.StockLow = len(rows)
	}
	return w, nil
}
