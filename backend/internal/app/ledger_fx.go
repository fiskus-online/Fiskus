package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/chart"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/journal"
	"github.com/fiskus-online/fiskus/backend/internal/ledger"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// Kurzové rozdiely k 31. 12.: neuhradené pohľadávky a záväzky v cudzej mene
// a zostatky pokladníc a bankových účtov v cudzej mene sa prepočítajú
// referenčným kurzom ECB ku dňu závierky (§ 24 ods. 2 zákona o účtovníctve);
// rozdiel oproti sume v účtovníctve je kurzový zisk (663) alebo strata (563).
// Prepočítaná suma pohľadávky a záväzku platí aj pri úhrade v nasledujúcom roku.

// FXItem je jedna prepočítaná pohľadávka alebo záväzok.
type FXItem struct {
	Key       string    `json:"key"`  // sale:ID | purchase:ID | cash:ID | bank:IBAN
	Kind      string    `json:"kind"` // receivable | payable | cash | bank
	Account   string    `json:"account"`
	Number    string    `json:"number"`
	Partner   string    `json:"partner"`
	Currency  string    `json:"currency"`
	Remaining money.Dec `json:"remaining"` // v cudzej mene
	Rate      money.Dec `json:"rate"`      // jednotiek meny za 1 EUR
	RateDate  string    `json:"rateDate"`
	BookEUR   money.Dec `json:"bookEur"` // v účtovníctve
	NewEUR    money.Dec `json:"newEur"`  // podľa kurzu k 31. 12.
	Diff      money.Dec `json:"diff"`    // + zisk, − strata
}

// FXRevaluation je návrh kurzových rozdielov k 31. 12.
type FXRevaluation struct {
	Year  int           `json:"year"`
	Items []FXItem      `json:"items"`
	Lines []ledger.Line `json:"lines"`
	// Missing sú meny bez kurzu k 31. 12. (treba aktualizovať kurzy).
	Missing []string `json:"missing"`
	// Warnings sú účty v cudzej mene, ktoré sa nedajú prepočítať.
	Warnings []string `json:"warnings"`
	// Posted je číslo interného dokladu, ak sú rozdiely zaúčtované.
	Posted string `json:"posted,omitempty"`
	// cashBook sú zostatky pokladníc v cudzej mene v eurách k 31. 12.
	cashBook map[string]money.Dec
}

// FXRevaluationFor navrhne kurzové rozdiely k 31. 12. roka.
func (a *App) FXRevaluationFor(ctx context.Context, companyID int64, year int) (FXRevaluation, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerRead); err != nil {
		return FXRevaluation{}, err
	}
	out := FXRevaluation{Year: year, Items: []FXItem{}, Lines: []ledger.Line{}, Missing: []string{}, Warnings: []string{}}
	if year < 2001 || year > 2100 {
		return out, mfaFieldErr("year", "Neplatný rok")
	}
	end := invoice.MustDate(fmt.Sprintf("%d-12-31", year))
	invs, err := a.Store.ListInvoices(ctx, companyID, store.InvoiceFilter{})
	if err != nil {
		return out, err
	}
	purs, err := a.Store.ListPurchases(ctx, companyID, store.PurchaseFilter{})
	if err != nil {
		return out, err
	}
	var prev map[string]money.Dec
	if d, err := a.loadLedger(ctx, companyID, year-1); err == nil {
		prev = d.Revalued
	}
	if d, err := a.loadLedger(ctx, companyID, year); err == nil {
		for _, doc := range d.Internal {
			if doc.FX {
				out.Posted = doc.Number
			}
		}
	}
	rates := map[string]FXItem{}
	missing := map[string]bool{}
	rateOf := func(cur string) (money.Dec, string, bool) {
		if r, ok := rates[cur]; ok {
			return r.Rate, r.RateDate, true
		}
		r, err := a.Store.RateBefore(ctx, cur, fmt.Sprintf("%d-01-01", year+1))
		if err != nil || r.Rate.Sign() <= 0 {
			missing[cur] = true
			return money.Zero, "", false
		}
		rates[cur] = FXItem{Rate: r.Rate, RateDate: r.Date}
		return r.Rate, r.Date, true
	}
	add := func(kind, key string, it journal.OpenItem) {
		if it.Currency == "EUR" || it.Remaining.IsZero() {
			return
		}
		rate, date, ok := rateOf(it.Currency)
		if !ok {
			return
		}
		book := it.RemainEUR
		if v, ok := prev[key]; ok {
			book = v
		}
		acc := "311"
		if kind == "payable" {
			acc = "321"
		}
		x := FXItem{Key: key, Kind: kind, Account: acc, Number: it.Number, Partner: it.Partner, Currency: it.Currency, Remaining: it.Remaining,
			Rate: rate, RateDate: date, BookEUR: book, NewEUR: it.Remaining.DivCents(rate)}
		x.Diff = x.NewEUR.Sub(x.BookEUR)
		if kind == "payable" {
			x.Diff = x.Diff.Neg() // vyšší záväzok je strata
		}
		if !x.Diff.IsZero() {
			out.Items = append(out.Items, x)
		}
	}
	var open []invoice.Invoice
	for _, inv := range invs {
		if inv.Kind != invoice.KindProforma && inv.Foreign() {
			open = append(open, inv)
		}
	}
	for _, it := range journal.Receivables(open, end) {
		add("receivable", "sale:"+strconv.FormatInt(it.ID, 10), it)
	}
	for _, it := range journal.Payables(purs, end) {
		add("payable", "purchase:"+strconv.FormatInt(it.ID, 10), it)
	}
	if err := a.fxMoney(ctx, companyID, year, prev, rateOf, &out); err != nil {
		return out, err
	}
	for c := range missing {
		out.Missing = append(out.Missing, c)
	}
	sort.Strings(out.Missing)
	out.Lines = fxRevaluationLines(out.Items)
	return out, nil
}

