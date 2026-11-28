package app

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/dpfo"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/payroll"
)

// Ročné zúčtovanie preddavkov na daň (§ 38 zákona o dani z príjmov) zo
// súčtu uzavretých mesiacov roka. Vykoná ho zamestnávateľ na písomnú
// žiadosť zamestnanca do 31. marca nasledujúceho roka; zamestnanec k nej
// pripojí doklady (manžel/ka, III. pilier, deti) a potvrdenia od iných
// zamestnávateľov, ktorých príjmy Fiškus nepozná.

// SettlementRequest je žiadosť zamestnanca a údaje, ktoré k nej doložil.
type SettlementRequest struct {
	Requested bool         `json:"requested"`
	DDS       money.Dec    `json:"dds"`
	Spouse    *dpfo.Spouse `json:"spouse,omitempty"`
	// príjmy od iných zamestnávateľov v roku (z ich potvrdení)
	OtherIncome  money.Dec `json:"otherIncome"`
	OtherSocial  money.Dec `json:"otherSocial"`
	OtherHealth  money.Dec `json:"otherHealth"`
	OtherPrepaid money.Dec `json:"otherPrepaid"`
	OtherBonus   money.Dec `json:"otherBonus"`
	Date         string    `json:"date,omitempty"` // dátum vykonania RRRR-MM-DD
	// SettleMonth: mzda (RRRR-MM, január až apríl nasledujúceho roka),
	// v ktorej sa preplatok vráti alebo nedoplatok zrazí; prázdne = ručne
	SettleMonth string `json:"settleMonth,omitempty"`
}

// SettlementRow je ročné zúčtovanie jedného zamestnanca.
type SettlementRow struct {
	Employee  payroll.Employee  `json:"employee"`
	Months    []int             `json:"months"` // uzavreté mesiace s príjmom
	Request   SettlementRequest `json:"request"`
	Rows      map[string]string `json:"rows"` // r. 01 … 22 (sumy ako text)
	Result    money.Dec         `json:"result"`
	Cert      Certificate       `json:"certificate"` // potvrdenie o zdaniteľných príjmoch
	TaxDiff   money.Dec         `json:"taxDiff"`     // r. 06 − r. 18: + nedoplatok, − preplatok
	BonusDiff money.Dec         `json:"bonusDiff"`   // r. 09 − r. 10: + doplatiť, − vybrať
	Notes     []string          `json:"notes"`
}

// SettlementView je ročné zúčtovanie za rok.
type SettlementView struct {
	Year     int             `json:"year"`
	Open     []int           `json:"open"` // mesiace roka, ktoré nie sú uzavreté
	Deadline string          `json:"deadline"`
	List     []SettlementRow `json:"list"`
}

func settlementKey(cid int64, year int) string {
	return fmt.Sprintf("payroll.rz.%d.%d", cid, year)
}

func settlementRows(s dpfo.Settlement) map[string]string {
	r := map[string]money.Dec{"01": s.R01, "01a": s.R01a, "02": s.R02, "02a": s.R02a, "02b": s.R02b, "03": s.R03,
		"04a": s.R04a, "04b": s.R04b, "04c": s.R04c, "04": s.R04, "05": s.R05, "06": s.R06, "09": s.R09, "10": s.R10,
		"11": s.R11, "12": s.R12, "13": s.R13, "14": s.R14, "18": s.R18, "19": s.R19, "20": s.R20, "21": s.R21, "22": s.R22}
	out := make(map[string]string, len(r))
	for k, v := range r {
		out[k] = v.RoundCents().String()
	}
	return out
}

// Settlements vráti ročné zúčtovanie všetkých zamestnancov s príjmom v roku.
func (a *App) Settlements(ctx context.Context, cid int64, year int) (SettlementView, error) {
	out := SettlementView{Year: year, List: []SettlementRow{}, Open: []int{},
		Deadline: fmt.Sprintf("%d-03-31", year+1)}
	if err := a.requireBusiness(ctx, cid, auth.PermLedgerRead); err != nil {
		return out, err
	}
	p, err := dpfo.ParamsFor(year)
	if err != nil {
		return out, vErr("year", err.Error())
	}
	reqs := map[string]SettlementRequest{}
	if _, err := a.loadJSON(ctx, settlementKey(cid, year), &reqs); err != nil {
		return out, err
	}
	b, err := a.employeeBook(ctx, cid)
	if err != nil {
		return out, err
	}
	current := map[int64]payroll.Employee{}
	for _, e := range b.List {
		current[e.ID] = e
	}
	type acc struct {
		e                                          payroll.Employee
		months                                     []int
		income, agree, social, health, prep, bonus money.Dec
		cert                                       certAcc
	}
	sums := map[int64]*acc{}
	for m := 1; m <= 12; m++ {
		v, err := a.Payroll(ctx, cid, year, m)
		if err != nil {
			return out, err
		}
		if !v.Closed {
			out.Open = append(out.Open, m)
			continue
		}
		for _, r := range v.Rows {
			x := sums[r.Employee.ID]
			if x == nil {
				x = &acc{e: r.Employee}
				sums[r.Employee.ID] = x
			}
			if e, ok := current[r.Employee.ID]; ok {
				x.e = e // deti a údaje z aktuálnej karty
			}
			res := r.Result
			x.cert.add(m, r.Employee, res, p)
			x.months = append(x.months, m)
			x.income = x.income.Add(res.Income)
			if r.Employee.Contract.Agreement() {
				x.agree = x.agree.Add(res.Income)
			}
			x.social, x.health = x.social.Add(res.SocialEmployee), x.health.Add(res.HealthEmployee)
			x.prep, x.bonus = x.prep.Add(res.Tax), x.bonus.Add(res.TaxBonus)
		}
	}
	for id, x := range sums {
		req := reqs[strconv.FormatInt(id, 10)]
		in := dpfo.SettlementInput{
			Income: x.income.Add(req.OtherIncome), Agreement: x.agree,
			Social: x.social.Add(req.OtherSocial), Health: x.health.Add(req.OtherHealth),
			DDS: req.DDS, Spouse: req.Spouse,
			BonusPaid: x.bonus.Add(req.OtherBonus), Prepaid: x.prep.Add(req.OtherPrepaid),
		}
		if x.e.Bonus {
			in.Children = x.e.Children
		}
		s := dpfo.Settle(in, p)
		row := SettlementRow{Employee: x.e, Months: x.months, Request: req, Rows: settlementRows(s), Result: s.R22,
			Cert: x.cert.certificate(), TaxDiff: s.R06.Sub(s.R18), BonusDiff: s.R09.Sub(s.R10), Notes: s.Notes}
		if row.Notes == nil {
			row.Notes = []string{}
		}
		if len(out.Open) > 0 {
			row.Notes = append(row.Notes, "Niektoré mesiace roka nie sú uzavreté — výsledok je predbežný.")
		}
		out.List = append(out.List, row)
	}
	sort.Slice(out.List, func(i, j int) bool { return out.List[i].Employee.Name() < out.List[j].Employee.Name() })
	return out, nil
}

