package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/bank"
	"github.com/palo-krnac/fiscus/backend/internal/household"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/journal"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Domácnosť (domáce účtovníctvo) je firma s druhom účtovníctva
// „household“. Oprávnenia sú ako pri účtovníctve (ledger.read/write).

// ErrNotHousehold — operácia domácnosti nad firmou, ktorá domácnosťou nie je.
var ErrNotHousehold = &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "company", Message: "Toto nie je domácnosť — domáce účtovníctvo má vlastnú „firmu“ druhu Domácnosť"}}}

func (a *App) requireHousehold(ctx context.Context, companyID int64, p auth.Perm) error {
	if err := a.Require(ctx, companyID, p); err != nil {
		return err
	}
	c, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return err
	}
	if c.Bookkeeping != store.BookkeepingHousehold {
		return ErrNotHousehold
	}
	return nil
}

func householdKey(companyID int64) string { return "household." + strconv.FormatInt(companyID, 10) }

// HouseholdSettings sú kategórie a rozpočet domácnosti.
type HouseholdSettings struct {
	Categories []household.Category `json:"categories"`
}

func (a *App) householdSettings(ctx context.Context, companyID int64) (HouseholdSettings, error) {
	v, err := a.Store.GetSetting(ctx, householdKey(companyID))
	if errors.Is(err, store.ErrNotFound) {
		return HouseholdSettings{Categories: append([]household.Category(nil), household.DefaultCategories...)}, nil
	}
	if err != nil {
		return HouseholdSettings{}, err
	}
	var s HouseholdSettings
	if err := json.Unmarshal([]byte(v), &s); err != nil {
		return s, err
	}
	return s, nil
}

// HouseholdCategories vráti kategórie s rozpočtom.
func (a *App) HouseholdCategories(ctx context.Context, companyID int64) ([]household.Category, error) {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerRead); err != nil {
		return nil, err
	}
	s, err := a.householdSettings(ctx, companyID)
	return s.Categories, err
}

// SaveHouseholdCategories uloží kategórie a mesačný rozpočet.
func (a *App) SaveHouseholdCategories(ctx context.Context, companyID int64, cats []household.Category) ([]household.Category, error) {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var in, out int
	for i := range cats {
		c := &cats[i]
		c.Name = strings.TrimSpace(c.Name)
		c.Budget = c.Budget.RoundCents()
		f := fmt.Sprintf("categories.%d", i)
		switch {
		case c.Name == "":
			return nil, vErr(f, "Zadajte názov kategórie")
		case len([]rune(c.Name)) > 60:
			return nil, vErr(f, "Názov kategórie môže mať najviac 60 znakov")
		case seen[strings.ToLower(c.Name)]:
			return nil, vErr(f, "Kategória „"+c.Name+"“ je dvakrát")
		case c.Budget.Sign() < 0:
			return nil, vErr(f, "Rozpočet nemôže byť záporný")
		}
		seen[strings.ToLower(c.Name)] = true
		if c.Income {
			in++
		} else {
			out++
		}
	}
	if in == 0 || out == 0 {
		return nil, vErr("categories", "Domácnosť potrebuje aspoň jednu kategóriu príjmov a jednu výdavkov")
	}
	before, err := a.householdSettings(ctx, companyID)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(HouseholdSettings{Categories: cats})
	if err != nil {
		return nil, err
	}
	if err := a.Store.SetSetting(ctx, householdKey(companyID), string(data)); err != nil {
		return nil, err
	}
	a.record(ctx, companyID, audit.CatLedger, "update", "household_categories", 0, "Domácnosť: kategórie a rozpočet", before.Categories, cats)
	return cats, nil
}

// HouseholdAccount je účet so zostatkom.
type HouseholdAccount struct {
	household.Account
	Balance money.Dec `json:"balance"`
	Entries int       `json:"entries"`
}

// HouseholdAccounts vráti účty domácnosti so zostatkami.
func (a *App) HouseholdAccounts(ctx context.Context, companyID int64) ([]HouseholdAccount, error) {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerRead); err != nil {
		return nil, err
	}
	accs, err := a.Store.ListHHAccounts(ctx, companyID)
	if err != nil {
		return nil, err
	}
	es, err := a.Store.ListHHEntries(ctx, companyID, 0)
	if err != nil {
		return nil, err
	}
	bal := household.Balances(accs, es)
	n := map[int64]int{}
	for _, e := range es {
		n[e.AccountID]++
		if e.ToAccountID != 0 {
			n[e.ToAccountID]++
		}
	}
	out := make([]HouseholdAccount, 0, len(accs))
	for _, x := range accs {
		out = append(out, HouseholdAccount{Account: x, Balance: bal[x.ID], Entries: n[x.ID]})
	}
	return out, nil
}

