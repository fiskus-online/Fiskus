package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/dpfo"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/party"
	"github.com/palo-krnac/fiscus/backend/internal/payroll"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Mzdy: zamestnanci a dohodári firmy a mesačné mzdové obdobia. Výpočet je
// v balíku payroll; tu je evidencia, kontrola údajov a uzavretie mesiaca
// (uzavretý mesiac drží výsledky tak, ako boli vypočítané).

// HealthInsurers sú zdravotné poisťovne (kód → názov).
var HealthInsurers = map[string]string{"24": "Dôvera", "25": "VšZP", "27": "Union"}

type employeeBook struct {
	Next int64              `json:"next"`
	List []payroll.Employee `json:"list"`
}

// PayrollMonth je uložené mzdové obdobie.
type PayrollMonth struct {
	Inputs    []payroll.Input    `json:"inputs"`
	Closed    bool               `json:"closed"`
	ClosedAt  time.Time          `json:"closedAt,omitzero"`
	ClosedBy  string             `json:"closedBy,omitempty"`
	Employees []payroll.Employee `json:"employees,omitempty"` // pri uzavretí
	Results   []payroll.Result   `json:"results,omitempty"`   // pri uzavretí
}

// PayrollRow je riadok výplatnej listiny.
type PayrollRow struct {
	Employee payroll.Employee `json:"employee"`
	Input    payroll.Input    `json:"input"`
	Result   payroll.Result   `json:"result"`
}

// PayrollView je mzdové obdobie s výpočtom.
type PayrollView struct {
	Year     int            `json:"year"`
	Month    int            `json:"month"`
	Closed   bool           `json:"closed"`
	ClosedAt time.Time      `json:"closedAt,omitzero"`
	ClosedBy string         `json:"closedBy,omitempty"`
	Rows     []PayrollRow   `json:"rows"`
	Totals   payroll.Totals `json:"totals"`
	Years    []int          `json:"years"` // roky, za ktoré Fiškus počíta mzdy
}

func employeesKey(cid int64) string { return "payroll.employees." + strconv.FormatInt(cid, 10) }
func payrollKey(cid int64, year, month int) string {
	return fmt.Sprintf("payroll.month.%d.%04d-%02d", cid, year, month)
}

// requireBusiness: mzdy patria firme, nie domácnosti.
func (a *App) requireBusiness(ctx context.Context, companyID int64, p auth.Perm) error {
	if err := a.Require(ctx, companyID, p); err != nil {
		return err
	}
	c, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return err
	}
	if c.Bookkeeping == store.BookkeepingHousehold {
		return vErr("company", "Mzdy patria firme, nie domácnosti")
	}
	return nil
}

func (a *App) loadJSON(ctx context.Context, key string, v any) (bool, error) {
	s, err := a.Store.GetSetting(ctx, key)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(s), v)
}

func (a *App) saveJSON(ctx context.Context, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return a.Store.SetSetting(ctx, key, string(b))
}

func (a *App) employeeBook(ctx context.Context, cid int64) (employeeBook, error) {
	var b employeeBook
	_, err := a.loadJSON(ctx, employeesKey(cid), &b)
	if b.List == nil {
		b.List = []payroll.Employee{}
	}
	return b, err
}

// Employees vráti zamestnancov firmy (podľa priezviska).
func (a *App) Employees(ctx context.Context, cid int64) ([]payroll.Employee, error) {
	if err := a.requireBusiness(ctx, cid, auth.PermLedgerRead); err != nil {
		return nil, err
	}
	b, err := a.employeeBook(ctx, cid)
	return b.List, err
}

func validDate(s string) bool {
	_, err := invoice.ParseDate(s)
	return err == nil
}

