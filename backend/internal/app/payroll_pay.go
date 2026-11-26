package app

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/bank"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/journal"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/party"
	"github.com/fiskus-online/fiskus/backend/internal/payroll"
)

// Príkazy na úhradu miezd: z uzavretého mesiaca hromadný príkaz SEPA
// (pain.001) s výplatami na účty zamestnancov, poistným Sociálnej
// poisťovni a zdravotným poisťovniam a preddavkami dane daňovému úradu.
// Čísla účtov a variabilné symboly inštitúcií prideľujú inštitúcie firme,
// preto ich zadáva používateľ; preddavky dane idú na účet s predčíslím
// 500259 a osobným účtom daňovníka (OÚD) vo variabilnom symbole 1100MMRRRR.

// PayrollPaySettings sú platobné údaje firmy pre odvody.
type PayrollPaySettings struct {
	SocialIBAN string `json:"socialIban"` // účet pobočky Sociálnej poisťovne
	SocialVS   string `json:"socialVs"`   // 10-miestny VS pridelený Sociálnou poisťovňou
	// Health: kód zdravotnej poisťovne → účet a VS.
	Health map[string]HealthPay `json:"health"`
	TaxOUD string               `json:"taxOud"` // osobný účet daňovníka (základné číslo účtu)
}

// HealthPay sú platobné údaje zdravotnej poisťovne.
type HealthPay struct {
	IBAN string `json:"iban"`
	VS   string `json:"vs"`
}

// PayrollPayment je jedna úhrada z mzdového mesiaca.
type PayrollPayment struct {
	Key     string    `json:"key"`  // wage:ID | social | health:KÓD | tax
	Kind    string    `json:"kind"` // wage | social | health | tax
	Name    string    `json:"name"`
	IBAN    string    `json:"iban"`
	Amount  money.Dec `json:"amount"`
	VS      string    `json:"vs"`
	SS      string    `json:"ss"`
	KS      string    `json:"ks"`
	Info    string    `json:"info"`
	Problem string    `json:"problem,omitempty"`
	// Journal: rozpis úhrady v peňažnom denníku (opatrenie MF SR, § 4 ods. 6):
	// čistá mzda, poistné zamestnanca a preddavok dane sú mzdy (e3),
	// poistné zamestnávateľa poistné a príspevky (e4).
	Journal []journal.Line `json:"-"`
}

// PayrollPayments je zoznam úhrad mesiaca.
type PayrollPayments struct {
	Year     int                `json:"year"`
	Month    int                `json:"month"`
	Closed   bool               `json:"closed"`
	Payments []PayrollPayment   `json:"payments"`
	Settings PayrollPaySettings `json:"settings"`
}

// TaxPrefix je predčíslie účtu pre preddavky na daň zo závislej činnosti.
const TaxPrefix = "500259"

func payrollPayKey(cid int64) string { return "payroll.pay." + strconv.FormatInt(cid, 10) }

func onlyDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// TaxIBAN zloží IBAN účtu správcu dane pre preddavky zo závislej činnosti.
func TaxIBAN(oud string) string {
	if oud == "" || len(oud) > 10 || !onlyDigits(oud) {
		return ""
	}
	return party.IBANFromAccount("SK", TaxPrefix+"-"+oud+"/8180")
}

func (a *App) payrollPaySettings(ctx context.Context, cid int64) (PayrollPaySettings, error) {
	var s PayrollPaySettings
	_, err := a.loadJSON(ctx, payrollPayKey(cid), &s)
	if s.Health == nil {
		s.Health = map[string]HealthPay{}
	}
	return s, err
}