// fxRevaluationLines zaúčtuje rozdiely: zisk D 663, strata MD 563, proti
// účtu odberateľov (311) alebo dodávateľov (321). Rovnaké účty a strany zlúči.
func fxRevaluationLines(items []FXItem) []ledger.Line {
	type k struct {
		acc   string
		debit bool
	}
	sums := map[k]money.Dec{}
	var order []k
	put := func(acc string, debit bool, v money.Dec) {
		key := k{acc, debit}
		if _, ok := sums[key]; !ok {
			order = append(order, key)
		}
		sums[key] = sums[key].Add(v)
	}
	for _, it := range items {
		acc := it.Account
		gain := it.Diff.Sign() > 0
		v := it.Diff
		if !gain {
			v = v.Neg()
		}
		asset := it.Kind != "payable" // pohľadávka, pokladnica, banka
		switch {
		case asset && gain: // majetok vzrástol
			put(acc, true, v)
			put("663", false, v)
		case asset:
			put("563", true, v)
			put(acc, false, v)
		case gain: // záväzok klesol
			put(acc, true, v)
			put("663", false, v)
		default:
			put("563", true, v)
			put(acc, false, v)
		}
	}
	out := []ledger.Line{}
	for _, key := range order {
		l := ledger.Line{Account: key.acc}
		if key.debit {
			l.Debit = sums[key]
		} else {
			l.Credit = sums[key]
		}
		out = append(out, l)
	}
	return out
}

// ErrNoFXDifferences — k 31. 12. nie sú kurzové rozdiely.
var ErrNoFXDifferences = errors.New("k 31. 12. nie sú žiadne kurzové rozdiely")

// PostFXRevaluation zaúčtuje kurzové rozdiely k 31. 12. ako interný doklad
// (predchádzajúci doklad s kurzovými rozdielmi toho roka nahradí).
func (a *App) PostFXRevaluation(ctx context.Context, companyID int64, year int) (InternalDoc, error) {
	rv, err := a.FXRevaluationFor(ctx, companyID, year)
	if err != nil {
		return InternalDoc{}, err
	}
	if len(rv.Missing) > 0 {
		return InternalDoc{}, mfaFieldErr("rates", "Chýba kurz ECB k 31. 12. pre "+fmt.Sprint(rv.Missing)+" — aktualizujte kurzy")
	}
	if len(rv.Items) == 0 {
		return InternalDoc{}, mfaFieldErr("items", ErrNoFXDifferences.Error())
	}
	var saved InternalDoc
	err = a.editLedger(ctx, companyID, year, func(d *LedgerData, c chart.Chart, _ []chart.Template) (string, error) {
		lines := append([]ledger.Line(nil), rv.Lines...)
		if err := ledger.CheckLines(lines, c); err != nil {
			return "", mfaFieldErr("lines", err.Error())
		}
		doc := InternalDoc{Date: invoice.MustDate(fmt.Sprintf("%d-12-31", year)), Text: fmt.Sprintf("Kurzové rozdiely k 31. 12. %d", year),
			Lines: lines, FX: true}
		for i, x := range d.Internal {
			if x.FX {
				doc.ID, doc.Number = x.ID, x.Number
				d.Internal[i] = doc
			}
		}
		if doc.ID == 0 {
			d.NextID++
			doc.ID = d.NextID
			doc.Number = fmt.Sprintf("ID%d%03d", year, doc.ID)
			d.Internal = append(d.Internal, doc)
		}
		d.Revalued = map[string]money.Dec{}
		for _, it := range rv.Items {
			d.Revalued[it.Key] = it.NewEUR
		}
		// pokladnice v cudzej mene: aj bez rozdielu, aby ďalší rok vychádzal
		// z prepočítanej sumy
		for k, v := range rv.cashBook {
			if _, ok := d.Revalued[k]; !ok {
				d.Revalued[k] = v
			}
		}
		saved = doc
		return fmt.Sprintf("Kurzové rozdiely k 31. 12. %d (%d dokladov) %s", year, len(rv.Items), doc.Number), nil
	})
	return saved, err
}