// validateEmployee skontroluje údaje zamestnanca (chyby polí formulára).
func validateEmployee(e *payroll.Employee) error {
	var f []invoice.FieldError
	add := func(field, msg string) { f = append(f, invoice.FieldError{Field: field, Message: msg}) }
	e.FirstName, e.LastName = strings.TrimSpace(e.FirstName), strings.TrimSpace(e.LastName)
	e.RC = strings.TrimSpace(e.RC)
	e.IBAN = strings.ToUpper(strings.ReplaceAll(e.IBAN, " ", ""))
	if e.LastName == "" {
		add("lastName", "Zadajte priezvisko")
	}
	if !e.Contract.Valid() {
		add("contract", "Vyberte druh pracovného vzťahu")
	}
	if e.Contract == payroll.TPP {
		e.Regular = true
	}
	if e.RC != "" {
		if _, err := dpfo.BirthDate(e.RC); err != nil {
			add("rc", "Neplatné rodné číslo")
		}
	}
	if e.IBAN != "" && !party.ValidIBAN(e.IBAN) {
		add("iban", "Neplatný IBAN")
	}
	if _, ok := HealthInsurers[e.Insurer]; !ok {
		add("insurer", "Vyberte zdravotnú poisťovňu")
	}
	if !validDate(e.Start) {
		add("start", "Zadajte dátum nástupu")
	}
	if e.End != "" && (!validDate(e.End) || e.End < e.Start) {
		add("end", "Koniec musí byť po nástupe")
	}
	if e.Wage.Sign() <= 0 {
		add("wage", "Zadajte mzdu alebo odmenu")
	}
	if e.Hours.Sign() < 0 || e.Hours.Cmp(money.FromInt(60)) > 0 {
		add("hours", "Týždenný pracovný čas 0 – 60 hodín")
	}
	switch e.Pension {
	case payroll.PensionNone, payroll.PensionOld, payroll.PensionDisabled:
	default:
		add("pension", "Neznámy druh dôchodku")
	}
	if e.OOP && e.Contract != payroll.DoBPS && !(e.Contract.Agreement() && e.Pension != payroll.PensionNone) {
		add("oop", "Odvodová výnimka je iba pri dohode dôchodcu alebo brigádnickej práci študenta")
	}
	if !e.Bonus {
		e.Children = nil
	}
	for i := range e.Children {
		c := &e.Children[i]
		c.Name = strings.TrimSpace(c.Name)
		if c.Name == "" {
			add(fmt.Sprintf("children.%d.name", i), "Zadajte meno dieťaťa")
		}
		if _, err := dpfo.BirthDate(c.RC); err != nil {
			add(fmt.Sprintf("children.%d.rc", i), "Neplatné rodné číslo dieťaťa")
		}
	}
	if e.Bonus && len(e.Children) == 0 {
		add("children", "Pridajte deti, na ktoré sa uplatňuje bonus")
	}
	if len(f) > 0 {
		return &invoice.ValidationError{Fields: f}
	}
	return nil
}

// SaveEmployee založí (ID 0) alebo upraví zamestnanca.
func (a *App) SaveEmployee(ctx context.Context, cid int64, e payroll.Employee) (payroll.Employee, error) {
	if err := a.requireBusiness(ctx, cid, auth.PermLedgerWrite); err != nil {
		return e, err
	}
	if err := validateEmployee(&e); err != nil {
		return e, err
	}
	b, err := a.employeeBook(ctx, cid)
	if err != nil {
		return e, err
	}
	var before *payroll.Employee
	if e.ID == 0 {
		b.Next++
		e.ID = b.Next
		b.List = append(b.List, e)
	} else {
		i := indexEmployee(b.List, e.ID)
		if i < 0 {
			return e, store.ErrNotFound
		}
		old := b.List[i]
		before = &old
		b.List[i] = e
	}
	sort.SliceStable(b.List, func(i, j int) bool {
		return strings.ToLower(b.List[i].Name()) < strings.ToLower(b.List[j].Name())
	})
	if err := a.saveJSON(ctx, employeesKey(cid), b); err != nil {
		return e, err
	}
	action, summary := "create", "Nový zamestnanec "+e.Name()
	if before != nil {
		action, summary = "update", "Úprava zamestnanca "+e.Name()
	}
	// rodné čísla do auditu nepatria v plnom znení
	a.record(ctx, cid, audit.CatPayroll, action, "employee", e.ID, summary, redact(before), redact(&e))
	return e, nil
}

func redact(e *payroll.Employee) any {
	if e == nil {
		return nil
	}
	c := *e
	if len(c.RC) > 4 {
		c.RC = "…" + c.RC[len(c.RC)-4:]
	}
	c.Children = nil
	return c
}

func indexEmployee(list []payroll.Employee, id int64) int {
	for i, e := range list {
		if e.ID == id {
			return i
		}
	}
	return -1
}

// DeleteEmployee zmaže zamestnanca, ktorý nemá žiadnu mzdu v uzavretom mesiaci
// (inak sa vzťah ukončí dátumom konca).
func (a *App) DeleteEmployee(ctx context.Context, cid, id int64) error {
	if err := a.requireBusiness(ctx, cid, auth.PermLedgerWrite); err != nil {
		return err
	}
	b, err := a.employeeBook(ctx, cid)
	if err != nil {
		return err
	}
	i := indexEmployee(b.List, id)
	if i < 0 {
		return store.ErrNotFound
	}
	e := b.List[i]
	if ym, err := a.closedPayrollWith(ctx, cid, e); err != nil {
		return err
	} else if ym != "" {
		return vErr("employee", fmt.Sprintf("%s má uzavretú mzdu za %s — namiesto zmazania zadajte koniec pracovného vzťahu", e.Name(), ym))
	}
	b.List = append(b.List[:i], b.List[i+1:]...)
	if err := a.saveJSON(ctx, employeesKey(cid), b); err != nil {
		return err
	}
	a.record(ctx, cid, audit.CatPayroll, "delete", "employee", id, "Zmazaný zamestnanec "+e.Name(), redact(&e), nil)
	return nil
}