// SaveSettlementRequest uloží žiadosť zamestnanca o ročné zúčtovanie.
func (a *App) SaveSettlementRequest(ctx context.Context, cid int64, year int, empID int64, r SettlementRequest) error {
	if err := a.requireBusiness(ctx, cid, auth.PermLedgerWrite); err != nil {
		return err
	}
	if _, err := dpfo.ParamsFor(year); err != nil {
		return vErr("year", err.Error())
	}
	b, err := a.employeeBook(ctx, cid)
	if err != nil {
		return err
	}
	var name string
	for _, e := range b.List {
		if e.ID == empID {
			name = e.Name()
		}
	}
	if name == "" {
		return vErr("employee", "Zamestnanec neexistuje")
	}
	for f, v := range map[string]money.Dec{"dds": r.DDS, "otherIncome": r.OtherIncome, "otherSocial": r.OtherSocial,
		"otherHealth": r.OtherHealth, "otherPrepaid": r.OtherPrepaid, "otherBonus": r.OtherBonus} {
		if v.Sign() < 0 {
			return vErr(f, "Suma nemôže byť záporná")
		}
	}
	if r.Spouse != nil {
		if r.Spouse.Months <= 0 {
			r.Spouse = nil
		} else if r.Spouse.Months > 12 {
			return vErr("spouse.months", "Najviac 12 mesiacov")
		}
	}
	if r.SettleMonth != "" {
		y, mo, ok := parseYM(r.SettleMonth)
		if !ok || y != year+1 || mo > 4 {
			return vErr("settleMonth", fmt.Sprintf("Vyrovnanie vo mzde za január až apríl %d", year+1))
		}
		if !r.Requested {
			return vErr("settleMonth", "Ročné zúčtovanie sa vykoná iba na žiadosť zamestnanca")
		}
		var pm PayrollMonth
		if _, err := a.loadJSON(ctx, payrollKey(cid, y, mo), &pm); err != nil {
			return err
		}
		if pm.Closed {
			return vErr("settleMonth", fmt.Sprintf("Mzdy za %d/%d sú uzavreté — otvorte ich alebo zvoľte iný mesiac", mo, y))
		}
	}
	if r.Date != "" {
		if _, err := invoice.ParseDate(r.Date); err != nil {
			return vErr("date", "Neplatný dátum")
		}
	}
	reqs := map[string]SettlementRequest{}
	if _, err := a.loadJSON(ctx, settlementKey(cid, year), &reqs); err != nil {
		return err
	}
	reqs[strconv.FormatInt(empID, 10)] = r
	if err := a.saveJSON(ctx, settlementKey(cid, year), reqs); err != nil {
		return err
	}
	a.record(ctx, cid, audit.CatPayroll, "update", "settlement", empID,
		fmt.Sprintf("Ročné zúčtovanie %d: %s", year, name), nil, nil)
	return nil
}

func parseYM(s string) (int, int, bool) {
	var y, m int
	if n, err := fmt.Sscanf(s, "%d-%d", &y, &m); err != nil || n != 2 || m < 1 || m > 12 || len(s) != 7 {
		return 0, 0, false
	}
	return y, m, true
}

// settlementInputs: rozdiely ročného zúčtovania za predchádzajúci rok, ktoré
// sa vyrovnajú v mzde year/month (zamestnanec → daň, bonus).
func (a *App) settlementInputs(ctx context.Context, cid int64, year, month int) (map[int64][2]money.Dec, error) {
	reqs := map[string]SettlementRequest{}
	if _, err := a.loadJSON(ctx, settlementKey(cid, year-1), &reqs); err != nil {
		return nil, err
	}
	ym := fmt.Sprintf("%04d-%02d", year, month)
	any := false
	for _, r := range reqs {
		if r.Requested && r.SettleMonth == ym {
			any = true
		}
	}
	if !any {
		return nil, nil
	}
	v, err := a.Settlements(ctx, cid, year-1)
	if err != nil {
		return nil, err
	}
	out := map[int64][2]money.Dec{}
	for _, r := range v.List {
		if r.Request.Requested && r.Request.SettleMonth == ym {
			out[r.Employee.ID] = [2]money.Dec{r.TaxDiff, r.BonusDiff}
		}
	}
	return out, nil
}