// fxPaymentBook vráti, akou sumou v eurách sa platba (bankový pohyb txID)
// faktúry v cudzej mene odpíše z účtu odberateľa: pomernou časťou sumy
// v účtovníctve podľa uhradenej sumy v mene faktúry, posledná platba
// zvyškom (bez haliernych rozdielov). Ak je faktúra prepočítaná k 31. 12.
// minulého roka (rv), delí sa prepočítaná suma medzi platby po tomto dni.
func fxPaymentBook(inv *invoice.Invoice, txID int64, year int, rv money.Dec, hasRV bool) (money.Dec, bool) {
	sum, ok := inv.TaxSummary()
	if !ok {
		return money.Zero, false
	}
	pays := make([]invoice.Payment, 0, len(inv.Payments))
	found := false
	for _, p := range inv.Payments {
		if p.Source == invoice.PaymentBank && p.Ref == txID {
			found = true
		}
		pays = append(pays, p)
	}
	if !found {
		return money.Zero, false
	}
	sort.SliceStable(pays, func(i, j int) bool {
		if pays[i].Date != pays[j].Date {
			return pays[i].Date.Before(pays[j].Date)
		}
		return pays[i].Ref < pays[j].Ref
	})
	base, foreign := sum.Total, inv.Summary.Total
	if hasRV {
		cut := invoice.MustDate(fmt.Sprintf("%d-12-31", year-1))
		kept := pays[:0]
		for _, p := range pays {
			if p.Date.After(cut) {
				kept = append(kept, p)
			} else {
				foreign = foreign.Sub(p.Amount)
			}
		}
		pays, base = kept, rv
	}
	if foreign.Sign() <= 0 {
		return money.Zero, false
	}
	cum, alloc := money.Zero, money.Zero
	for _, p := range pays {
		cum = cum.Add(p.Amount)
		v := p.Amount.MulRatio(base, foreign).RoundCents()
		if cum.Cmp(foreign) >= 0 {
			v = base.Sub(alloc) // posledná platba: zvyšok
		}
		if p.Source == invoice.PaymentBank && p.Ref == txID {
			return v, true
		}
		alloc = alloc.Add(v)
	}
	return money.Zero, false
}