// closedPayrollWith nájde uzavretý mesiac s mzdou zamestnanca (od nástupu).
func (a *App) closedPayrollWith(ctx context.Context, cid int64, e payroll.Employee) (string, error) {
	start, err := invoice.ParseDate(e.Start)
	if err != nil {
		return "", nil
	}
	today := a.Today()
	for y, mo := start.Year(), start.Month(); y < today.Year() || (y == today.Year() && mo <= today.Month()); {
		var pm PayrollMonth
		if ok, err := a.loadJSON(ctx, payrollKey(cid, y, mo), &pm); err != nil {
			return "", err
		} else if ok && pm.Closed {
			for _, r := range pm.Results {
				if r.EmployeeID == e.ID {
					return fmt.Sprintf("%d/%d", mo, y), nil
				}
			}
		}
		if mo++; mo > 12 {
			y, mo = y+1, 1
		}
	}
	return "", nil
}

func checkPeriod(year, month int) error {
	if month < 1 || month > 12 {
		return vErr("month", "Neplatný mesiac")
	}
	if _, err := payroll.ParamsFor(year); err != nil {
		return vErr("year", fmt.Sprintf("Mzdy za rok %d Fiškus zatiaľ nepočíta (vie roky %v)", year, payroll.Years()))
	}
	return nil
}

// Payroll vráti mzdové obdobie: uzavreté tak, ako bolo uložené, otvorené
// vypočítané z aktuálnych údajov zamestnancov.
func (a *App) Payroll(ctx context.Context, cid int64, year, month int) (PayrollView, error) {
	v := PayrollView{Year: year, Month: month, Rows: []PayrollRow{}, Years: payroll.Years()}
	if err := a.requireBusiness(ctx, cid, auth.PermLedgerRead); err != nil {
		return v, err
	}
	if err := checkPeriod(year, month); err != nil {
		return v, err
	}
	var pm PayrollMonth
	if _, err := a.loadJSON(ctx, payrollKey(cid, year, month), &pm); err != nil {
		return v, err
	}
	inputs := map[int64]payroll.Input{}
	for _, in := range pm.Inputs {
		inputs[in.EmployeeID] = in
	}
	insurers := map[int64]string{}
	if pm.Closed {
		v.Closed, v.ClosedAt, v.ClosedBy = true, pm.ClosedAt, pm.ClosedBy
		res := map[int64]payroll.Result{}
		for _, r := range pm.Results {
			res[r.EmployeeID] = r
		}
		for _, e := range pm.Employees {
			v.Rows = append(v.Rows, PayrollRow{Employee: e, Input: inputs[e.ID], Result: res[e.ID]})
			insurers[e.ID] = e.Insurer
		}
	} else {
		b, err := a.employeeBook(ctx, cid)
		if err != nil {
			return v, err
		}
		p, _ := payroll.ParamsFor(year)
		ym := fmt.Sprintf("%04d-%02d", year, month)
		for _, e := range b.List {
			if !e.ActiveIn(ym) {
				continue
			}
			in := inputs[e.ID]
			in.EmployeeID = e.ID
			r, err := payroll.Compute(e, in, p, month)
			if err != nil {
				return v, vErr("employee", e.Name()+": "+err.Error())
			}
			v.Rows = append(v.Rows, PayrollRow{Employee: e, Input: in, Result: r})
			insurers[e.ID] = e.Insurer
		}
	}
	results := make([]payroll.Result, len(v.Rows))
	for i, r := range v.Rows {
		results[i] = r.Result
	}
	v.Totals = payroll.Sum(results, insurers)
	return v, nil
}