// SaveHouseholdAccount uloží účet domácnosti (ID 0 = nový).
func (a *App) SaveHouseholdAccount(ctx context.Context, companyID int64, x household.Account) (household.Account, error) {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return x, err
	}
	x.Normalize()
	if err := x.Validate(); err != nil {
		return x, err
	}
	accs, err := a.Store.ListHHAccounts(ctx, companyID)
	if err != nil {
		return x, err
	}
	var before *household.Account
	for _, o := range accs {
		if o.ID == x.ID {
			o := o
			before = &o
		} else if strings.EqualFold(o.Name, x.Name) {
			return x, vErr("name", "Účet „"+x.Name+"“ už existuje")
		}
	}
	if x.ID != 0 && before == nil {
		return x, store.ErrNotFound
	}
	if err := a.Store.SaveHHAccount(ctx, companyID, &x); err != nil {
		return x, err
	}
	a.record(ctx, companyID, audit.CatLedger, "save", "household_account", x.ID, "Domácnosť: účet "+x.Name, before, x)
	return x, nil
}

// DeleteHouseholdAccount zmaže účet bez pohybov (s pohybmi ho treba zrušiť).
func (a *App) DeleteHouseholdAccount(ctx context.Context, companyID, id int64) error {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return err
	}
	accs, err := a.HouseholdAccounts(ctx, companyID)
	if err != nil {
		return err
	}
	for _, x := range accs {
		if x.ID != id {
			continue
		}
		if x.Entries > 0 {
			return vErr("id", "Účet má pohyby — namiesto zmazania ho označte ako zrušený")
		}
		if err := a.Store.DeleteHHAccount(ctx, companyID, id); err != nil {
			return err
		}
		a.record(ctx, companyID, audit.CatLedger, "delete", "household_account", id, "Domácnosť: zmazaný účet "+x.Name, x.Account, nil)
		return nil
	}
	return store.ErrNotFound
}

// HouseholdEntries vráti pohyby za rok (0 = všetky).
func (a *App) HouseholdEntries(ctx context.Context, companyID int64, year int) ([]household.Entry, error) {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerRead); err != nil {
		return nil, err
	}
	return a.Store.ListHHEntries(ctx, companyID, year)
}

// checkEntry overí účty a kategóriu pohybu.
func (a *App) checkEntry(ctx context.Context, companyID int64, e household.Entry, cats []household.Category) error {
	if err := e.Validate(); err != nil {
		return err
	}
	accs, err := a.Store.ListHHAccounts(ctx, companyID)
	if err != nil {
		return err
	}
	have := map[int64]bool{}
	for _, x := range accs {
		have[x.ID] = true
	}
	if !have[e.AccountID] {
		return vErr("accountId", "Účet neexistuje")
	}
	if e.Transfer() && !have[e.ToAccountID] {
		return vErr("toAccountId", "Účet neexistuje")
	}
	if e.ObligationID != 0 {
		if _, err := a.obligation(ctx, companyID, e.ObligationID); err != nil {
			return vErr("obligationId", "Záväzok neexistuje")
		}
	}
	if !e.Transfer() {
		have := map[string]bool{}
		for _, c := range cats {
			have[c.Name] = true
		}
		for i, x := range e.Split {
			if !have[x.Category] {
				return vErr(fmt.Sprintf("split.%d", i), "Kategória „"+x.Category+"“ nie je v zozname")
			}
		}
		if !have[e.Category] {
			return vErr("category", "Kategória „"+e.Category+"“ nie je v zozname — doplňte ju v kategóriách")
		}
	}
	return nil
}