// fxMoney prepočíta zostatky pokladníc a bankových účtov v cudzej mene.
//
// Pokladnica: zostatok v mene je súčet dokladov (pokladnica začína nulou),
// suma v účtovníctve je súčet dokladov v eurách (kurz ECB ku dňu dokladu),
// od minulého roka prepočítaná suma k 31. 12. plus doklady roka.
//
// Bankový účet: pohyby v cudzej mene sa účtujú ručne v eurách na
// analytický účet banky, preto suma v účtovníctve je zostatok tohto účtu
// v hlavnej knihe (bez kurzových rozdielov k 31. 12. tohto roka). Zostatok
// v mene je súčet pohybov vo Fiškuse alebo suma podľa výpisu, ak je zadaná.
func (a *App) fxMoney(ctx context.Context, companyID int64, year int, prev map[string]money.Dec,
	rateOf func(string) (money.Dec, string, bool), out *FXRevaluation) error {
	end := invoice.MustDate(fmt.Sprintf("%d-12-31", year))
	cb, err := a.loadChart(ctx, companyID)
	if err != nil {
		return err
	}
	d, err := a.loadLedger(ctx, companyID, year)
	if err != nil {
		return err
	}
	item := func(kind, key, label, acc, cur string, balance, book money.Dec) {
		rate, date, ok := rateOf(cur)
		if !ok {
			return
		}
		x := FXItem{Key: key, Kind: kind, Account: acc, Number: label, Currency: cur, Remaining: balance,
			Rate: rate, RateDate: date, BookEUR: book, NewEUR: balance.DivCents(rate)}
		x.Diff = x.NewEUR.Sub(x.BookEUR)
		if kind == "cash" {
			if out.cashBook == nil {
				out.cashBook = map[string]money.Dec{}
			}
			out.cashBook[key] = x.NewEUR
		}
		if !x.Diff.IsZero() {
			out.Items = append(out.Items, x)
		}
	}

	// pokladnice
	regs, err := a.cashRegisters(ctx, companyID)
	if err != nil {
		return err
	}
	for _, r := range regs {
		if r.Currency == "" || r.Currency == "EUR" {
			continue
		}
		key := "cash:" + strconv.FormatInt(r.ID, 10)
		docs, err := a.registerDocs(ctx, companyID, r.ID)
		if err != nil {
			return err
		}
		balance, book := money.Zero, money.Zero
		pv, hasPrev := prev[key]
		if hasPrev {
			book = pv
		}
		for _, doc := range docs {
			if doc.Cancelled || end.Before(doc.Date) {
				continue
			}
			balance = balance.Add(doc.Signed())
			if !hasPrev || doc.Date.Year() == year {
				book = book.Add(doc.SignedEUR())
			}
		}
		acc := cb.Money[key]
		if acc == "" {
			acc = "211"
		}
		item("cash", key, r.Name, acc, r.Currency, balance, book)
	}

	// bankové účty v cudzej mene
	txs, err := a.Store.ListBankTransactions(ctx, companyID, store.BankFilter{})
	if err != nil {
		return err
	}
	type acct struct {
		cur     string
		balance money.Dec
		mixed   bool
	}
	banks := map[string]*acct{}
	var order []string
	for _, t := range txs {
		if t.Currency == "" || t.Currency == "EUR" || end.Before(t.Date) {
			continue
		}
		key := "bank:" + normIBAN(t.Account)
		b := banks[key]
		if b == nil {
			b = &acct{cur: t.Currency}
			banks[key] = b
			order = append(order, key)
		}
		if b.cur != t.Currency {
			b.mixed = true
		}
		b.balance = b.balance.Add(t.Signed())
	}
	if len(order) == 0 {
		return nil
	}
	book, err := a.Ledger(ctx, companyID, year)
	if err != nil {
		return err
	}
	closing := map[string]money.Dec{}
	for _, t := range book.Accounts {
		closing[t.Account] = t.Closing
	}
	// kurzové rozdiely k 31. 12. tohto roka sa do sumy v účtovníctve nepočítajú
	for _, doc := range d.Internal {
		if doc.FX {
			for _, l := range doc.Lines {
				closing[l.Account] = closing[l.Account].Sub(l.Debit).Add(l.Credit)
			}
		}
	}
	unposted := map[string]int{}
	for _, ps := range book.Postings {
		if ps.Problem != "" && strings.HasPrefix(ps.Key, "bank:") {
			if id, err := strconv.ParseInt(strings.TrimPrefix(ps.Key, "bank:"), 10, 64); err == nil {
				for _, t := range txs {
					if t.ID == id && t.Currency != "" && t.Currency != "EUR" {
						unposted["bank:"+normIBAN(t.Account)]++
					}
				}
			}
		}
	}
	sort.Strings(order)
	for _, key := range order {
		b := banks[key]
		iban := strings.TrimPrefix(key, "bank:")
		acc := cb.Money[key]
		switch {
		case b.mixed:
			out.Warnings = append(out.Warnings, "Účet "+iban+" má pohyby vo viacerých menách — prepočítajte ho ručne interným dokladom")
			continue
		case acc == "":
			out.Warnings = append(out.Warnings, fmt.Sprintf("Účet %s (%s) nemá priradený analytický účet — priraďte ho v Účty bánk a pokladníc", iban, b.cur))
			continue
		case unposted[key] > 0:
			out.Warnings = append(out.Warnings, fmt.Sprintf("Účet %s (%s): %d pohybov nie je zaúčtovaných — najprv ich zaúčtujte v eurách", iban, b.cur, unposted[key]))
			continue
		}
		balance := b.balance
		if v, ok := d.FXStatement[key]; ok {
			balance = v
		}
		item("bank", key, "Banka "+iban, acc, b.cur, balance, closing[acc])
	}
	return nil
}

// SetFXStatement uloží zostatok bankového účtu v cudzej mene k 31. 12. podľa
// výpisu (nil = podľa pohybov vo Fiškuse).
func (a *App) SetFXStatement(ctx context.Context, companyID int64, year int, key string, balance *money.Dec) error {
	if !strings.HasPrefix(key, "bank:") {
		return mfaFieldErr("key", "Zostatok podľa výpisu sa zadáva iba pri bankovom účte")
	}
	return a.editLedger(ctx, companyID, year, func(d *LedgerData, _ chart.Chart, _ []chart.Template) (string, error) {
		if balance == nil {
			delete(d.FXStatement, key)
			return fmt.Sprintf("Zostatok %s k 31. 12. %d podľa pohybov", key, year), nil
		}
		if d.FXStatement == nil {
			d.FXStatement = map[string]money.Dec{}
		}
		d.FXStatement[key] = *balance
		return fmt.Sprintf("Zostatok %s k 31. 12. %d podľa výpisu: %s", key, year, balance.Display()), nil
	})
}