// SavePayroll uloží vstupy mesiaca (hodiny, odmeny, PN …) a vráti prepočet.
func (a *App) SavePayroll(ctx context.Context, cid int64, year, month int, inputs []payroll.Input) (PayrollView, error) {
	if err := a.requireBusiness(ctx, cid, auth.PermLedgerWrite); err != nil {
		return PayrollView{}, err
	}
	if err := checkPeriod(year, month); err != nil {
		return PayrollView{}, err
	}
	key := payrollKey(cid, year, month)
	var pm PayrollMonth
	if _, err := a.loadJSON(ctx, key, &pm); err != nil {
		return PayrollView{}, err
	}
	if pm.Closed {
		return PayrollView{}, vErr("month", fmt.Sprintf("Mzdy za %d/%d sú uzavreté — najprv ich otvorte", month, year))
	}
	var f []invoice.FieldError
	for i, in := range inputs {
		for name, d := range map[string]money.Dec{"hours": in.Hours, "base": in.Base, "extra": in.Extra, "leave": in.Leave, "sickPay": in.SickPay, "deduction": in.Deduction} {
			if d.Sign() < 0 {
				f = append(f, invoice.FieldError{Field: fmt.Sprintf("inputs.%d.%s", i, name), Message: "Suma nemôže byť záporná"})
			}
		}
	}
	if len(f) > 0 {
		sort.Slice(f, func(i, j int) bool { return f[i].Field < f[j].Field })
		return PayrollView{}, &invoice.ValidationError{Fields: f}
	}
	pm.Inputs = inputs
	if err := a.saveJSON(ctx, key, pm); err != nil {
		return PayrollView{}, err
	}
	a.record(ctx, cid, audit.CatPayroll, "update", "payroll", int64(year*100+month), fmt.Sprintf("Mzdy %d/%d: vstupy", month, year), nil, nil)
	return a.Payroll(ctx, cid, year, month)
}

// ClosePayroll uzavrie mesiac: výsledky sa uložia a už sa neprepočítavajú.
func (a *App) ClosePayroll(ctx context.Context, cid int64, year, month int) (PayrollView, error) {
	if err := a.requireBusiness(ctx, cid, auth.PermLedgerWrite); err != nil {
		return PayrollView{}, err
	}
	v, err := a.Payroll(ctx, cid, year, month)
	if err != nil {
		return v, err
	}
	if v.Closed {
		return v, vErr("month", fmt.Sprintf("Mzdy za %d/%d sú už uzavreté", month, year))
	}
	if len(v.Rows) == 0 {
		return v, vErr("month", "V mesiaci nie je žiadny zamestnanec")
	}
	key := payrollKey(cid, year, month)
	var pm PayrollMonth
	if _, err := a.loadJSON(ctx, key, &pm); err != nil {
		return v, err
	}
	pm.Closed, pm.ClosedAt, pm.ClosedBy = true, time.Now().UTC().Truncate(time.Second), a.actorName(ctx)
	pm.Employees, pm.Results, pm.Inputs = nil, nil, nil
	for _, r := range v.Rows {
		pm.Employees = append(pm.Employees, r.Employee)
		pm.Inputs = append(pm.Inputs, r.Input)
		pm.Results = append(pm.Results, r.Result)
	}
	if err := a.saveJSON(ctx, key, pm); err != nil {
		return v, err
	}
	a.record(ctx, cid, audit.CatPayroll, "close", "payroll", int64(year*100+month),
		fmt.Sprintf("Uzavreté mzdy %d/%d: %d zamestnancov, na výplatu %s €", month, year, len(v.Rows), v.Totals.ToPay.Display()), nil, v.Totals)
	return a.Payroll(ctx, cid, year, month)
}

// ReopenPayroll otvorí uzavretý mesiac s dôvodom (výsledky sa znova prepočítajú).
func (a *App) ReopenPayroll(ctx context.Context, cid int64, year, month int, reason string) (PayrollView, error) {
	if err := a.requireBusiness(ctx, cid, auth.PermLedgerWrite); err != nil {
		return PayrollView{}, err
	}
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) < 5 {
		return PayrollView{}, vErr("reason", "Napíšte, prečo mzdy otvárate (aspoň 5 znakov)")
	}
	if err := checkPeriod(year, month); err != nil {
		return PayrollView{}, err
	}
	key := payrollKey(cid, year, month)
	var pm PayrollMonth
	if _, err := a.loadJSON(ctx, key, &pm); err != nil {
		return PayrollView{}, err
	}
	if !pm.Closed {
		return PayrollView{}, vErr("month", fmt.Sprintf("Mzdy za %d/%d nie sú uzavreté", month, year))
	}
	before := pm.Results
	pm = PayrollMonth{Inputs: pm.Inputs}
	if err := a.saveJSON(ctx, key, pm); err != nil {
		return PayrollView{}, err
	}
	a.record(ctx, cid, audit.CatPayroll, "reopen", "payroll", int64(year*100+month),
		fmt.Sprintf("Otvorené mzdy %d/%d: %s", month, year, reason), before, nil)
	return a.Payroll(ctx, cid, year, month)
}