// SaveHouseholdEntry uloží pohyb (ID 0 = nový).
func (a *App) SaveHouseholdEntry(ctx context.Context, companyID int64, e household.Entry) (household.Entry, error) {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return e, err
	}
	e.Normalize()
	var before *household.Entry
	if e.ID != 0 {
		old, err := a.Store.GetHHEntry(ctx, companyID, e.ID)
		if err != nil {
			return e, err
		}
		before = &old
		e.Source = old.Source   // pôvod importu sa nemení
		e.Receipt = old.Receipt // údaje bločku sú záznam z eKasy/fotky, neupravujú sa
	} else if !strings.HasPrefix(e.Source, "ekasa:") && !strings.HasPrefix(e.Source, "photo:") {
		e.Source = "" // nový pohyb: pôvod iba z návrhu bločku (proti dvojitému zápisu)
		e.Receipt = nil
	} else if e.Receipt != nil && len(e.Receipt.Items) > 1000 {
		return e, vErr("receipt", "Bloček má priveľa položiek")
	}
	s, err := a.householdSettings(ctx, companyID)
	if err != nil {
		return e, err
	}
	if err := a.checkEntry(ctx, companyID, e, s.Categories); err != nil {
		return e, err
	}
	if err := a.Store.SaveHHEntry(ctx, companyID, &e); errors.Is(err, store.ErrDuplicate) {
		return e, vErr("source", "Tento bloček už je v domácnosti zapísaný")
	} else if err != nil {
		return e, err
	}
	a.record(ctx, companyID, audit.CatLedger, "save", "household_entry", e.ID, "Domácnosť: "+entrySummary(e), before, e)
	return e, nil
}

func entrySummary(e household.Entry) string {
	what := e.Category
	if e.Transfer() {
		what = "prevod"
	}
	s := e.Date.Display() + " " + what + " " + e.Amount.Display() + " €"
	if e.Payee != "" {
		s += " (" + e.Payee + ")"
	}
	return s
}

// DeleteHouseholdEntry zmaže pohyb.
func (a *App) DeleteHouseholdEntry(ctx context.Context, companyID, id int64) error {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return err
	}
	e, err := a.Store.GetHHEntry(ctx, companyID, id)
	if err != nil {
		return err
	}
	if err := a.Store.DeleteHHEntry(ctx, companyID, id); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatLedger, "delete", "household_entry", id, "Domácnosť: zmazaný pohyb "+entrySummary(e), e, nil)
	return nil
}

// HouseholdOverview je prehľad roka s čerpaním rozpočtu v mesiaci.
type HouseholdOverview struct {
	household.Overview
	Month    int                     `json:"month"`
	Budget   []household.MonthBudget `json:"budget"`
	Accounts []HouseholdAccount      `json:"accounts"`
	Total    money.Dec               `json:"total"` // súčet zostatkov otvorených účtov
	// Upcoming sú nezaplatené splátky záväzkov do mesiaca (aj po splatnosti).
	Upcoming []UpcomingDue `json:"upcoming"`
}

// HouseholdSummary zostaví prehľad roka a rozpočet mesiaca (0 = aktuálny,
// v minulom roku december).
func (a *App) HouseholdSummary(ctx context.Context, companyID int64, year, month int) (HouseholdOverview, error) {
	accs, err := a.HouseholdAccounts(ctx, companyID)
	if err != nil {
		return HouseholdOverview{}, err
	}
	s, err := a.householdSettings(ctx, companyID)
	if err != nil {
		return HouseholdOverview{}, err
	}
	today := a.Today()
	if year == 0 {
		year = today.Year()
	}
	if month < 1 || month > 12 {
		month = 12
		if year == today.Year() {
			month = today.Month()
		}
	}
	es, err := a.Store.ListHHEntries(ctx, companyID, year)
	if err != nil {
		return HouseholdOverview{}, err
	}
	ov := household.Summarize(year, s.Categories, es)
	up, err := a.obligationDues(ctx, companyID, today.AddDays(31))
	if err != nil {
		return HouseholdOverview{}, err
	}
	out := HouseholdOverview{Overview: ov, Month: month, Budget: household.Budget(ov, month), Accounts: accs, Upcoming: up}
	for _, x := range accs {
		if !x.Closed {
			out.Total = out.Total.Add(x.Balance)
		}
	}
	return out, nil
}

// HouseholdSuggest navrhne kategóriu pre obchod alebo popis.
func (a *App) HouseholdSuggest(ctx context.Context, companyID int64, text string, income bool) (string, error) {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerRead); err != nil {
		return "", err
	}
	s, err := a.householdSettings(ctx, companyID)
	if err != nil {
		return "", err
	}
	es, err := a.Store.ListHHEntries(ctx, companyID, 0)
	if err != nil {
		return "", err
	}
	return household.Suggest(text, income, es, s.Categories), nil
}

