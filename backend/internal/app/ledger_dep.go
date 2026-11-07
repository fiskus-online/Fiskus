package app

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/fiskus-online/fiskus/backend/internal/asset"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/chart"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/ledger"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// Účtovné odpisy v podvojnom účtovníctve: z registra majetku za rok jeden
// interný doklad k 31. 12. — odpisy 551 / 08x (07x), pri vyradení
// zostatková cena 541 (predaj) alebo 551 / 08x a vyradenie 08x / 02x (01x)
// v obstarávacej cene.

// DepItem je odpis jednej položky majetku v roku.
type DepItem struct {
	AssetID    int64     `json:"assetId"`
	Number     string    `json:"number"`
	Name       string    `json:"name"`
	Account    string    `json:"account"`
	DepAccount string    `json:"depAccount"`
	Amount     money.Dec `json:"amount"`   // účtovný odpis
	Tax        money.Dec `json:"tax"`      // daňový odpis (pred krátením podielom)
	Diff       money.Dec `json:"diff"`     // účtovný − daňový
	Disposal   money.Dec `json:"disposal"` // zostatková cena pri vyradení
	Kind       string    `json:"disposalKind,omitempty"`
	Cost       money.Dec `json:"cost"`
}

// DepPosting je návrh zaúčtovania účtovných odpisov za rok.
type DepPosting struct {
	Year   int           `json:"year"`
	Items  []DepItem     `json:"items"`
	Lines  []ledger.Line `json:"lines"`
	Total  money.Dec     `json:"total"` // odpisy spolu
	Diff   money.Dec     `json:"diff"`  // rozdiel oproti daňovým odpisom
	Posted string        `json:"posted,omitempty"`
}

// ErrNoDepreciation — v roku nie sú účtovné odpisy ani vyradenie.
var ErrNoDepreciation = errors.New("v roku nie sú žiadne účtovné odpisy ani vyradený majetok")

// AccDepreciationFor navrhne zaúčtovanie účtovných odpisov za rok.
func (a *App) AccDepreciationFor(ctx context.Context, companyID int64, year int) (DepPosting, error) {
	out := DepPosting{Year: year, Items: []DepItem{}, Lines: []ledger.Line{}}
	if err := a.Require(ctx, companyID, auth.PermLedgerRead); err != nil {
		return out, err
	}
	co, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return out, err
	}
	if co.Bookkeeping != store.BookkeepingDouble {
		return out, mfaFieldErr("bookkeeping", "Účtovné odpisy sa účtujú iba v podvojnom účtovníctve")
	}
	if year < 2001 || year > 2100 {
		return out, mfaFieldErr("year", "Neplatný rok")
	}
	list, err := a.Store.ListAssets(ctx, companyID)
	if err != nil {
		return out, err
	}
	type key struct{ debit, credit string }
	sums := map[key]money.Dec{}
	addLine := func(debit, credit string, v money.Dec) {
		if v.Sign() > 0 {
			sums[key{debit, credit}] = sums[key{debit, credit}].Add(v)
		}
	}
	for i := range list {
		x := &list[i]
		y := x.AccInYear(year)
		if y.Amount.IsZero() && y.Disposal.IsZero() && (x.Disposed.IsZero() || x.Disposed.Year() != year) {
			continue
		}
		acc := x.AccountOrDefault()
		dep := asset.DepAccount(acc)
		it := DepItem{AssetID: x.ID, Number: x.Number, Name: x.Name, Account: acc, DepAccount: dep,
			Amount: y.Amount, Tax: y.Tax, Diff: y.Diff, Disposal: y.Disposal, Cost: x.Cost}
		addLine("551", dep, y.Amount)
		if !x.Disposed.IsZero() && x.Disposed.Year() == year {
			it.Kind = string(x.DisposalKind)
			zcAcc := "551" // likvidácia, škoda, vklad: zostatková cena do nákladov
			if x.DisposalKind == asset.DisposalSale {
				zcAcc = "541" // zostatková cena predaného majetku
			}
			addLine(zcAcc, dep, y.Disposal)
			addLine(dep, acc, x.Cost) // vyradenie: oprávky proti obstarávacej cene
		}
		out.Items = append(out.Items, it)
		out.Total = out.Total.Add(y.Amount)
		out.Diff = out.Diff.Add(y.Diff)
	}
	keys := make([]key, 0, len(sums))
	for k := range sums {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].debit != keys[j].debit {
			return keys[i].debit > keys[j].debit // najprv náklady (5xx), potom vyradenie (0xx)
		}
		return keys[i].credit < keys[j].credit
	})
	for _, k := range keys {
		v := sums[k]
		out.Lines = append(out.Lines, ledger.Line{Account: k.debit, Debit: v}, ledger.Line{Account: k.credit, Credit: v})
	}
	d, err := a.loadLedger(ctx, companyID, year)
	if err != nil {
		return out, err
	}
	for _, x := range d.Internal {
		if x.Dep {
			out.Posted = x.Number
		}
	}
	return out, nil
}

// PostAccDepreciation zaúčtuje účtovné odpisy roka ako interný doklad k 31. 12.
// (predchádzajúci doklad s odpismi toho roka nahradí).
func (a *App) PostAccDepreciation(ctx context.Context, companyID int64, year int) (InternalDoc, error) {
	dp, err := a.AccDepreciationFor(ctx, companyID, year)
	if err != nil {
		return InternalDoc{}, err
	}
	if len(dp.Lines) == 0 {
		return InternalDoc{}, mfaFieldErr("items", ErrNoDepreciation.Error())
	}
	var saved InternalDoc
	err = a.editLedger(ctx, companyID, year, func(d *LedgerData, c chart.Chart, _ []chart.Template) (string, error) {
		lines := append([]ledger.Line(nil), dp.Lines...)
		if err := ledger.CheckLines(lines, c); err != nil {
			return "", mfaFieldErr("lines", err.Error())
		}
		doc := InternalDoc{Date: invoice.MustDate(fmt.Sprintf("%d-12-31", year)), Text: fmt.Sprintf("Účtovné odpisy majetku %d", year),
			Amount: dp.Total, Lines: lines, Dep: true}
		for i, x := range d.Internal {
			if x.Dep {
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
		return fmt.Sprintf("Účtovné odpisy %d (%d položiek majetku) %s", year, len(dp.Items), doc.Number), nil
	})
	return saved, err
}