// SavePayrollPaySettings uloží platobné údaje pre odvody.
func (a *App) SavePayrollPaySettings(ctx context.Context, cid int64, s PayrollPaySettings) (PayrollPaySettings, error) {
	if err := a.requireBusiness(ctx, cid, auth.PermLedgerWrite); err != nil {
		return s, err
	}
	var f []invoice.FieldError
	add := func(field, msg string) { f = append(f, invoice.FieldError{Field: field, Message: msg}) }
	iban := func(field, v string) string {
		v = party.NormalizeID(v)
		if v != "" && !party.ValidIBAN(v) {
			add(field, "Neplatný IBAN")
		}
		return v
	}
	vs := func(field, v string) string {
		v = strings.TrimSpace(v)
		if v != "" && (len(v) > 10 || !onlyDigits(v)) {
			add(field, "Variabilný symbol má najviac 10 číslic")
		}
		return v
	}
	s.SocialIBAN = iban("socialIban", s.SocialIBAN)
	s.SocialVS = vs("socialVs", s.SocialVS)
	if s.SocialVS != "" && len(s.SocialVS) != 10 {
		add("socialVs", "Sociálna poisťovňa prideľuje 10-miestny variabilný symbol")
	}
	health := map[string]HealthPay{}
	for code, h := range s.Health {
		if _, ok := HealthInsurers[code]; !ok {
			continue
		}
		h.IBAN = iban("health."+code+".iban", h.IBAN)
		h.VS = vs("health."+code+".vs", h.VS)
		if h.IBAN != "" || h.VS != "" {
			health[code] = h
		}
	}
	s.Health = health
	s.TaxOUD = strings.TrimLeft(strings.TrimSpace(s.TaxOUD), "0")
	if s.TaxOUD != "" && TaxIBAN(s.TaxOUD) == "" {
		add("taxOud", "Osobný účet daňovníka má najviac 10 číslic")
	}
	if len(f) > 0 {
		sort.Slice(f, func(i, j int) bool { return f[i].Field < f[j].Field })
		return s, &invoice.ValidationError{Fields: f}
	}
	if err := a.saveJSON(ctx, payrollPayKey(cid), s); err != nil {
		return s, err
	}
	a.record(ctx, cid, audit.CatPayroll, "update", "payroll_pay", 0, "Platobné údaje pre odvody", nil, nil)
	return s, nil
}