// fallbackCategory: posledná kategória daného druhu (zvyčajne „Iné …“).
func fallbackCategory(cats []household.Category, income bool) string {
	name := ""
	for _, c := range cats {
		if c.Income == income {
			name = c.Name
		}
	}
	return name
}

// HouseholdImport je výsledok importu do domácnosti.
type HouseholdImport struct {
	Added   int               `json:"added"`
	Skipped int               `json:"skipped"` // už importované skôr
	Foreign int               `json:"foreign"` // v cudzej mene (neimportované)
	Entries []household.Entry `json:"entries"`
}

// addImported uloží importované pohyby s kategóriou podľa návrhu.
func (a *App) addImported(ctx context.Context, companyID int64, es []household.Entry, res *HouseholdImport) error {
	s, err := a.householdSettings(ctx, companyID)
	if err != nil {
		return err
	}
	hist, err := a.Store.ListHHEntries(ctx, companyID, 0)
	if err != nil {
		return err
	}
	for _, e := range es {
		if e.Category == "" {
			income := e.Amount.Sign() > 0
			e.Category = household.Suggest(strings.TrimSpace(e.Payee+" "+e.Note), income, hist, s.Categories)
			if e.Category == "" {
				e.Category = fallbackCategory(s.Categories, income)
			}
		}
		e.Normalize()
		if err := e.Validate(); err != nil {
			return err
		}
		err := a.Store.SaveHHEntry(ctx, companyID, &e)
		if errors.Is(err, store.ErrDuplicate) {
			res.Skipped++
			continue
		}
		if err != nil {
			return err
		}
		hist = append(hist, e)
		res.Added++
		res.Entries = append(res.Entries, e)
	}
	return nil
}

// ImportHouseholdBank načíta bankový výpis (camt.053, GPC, CSV/XLSX) na
// účet domácnosti. Opakovaný import toho istého výpisu nič nezdvojí.
func (a *App) ImportHouseholdBank(ctx context.Context, companyID, accountID int64, data []byte) (HouseholdImport, error) {
	var res HouseholdImport
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return res, err
	}
	accs, err := a.Store.ListHHAccounts(ctx, companyID)
	if err != nil {
		return res, err
	}
	var acc *household.Account
	for i := range accs {
		if accs[i].ID == accountID {
			acc = &accs[i]
		}
	}
	if acc == nil {
		return res, vErr("accountId", "Vyberte účet, na ktorý výpis patrí")
	}
	txs, _, err := bank.Parse(data, acc.IBAN)
	if err != nil {
		return res, vErr("file", "Výpis sa nedá načítať: "+err.Error())
	}
	es := make([]household.Entry, 0, len(txs))
	for _, t := range txs {
		if t.Currency != "" && !strings.EqualFold(t.Currency, "EUR") {
			res.Foreign++
			continue
		}
		if acc.IBAN != "" && t.Account != "" && normIBAN(t.Account) != acc.IBAN {
			return res, vErr("file", "Výpis je z účtu "+t.Account+", nie z účtu "+acc.Name)
		}
		es = append(es, household.Entry{Date: t.Date, AccountID: acc.ID, Amount: t.Signed(), Payee: t.CounterName,
			Note: t.Message, Source: "bank:" + t.Fingerprint})
	}
	if err := a.addImported(ctx, companyID, es, &res); err != nil {
		return res, err
	}
	a.record(ctx, companyID, audit.CatLedger, "import", "household_entry", 0,
		fmt.Sprintf("Domácnosť: import výpisu na účet %s — %d nových, %d už bolo", acc.Name, res.Added, res.Skipped), nil, nil)
	return res, nil
}

// BusinessDraw je výber (alebo vklad) podnikateľa v peňažnom denníku živnosti
// — peniaze, ktoré prešli medzi živnosťou a domácnosťou.
type BusinessDraw struct {
	CompanyID   int64        `json:"companyId"`
	CompanyName string       `json:"companyName"`
	Key         string       `json:"key"`
	Date        invoice.Date `json:"date"`
	// Amount z pohľadu domácnosti: výber zo živnosti kladný (príjem),
	// vklad do živnosti záporný (výdavok).
	Amount   money.Dec `json:"amount"`
	Text     string    `json:"text"`
	Imported bool      `json:"imported"`
}

func drawSource(companyID int64, key string) string {
	return "biz:" + strconv.FormatInt(companyID, 10) + ":" + key
}

