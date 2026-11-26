package app

import (
	"context"
	"fmt"
	"time"

	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/chart"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/ledger"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/payroll"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// Zaúčtovanie miezd v podvojnom účtovníctve: z uzavretého mesiaca jeden
// interný doklad k poslednému dňu mesiaca. Po zaúčtovaní zostane na účte
// 331 suma na výplatu, na 336 odvody poisťovniam a na 342 preddavok dane.

// PayrollPosting je návrh zaúčtovania miezd za mesiac.
type PayrollPosting struct {
	Year   int           `json:"year"`
	Month  int           `json:"month"`
	Closed bool          `json:"closed"`
	Lines  []ledger.Line `json:"lines"`
	ToPay  money.Dec     `json:"toPay"` // zostatok 331 = na výplatu
	Posted string        `json:"posted,omitempty"`
	// Current: zaúčtovaný doklad zodpovedá uzavretým mzdám (po otvorení
	// a zmene miezd treba zaúčtovať znova).
	Current bool `json:"current"`
}

// payrollLines zostaví riadky dokladu z výsledkov miezd (nulové vynechá).
func payrollLines(results []payroll.Result) []ledger.Line {
	var gross, sick, er, contrib, tax, bonus, ded money.Dec
	for _, r := range results {
		gross = gross.Add(r.Gross)
		sick = sick.Add(r.SickPay)
		er = er.Add(r.ContribER)
		contrib = contrib.Add(r.Contrib)
		tax = tax.Add(r.TaxPaid)
		bonus = bonus.Add(r.BonusPaid)
		ded = ded.Add(r.Deduction)
	}
	lines := []ledger.Line{}
	add := func(debit, credit string, v money.Dec) {
		if v.Sign() > 0 {
			lines = append(lines, ledger.Line{Account: debit, Debit: v}, ledger.Line{Account: credit, Credit: v})
		}
	}
	add("521", "331", gross)   // hrubé mzdy
	add("527", "331", sick)    // náhrada príjmu pri PN
	add("524", "336", er)      // poistné zamestnávateľa
	add("331", "336", contrib) // poistné zamestnancov
	add("331", "342", tax)     // preddavok dane
	add("342", "331", bonus)   // daňový bonus vyplatený zamestnávateľom
	add("331", "379", ded)     // zrážky zo mzdy
	return lines
}

func payrollYM(year, month int) string { return fmt.Sprintf("%04d-%02d", year, month) }

func sameLines(a, b []ledger.Line) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Account != b[i].Account || !a[i].Debit.Equal(b[i].Debit) || !a[i].Credit.Equal(b[i].Credit) {
			return false
		}
	}
	return true
}

// PayrollPostingFor navrhne zaúčtovanie miezd za mesiac.
func (a *App) PayrollPostingFor(ctx context.Context, cid int64, year, month int) (PayrollPosting, error) {
	out := PayrollPosting{Year: year, Month: month, Lines: []ledger.Line{}}
	if err := a.Require(ctx, cid, auth.PermLedgerRead); err != nil {
		return out, err
	}
	co, err := a.Store.GetCompany(ctx, cid)
	if err != nil {
		return out, err
	}
	if co.Bookkeeping != store.BookkeepingDouble {
		return out, vErr("bookkeeping", "Mzdy sa do hlavnej knihy účtujú iba v podvojnom účtovníctve")
	}
	if err := checkPeriod(year, month); err != nil {
		return out, err
	}
	var pm PayrollMonth
	if _, err := a.loadJSON(ctx, payrollKey(cid, year, month), &pm); err != nil {
		return out, err
	}
	out.Closed = pm.Closed
	if pm.Closed {
		out.Lines = payrollLines(pm.Results)
		for _, r := range pm.Results {
			out.ToPay = out.ToPay.Add(r.ToPay)
		}
	}
	d, err := a.loadLedger(ctx, cid, year)
	if err != nil {
		return out, err
	}
	ym := payrollYM(year, month)
	for _, x := range d.Internal {
		if x.Payroll == ym {
			out.Posted = x.Number
			out.Current = pm.Closed && sameLines(x.Lines, out.Lines)
		}
	}
	return out, nil
}

// PostPayroll zaúčtuje uzavreté mzdy mesiaca ako interný doklad k poslednému
// dňu mesiaca (predchádzajúci doklad miezd toho mesiaca nahradí).
func (a *App) PostPayroll(ctx context.Context, cid int64, year, month int) (InternalDoc, error) {
	pp, err := a.PayrollPostingFor(ctx, cid, year, month)
	if err != nil {
		return InternalDoc{}, err
	}
	if !pp.Closed {
		return InternalDoc{}, vErr("month", fmt.Sprintf("Mzdy za %d/%d najprv uzavrite", month, year))
	}
	if len(pp.Lines) == 0 {
		return InternalDoc{}, vErr("month", fmt.Sprintf("Mzdy za %d/%d sú nulové — nie je čo účtovať", month, year))
	}
	ym := payrollYM(year, month)
	last := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC)
	var saved InternalDoc
	err = a.editLedger(ctx, cid, year, func(d *LedgerData, c chart.Chart, _ []chart.Template) (string, error) {
		lines := append([]ledger.Line(nil), pp.Lines...)
		if err := ledger.CheckLines(lines, c); err != nil {
			return "", mfaFieldErr("lines", err.Error())
		}
		doc := InternalDoc{Date: invoice.MustDate(last.Format("2006-01-02")), Text: fmt.Sprintf("Mzdy %d/%d", month, year),
			Lines: lines, Payroll: ym}
		for i, x := range d.Internal {
			if x.Payroll == ym {
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
		saved = doc
		return fmt.Sprintf("Mzdy %d/%d zaúčtované %s, na výplatu %s €", month, year, doc.Number, pp.ToPay.Display()), nil
	})
	return saved, err
}