// payrollPayList zostaví úhrady z výsledkov uzavretého mesiaca.
func payrollPayList(year, month int, rows []PayrollRow, ibans map[int64]string, s PayrollPaySettings) []PayrollPayment {
	period := fmt.Sprintf("%d/%d", month, year)
	out := []PayrollPayment{}
	problem := func(p *PayrollPayment, missing string) {
		switch {
		case p.Amount.Sign() <= 0:
			p.Problem = "Nie je čo uhradiť"
		case p.IBAN == "":
			p.Problem = missing
		case !party.ValidIBAN(p.IBAN):
			p.Problem = "Neplatný IBAN"
		}
	}
	results := make([]payroll.Result, 0, len(rows))
	insurers := map[int64]string{}
	socialEmp, healthEmp := money.Zero, map[string]money.Dec{}
	for _, r := range rows {
		socialEmp = socialEmp.Add(r.Result.SocialEmployee)
		healthEmp[r.Employee.Insurer] = healthEmp[r.Employee.Insurer].Add(r.Result.HealthEmployee)
		results = append(results, r.Result)
		insurers[r.Employee.ID] = r.Employee.Insurer
		if r.Result.ToPay.Sign() <= 0 {
			continue
		}
		iban := ibans[r.Employee.ID]
		if iban == "" {
			iban = r.Employee.IBAN
		}
		p := PayrollPayment{Key: "wage:" + strconv.FormatInt(r.Employee.ID, 10), Kind: "wage", Name: r.Employee.Name(),
			IBAN: formatIBAN(iban), Amount: r.Result.ToPay, Info: "Mzda " + period}
		p.Journal = splitLines(p.Amount, money.Zero)
		problem(&p, "Chýba IBAN zamestnanca — doplňte ho alebo vyplaťte v hotovosti")
		out = append(out, p)
	}
	t := payroll.Sum(results, insurers)
	mm := fmt.Sprintf("%02d", month)
	if t.Social.Sign() > 0 {
		p := PayrollPayment{Key: "social", Kind: "social", Name: "Sociálna poisťovňa", IBAN: formatIBAN(s.SocialIBAN), Amount: t.Social,
			VS: s.SocialVS, SS: fmt.Sprintf("%s99%d", mm, year), KS: "3118", Info: "Poistne " + period}
		p.Journal = splitLines(socialEmp, t.Social.Sub(socialEmp))
		problem(&p, "Doplňte účet pobočky Sociálnej poisťovne v platobných údajoch")
		if p.Problem == "" && p.VS == "" {
			p.Problem = "Doplňte variabilný symbol pridelený Sociálnou poisťovňou"
		}
		out = append(out, p)
	}
	codes := make([]string, 0, len(t.Health))
	for c := range t.Health {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	for _, c := range codes {
		h := s.Health[c]
		name := HealthInsurers[c]
		if name == "" {
			name = "Zdravotná poisťovňa " + c
		}
		p := PayrollPayment{Key: "health:" + c, Kind: "health", Name: name, IBAN: formatIBAN(h.IBAN), Amount: t.Health[c],
			VS: h.VS, Info: "Preddavky ZP " + period}
		p.Journal = splitLines(healthEmp[c], t.Health[c].Sub(healthEmp[c]))
		problem(&p, "Doplňte účet poisťovne "+name+" v platobných údajoch")
		if p.Problem == "" && p.VS == "" {
			p.Problem = "Doplňte variabilný symbol zamestnávateľa v poisťovni " + name
		}
		out = append(out, p)
	}
	if t.Tax.Sign() != 0 {
		p := PayrollPayment{Key: "tax", Kind: "tax", Name: "Daňový úrad", IBAN: formatIBAN(TaxIBAN(s.TaxOUD)), Amount: t.Tax,
			VS: fmt.Sprintf("1100%s%d", mm, year), Info: "Preddavky dane " + period}
		if t.Tax.Sign() < 0 {
			p.Problem = "Vyplatený daňový bonus je vyšší ako preddavky — nie je čo odviesť"
		} else {
			p.Journal = splitLines(p.Amount, money.Zero)
			problem(&p, "Doplňte osobný účet daňovníka (OÚD) v platobných údajoch")
		}
		out = append(out, p)
	}
	return out
}

// splitLines: časť do miezd (e3), časť do poistného zamestnávateľa (e4).
func splitLines(wages, insur money.Dec) []journal.Line {
	var out []journal.Line
	if wages.Sign() != 0 {
		out = append(out, journal.Line{Column: journal.ExpWages, Amount: wages})
	}
	if insur.Sign() != 0 {
		out = append(out, journal.Line{Column: journal.ExpInsur, Amount: insur})
	}
	return out
}

// matchPayroll nájde úhradu miezd k odchádzajúcej platbe z banky: rovnaký
// účet príjemcu, suma a (ak sú oba vyplnené) variabilný symbol.
func matchPayroll(pays []PayrollPayment, iban, vs string, amount money.Dec) *PayrollPayment {
	iban = party.NormalizeID(iban)
	if iban == "" {
		return nil
	}
	for i, p := range pays {
		if p.Problem != "" || party.NormalizeID(p.IBAN) != iban || !p.Amount.Equal(amount) {
			continue
		}
		if p.VS != "" && vs != "" && strings.TrimLeft(p.VS, "0") != strings.TrimLeft(vs, "0") {
			continue
		}
		return &pays[i]
	}
	return nil
}

// matchCashWage nájde výplatu zamestnanca v hotovosti (meno a suma).
func matchCashWage(pays []PayrollPayment, partner string, amount money.Dec) *PayrollPayment {
	name := strings.Join(strings.Fields(strings.ToLower(partner)), " ")
	if name == "" {
		return nil
	}
	for i, p := range pays {
		if p.Kind != "wage" || !p.Amount.Equal(amount) {
			continue
		}
		n := strings.ToLower(p.Name)
		f := strings.Fields(n)
		rev := ""
		if len(f) == 2 {
			rev = f[1] + " " + f[0]
		}
		if name == n || name == rev {
			return &pays[i]
		}
	}
	return nil
}

// PayrollPaymentsFor vráti úhrady uzavretého mesiaca a platobné údaje.
func (a *App) PayrollPaymentsFor(ctx context.Context, cid int64, year, month int) (PayrollPayments, error) {
	out := PayrollPayments{Year: year, Month: month, Payments: []PayrollPayment{}}
	v, err := a.Payroll(ctx, cid, year, month)
	if err != nil {
		return out, err
	}
	if out.Settings, err = a.payrollPaySettings(ctx, cid); err != nil {
		return out, err
	}
	out.Closed = v.Closed
	if !v.Closed {
		return out, nil
	}
	b, err := a.employeeBook(ctx, cid)
	if err != nil {
		return out, err
	}
	ibans := map[int64]string{} // aktuálny IBAN (mohol pribudnúť po uzavretí)
	for _, e := range b.List {
		ibans[e.ID] = e.IBAN
	}
	out.Payments = payrollPayList(year, month, v.Rows, ibans, out.Settings)
	return out, nil
}

// PayrollPaymentOrder zostaví hromadný príkaz (pain.001) na vybrané úhrady.
func (a *App) PayrollPaymentOrder(ctx context.Context, cid int64, year, month int, keys []string, on invoice.Date) (string, []byte, error) {
	if err := a.requireBusiness(ctx, cid, auth.PermLedgerWrite); err != nil {
		return "", nil, err
	}
	if len(keys) == 0 {
		return "", nil, &PaymentOrderError{"Vyberte aspoň jednu úhradu."}
	}
	company, err := a.Store.GetCompany(ctx, cid)
	if err != nil {
		return "", nil, err
	}
	if !party.ValidIBAN(company.IBAN) {
		return "", nil, &PaymentOrderError{"Doplňte platný IBAN firmy v nastaveniach — z neho sa platí."}
	}
	today := a.Today()
	if on.IsZero() {
		on = today
	}
	if on.Before(today) {
		return "", nil, &PaymentOrderError{"Dátum splatnosti príkazu nemôže byť v minulosti."}
	}
	pp, err := a.PayrollPaymentsFor(ctx, cid, year, month)
	if err != nil {
		return "", nil, err
	}
	if !pp.Closed {
		return "", nil, &PaymentOrderError{fmt.Sprintf("Mzdy za %d/%d najprv uzavrite.", month, year)}
	}
	byKey := map[string]PayrollPayment{}
	for _, p := range pp.Payments {
		byKey[p.Key] = p
	}
	now := a.now()
	order := bank.PaymentOrder{
		MsgID:   fmt.Sprintf("FISKUS-%d-M%s", cid, now.Format("20060102150405")),
		Created: now, Debtor: company.Name, DebtorIBAN: company.IBAN, Date: on,
	}
	seen := map[string]bool{}
	var names []string
	for _, k := range keys {
		if seen[k] {
			continue
		}
		seen[k] = true
		p, ok := byKey[k]
		if !ok {
			return "", nil, &PaymentOrderError{"Úhrada " + k + " v mzdách mesiaca nie je."}
		}
		if p.Problem != "" {
			return "", nil, &PaymentOrderError{p.Name + ": " + p.Problem}
		}
		order.Transfers = append(order.Transfers, bank.Transfer{Name: p.Name, IBAN: p.IBAN, Amount: p.Amount, VS: p.VS, SS: p.SS, KS: p.KS, Info: p.Info})
		names = append(names, p.Name)
	}
	data, err := order.XML()
	if err != nil {
		return "", nil, &PaymentOrderError{err.Error()}
	}
	a.record(ctx, cid, audit.CatPayroll, "payment_order", "payroll", int64(year*100+month),
		fmt.Sprintf("Príkaz na úhradu miezd %d/%d: %s (%d úhrad, %s €, splatnosť %s)", month, year, strings.Join(names, ", "),
			len(order.Transfers), order.Total().Display(), on.Display()), nil, map[string]any{"keys": keys, "date": on})
	return fmt.Sprintf("mzdy-%04d-%02d-prikaz.xml", year, month), data, nil
}