// BusinessDraws vráti výbery a vklady podnikateľa za rok zo živností
// (jednoduché účtovníctvo), ku ktorým má používateľ prístup: v peňažnom
// denníku sú v stĺpcoch „mimo základu dane“.
func (a *App) BusinessDraws(ctx context.Context, companyID int64, year int) ([]BusinessDraw, error) {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerRead); err != nil {
		return nil, err
	}
	if year == 0 {
		year = a.Today().Year()
	}
	have := map[string]bool{}
	es, err := a.Store.ListHHEntries(ctx, companyID, 0)
	if err != nil {
		return nil, err
	}
	for _, e := range es {
		if e.Source != "" {
			have[e.Source] = true
		}
	}
	cs, err := a.Store.ListCompanies(ctx) // prístup overí Require nižšie
	if err != nil {
		return nil, err
	}
	out := []BusinessDraw{}
	for _, c := range cs {
		if c.Bookkeeping == store.BookkeepingHousehold || c.Bookkeeping == store.BookkeepingDouble {
			continue
		}
		if a.Require(ctx, c.ID, auth.PermLedgerRead) != nil {
			continue
		}
		book, err := a.Journal(ctx, c.ID, year)
		if err != nil {
			return nil, err
		}
		for _, e := range book.Entries {
			amt := money.Zero
			for _, l := range e.Lines {
				// výdavok mimo základu dane (výber) je v živnosti záporný → príjem
				// domácnosti; príjem mimo základu dane (vklad) → výdavok domácnosti
				if l.Column == journal.ExpNonTax || l.Column == journal.IncNonTax {
					amt = amt.Sub(l.Signed())
				}
			}
			if amt.IsZero() {
				continue
			}
			text := strings.TrimSpace(strings.Join(nonEmpty(e.Partner, e.Text, e.Note), " — "))
			out = append(out, BusinessDraw{CompanyID: c.ID, CompanyName: c.Name, Key: e.Key, Date: e.Date, Amount: amt, Text: text,
				Imported: have[drawSource(c.ID, e.Key)]})
		}
	}
	return out, nil
}

func nonEmpty(ss ...string) []string {
	out := []string{}
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// DrawRef je výber zo živnosti na prenos do domácnosti.
type DrawRef struct {
	CompanyID int64  `json:"companyId"`
	Key       string `json:"key"`
}

// ImportBusinessDraws prenesie vybrané výbery a vklady zo živnosti na účet
// domácnosti (výber ako príjem, vklad ako výdavok).
func (a *App) ImportBusinessDraws(ctx context.Context, companyID, accountID int64, year int, refs []DrawRef) (HouseholdImport, error) {
	var res HouseholdImport
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return res, err
	}
	accs, err := a.Store.ListHHAccounts(ctx, companyID)
	if err != nil {
		return res, err
	}
	ok := false
	for _, x := range accs {
		ok = ok || x.ID == accountID
	}
	if !ok {
		return res, vErr("accountId", "Vyberte účet domácnosti")
	}
	draws, err := a.BusinessDraws(ctx, companyID, year)
	if err != nil {
		return res, err
	}
	want := map[string]bool{}
	for _, r := range refs {
		want[drawSource(r.CompanyID, r.Key)] = true
	}
	s, err := a.householdSettings(ctx, companyID)
	if err != nil {
		return res, err
	}
	es := []household.Entry{}
	for _, d := range draws {
		src := drawSource(d.CompanyID, d.Key)
		if !want[src] {
			continue
		}
		cat := fallbackCategory(s.Categories, d.Amount.Sign() > 0)
		if d.Amount.Sign() > 0 {
			// výber zo živnosti: prvá kategória príjmov („Mzda a príjem zo živnosti“)
			for _, c := range s.Categories {
				if c.Income {
					cat = c.Name
					break
				}
			}
		}
		note := "Zo živnosti"
		if d.Amount.Sign() < 0 {
			note = "Vklad do živnosti"
		}
		if d.Text != "" {
			note += ": " + d.Text
		}
		es = append(es, household.Entry{Date: d.Date, AccountID: accountID, Amount: d.Amount, Category: cat, Payee: d.CompanyName,
			Note: note, Source: src})
	}
	if err := a.addImported(ctx, companyID, es, &res); err != nil {
		return res, err
	}
	a.record(ctx, companyID, audit.CatLedger, "import", "household_entry", 0,
		fmt.Sprintf("Domácnosť: prenos zo živnosti — %d nových, %d už bolo", res.Added, res.Skipped), nil, nil)
	return res, nil
}
