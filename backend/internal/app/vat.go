package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/purchase"
	"github.com/fiskus-online/fiskus/backend/internal/store"
	"github.com/fiskus-online/fiskus/backend/internal/vatreturn"
)

// Chyby priznania k DPH.
var (
	ErrNotVATPayer    = errors.New("firma nie je platiteľom DPH — priznanie k DPH sa nepodáva")
	ErrPeriodTooOld   = errors.New("obdobia pred júlom 2025 sa podávajú na staršom tlačive, ktoré Fiškus nepodporuje")
	ErrPeriodInFuture = errors.New("obdobie ešte nezačalo")
	ErrMissingVATData = errors.New("v nastaveniach firmy doplňte IČ DPH a DIČ")
)

// VATReturn zostaví riadne priznanie k DPH a kontrolný výkaz za obdobie.
func (a *App) VATReturn(ctx context.Context, companyID int64, period string, excess money.Dec) (vatreturn.Return, store.Company, error) {
	return a.VATReturnWith(ctx, companyID, period, excess, vatreturn.Options{})
}

// VATReturnWith zostaví priznanie zvoleného druhu (riadne, opravné, dodatočné).
func (a *App) VATReturnWith(ctx context.Context, companyID int64, period string, excess money.Dec, o vatreturn.Options) (vatreturn.Return, store.Company, error) {
	if err := a.Require(ctx, companyID, auth.PermVATRead); err != nil {
		return vatreturn.Return{}, store.Company{}, err
	}
	c, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return vatreturn.Return{}, c, err
	}
	if !c.VATPayer {
		return vatreturn.Return{}, c, ErrNotVATPayer
	}
	p, err := vatreturn.ParsePeriod(period)
	if err != nil {
		return vatreturn.Return{}, c, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "period", Message: err.Error()}}}
	}
	if !p.Supported() {
		return vatreturn.Return{}, c, ErrPeriodTooOld
	}
	if a.Today().Before(p.Start()) {
		return vatreturn.Return{}, c, ErrPeriodInFuture
	}
	// faktúra dodaná v decembri môže byť vystavená v januári → aj nasledujúci rok
	var invs []invoice.Invoice
	var purs []purchase.Purchase
	for _, y := range []int{p.Year, p.Year + 1} {
		list, err := a.Store.ListInvoices(ctx, companyID, store.InvoiceFilter{Year: y})
		if err != nil {
			return vatreturn.Return{}, c, err
		}
		invs = append(invs, list...)
		pl, err := a.Store.ListPurchases(ctx, companyID, store.PurchaseFilter{Year: y})
		if err != nil {
			return vatreturn.Return{}, c, err
		}
		purs = append(purs, pl...)
	}
	if excess.Sign() < 0 {
		excess = money.Zero
	}
	cashDocs, err := a.Store.ListCashDocuments(ctx, companyID, p.Year)
	if err != nil {
		return vatreturn.Return{}, c, err
	}
	coef, err := a.coefFor(ctx, c, p)
	if err != nil {
		return vatreturn.Return{}, c, err
	}
	var adj []vatreturn.DeductionAdjustment
	if p.End().Month() == 12 {
		list, err := a.VehicleVATAdjustments(ctx, companyID, p.Year)
		if err != nil {
			return vatreturn.Return{}, c, err
		}
		for _, x := range list {
			adj = append(adj, vatreturn.DeductionAdjustment{Label: x.Vehicle, Rate: x.Rate, Tax: x.Tax, Reason: x.Reason})
		}
	}
	r := vatreturn.Compute(vatreturn.Input{Period: p, Invoices: invs, Purchases: purs, CashDocs: cashDocs, ExcessCarried: excess, Coef: coef, Adjustments: adj})
	if err := r.Apply(o); err != nil {
		return r, c, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "kind", Message: err.Error()}}}
	}
	if r.Kind == vatreturn.KindSupplementary {
		if err := a.supplementKV(ctx, companyID, &r); err != nil {
			return r, c, err
		}
	}
	return r, c, nil
}

// supplementKV pripojí k dodatočnému priznaniu dodatočný KV: rozdiel medzi
// naposledy podaným KV (zo záznamu podania alebo načítaný z XML — novší
// z nich) a KV z aktuálnych dokladov.
func (a *App) supplementKV(ctx context.Context, companyID int64, r *vatreturn.Return) error {
	filed, from, err := a.filedKV(ctx, companyID, r.Period)
	if err != nil {
		return err
	}
	if filed == nil {
		r.KVFixNote = vatreturn.ErrSupplementaryK.Error()
		return nil
	}
	r.KVBase = from
	fix, err := vatreturn.SupplementKV(*filed, r.KV)
	if err != nil {
		r.KVFixNote = err.Error()
		return nil
	}
	// názvy partnerov iba na zobrazenie (XML ich nemá)
	names := map[string]string{}
	for _, l := range append(append(append([]vatreturn.KVLine{}, r.KV.A1...), r.KV.B1...), r.KV.B2...) {
		names[l.Partner] = l.Name
	}
	for _, list := range [][]vatreturn.KVLine{fix.A1, fix.B1, fix.B2} {
		for i := range list {
			if list[i].Name == "" {
				list[i].Name = names[list[i].Partner]
			}
		}
	}
	r.KVFix = &fix
	return nil
}

// LoadedKV je kontrolný výkaz načítaný zo súboru XML z eDane — podaný stav
// pre dodatočný KV pri obdobiach, ktorých podanie Fiškus nezaznamenal.
type LoadedKV struct {
	KV     vatreturn.KV `json:"kv"`
	Druh   string       `json:"druh"` // R, O, D (posledný načítaný súbor)
	Files  int          `json:"files"`
	User   string       `json:"user"`
	Loaded time.Time    `json:"loaded"`
}

func loadedKVKey(companyID int64, period string) string {
	return fmt.Sprintf("vat.kvloaded.%d.%s", companyID, period)
}

func (a *App) loadedKV(ctx context.Context, companyID int64, p vatreturn.Period) (*LoadedKV, error) {
	raw, err := a.Store.GetSetting(ctx, loadedKVKey(companyID, p.String()))
	if errors.Is(err, store.ErrNotFound) || (err == nil && raw == "") {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var l LoadedKV
	return &l, json.Unmarshal([]byte(raw), &l)
}

// filedKV vráti posledný podaný stav KV za obdobie a odkiaľ je.
func (a *App) filedKV(ctx context.Context, companyID int64, p vatreturn.Period) (*vatreturn.KV, string, error) {
	list, err := a.VATFilings(ctx, companyID, p.String())
	if err != nil {
		return nil, "", err
	}
	var rec *VATFiling
	for i := len(list) - 1; i >= 0 && rec == nil; i-- {
		if list[i].KV != nil {
			rec = &list[i]
		}
	}
	l, err := a.loadedKV(ctx, companyID, p)
	if err != nil {
		return nil, "", err
	}
	switch {
	case l != nil && (rec == nil || l.Loaded.After(rec.Recorded)):
		return &l.KV, "súbor XML načítaný " + l.Loaded.In(time.Local).Format("2. 1. 2006"), nil
	case rec != nil:
		return rec.KV, "záznam podania " + rec.Filed.Display(), nil
	}
	return nil, "", nil
}

// LoadFiledKV načíta podaný kontrolný výkaz zo súboru XML z eDane. Riadny
// a opravný nahradí podaný stav, dodatočný sa použije na posledný podaný
// stav. Slúži ako základ pre dodatočný KV.
func (a *App) LoadFiledKV(ctx context.Context, companyID int64, period string, data []byte) (LoadedKV, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return LoadedKV{}, err
	}
	c, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return LoadedKV{}, err
	}
	p, err := vatreturn.ParsePeriod(period)
	if err != nil {
		return LoadedKV{}, vErr("period", err.Error())
	}
	f, err := vatreturn.ParseKVXML(data)
	if err != nil {
		return LoadedKV{}, vErr("file", err.Error())
	}
	if want := strings.ToUpper(strings.ReplaceAll(c.ICDPH, " ", "")); f.ICDPH != want {
		return LoadedKV{}, vErr("file", "Výkaz je pre IČ DPH "+f.ICDPH+", firma má "+want)
	}
	if f.Period != p {
		return LoadedKV{}, vErr("file", "Výkaz je za obdobie "+f.Period.Label()+", nie za "+p.Label())
	}
	kv := f.KV
	prev, err := a.loadedKV(ctx, companyID, p)
	if err != nil {
		return LoadedKV{}, err
	}
	files := 1
	if f.Druh == "D" {
		base, _, err := a.filedKV(ctx, companyID, p)
		if err != nil {
			return LoadedKV{}, err
		}
		if base == nil {
			return LoadedKV{}, vErr("file", "Toto je dodatočný kontrolný výkaz — najprv načítajte riadny alebo opravný výkaz za "+p.Label()+", potom dodatočné v poradí podania")
		}
		if kv, err = vatreturn.ApplySupplement(*base, f.KV); err != nil {
			return LoadedKV{}, vErr("file", err.Error())
		}
		if prev != nil {
			files = prev.Files + 1
		}
	}
	l := LoadedKV{KV: kv, Druh: f.Druh, Files: files, User: actorName(ctx), Loaded: a.clock().UTC()}
	b, _ := json.Marshal(l)
	if err := a.Store.SetSetting(ctx, loadedKVKey(companyID, p.String()), string(b)); err != nil {
		return LoadedKV{}, err
	}
	a.record(ctx, companyID, audit.CatTaxes, "import", "vat", 0,
		fmt.Sprintf("Načítaný podaný kontrolný výkaz (%s) za %s zo súboru XML", map[string]string{"R": "riadny", "O": "opravný", "D": "dodatočný"}[f.Druh], p.Label()), nil, nil)
	return l, nil
}

// Taxpayer pripraví údaje platiteľa do hlavičky.
func Taxpayer(c store.Company) vatreturn.Taxpayer {
	return vatreturn.Taxpayer{Name: c.Name, Street: c.Street, City: c.City, Zip: c.Zip, Phone: c.Phone, Email: c.Email,
		DIC: c.DIC, ICDPH: c.ICDPH, TaxOffice: c.TaxOffice}
}

// VATExport vytvorí XML pre eDane ("dph" alebo "kv") a zapíše to do auditu.
func (a *App) VATExport(ctx context.Context, companyID int64, kind, period string, excess money.Dec) ([]byte, string, error) {
	return a.VATExportWith(ctx, companyID, kind, period, excess, vatreturn.Options{})
}

// VATExportWith vytvorí XML zvoleného druhu priznania (opravné, dodatočné).
func (a *App) VATExportWith(ctx context.Context, companyID int64, kind, period string, excess money.Dec, o vatreturn.Options) ([]byte, string, error) {
	r, c, err := a.VATReturnWith(ctx, companyID, period, excess, o)
	if err != nil {
		return nil, "", err
	}
	if kind == "kv" && r.Kind == vatreturn.KindSupplementary {
		if r.KVFix == nil {
			return nil, "", &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "kind", Message: r.KVFixNote}}}
		}
		r.KV = *r.KVFix // dodatočný KV obsahuje iba opravované riadky
	}
	if c.ICDPH == "" || c.DIC == "" {
		return nil, "", ErrMissingVATData
	}
	tp := Taxpayer(c)
	var out []byte
	var name, what string
	switch kind {
	case "kv":
		out, name, what = vatreturn.KVXML(r, tp), "kv-dph-"+r.Period.String()+suffix(r.Kind)+".xml", "kontrolný výkaz"+kindText(r.Kind)
	case "sv":
		out, name, what = vatreturn.SVXML(r, tp, a.Today()), "sv-dph-"+r.Period.String()+".xml", "súhrnný výkaz"
	default:
		out, name, what = vatreturn.DPHXML(r, tp, a.Today()), "dph-"+r.Period.String()+suffix(r.Kind)+".xml", "priznanie k DPH"+kindText(r.Kind)
	}
	a.record(ctx, companyID, audit.CatTaxes, "export", "vat", 0,
		"Export "+what+" za "+r.Label+" (XML pre eDane), vlastná daňová povinnosť "+r.Rows["r32"].Display()+" €, nadmerný odpočet "+r.Rows["r33"].Display()+" €",
		nil, map[string]any{"period": r.Period.String(), "kind": kind, "r32": r.Rows["r32"], "r33": r.Rows["r33"]})
	return out, name, nil
}

func suffix(k vatreturn.Kind) string {
	switch k {
	case vatreturn.KindCorrective:
		return "-opravne"
	case vatreturn.KindSupplementary:
		return "-dodatocne"
	}
	return ""
}

func kindText(k vatreturn.Kind) string {
	switch k {
	case vatreturn.KindCorrective:
		return " (opravné)"
	case vatreturn.KindSupplementary:
		return " (dodatočné)"
	}
	return ""
}

// VATFiling je záznam o podanom priznaní k DPH (posledná známa povinnosť
// pre dodatočné priznanie). Fiškus priznanie nepodáva sám — zapíše ho
// používateľ po podaní v eDane.
type VATFiling struct {
	Kind   vatreturn.Kind `json:"kind"`
	Filed  invoice.Date   `json:"filed"`
	Result money.Dec      `json:"result"` // + vlastná daňová povinnosť, − nadmerný odpočet
	R35    money.Dec      `json:"r35"`
	R37    money.Dec      `json:"r37"`
	// KV je kontrolný výkaz v stave po tomto podaní (celý, aj po
	// dodatočnom) — základ pre ďalší dodatočný KV.
	KV       *vatreturn.KV `json:"kv,omitempty"`
	User     string        `json:"user"`
	Recorded time.Time     `json:"recorded"`
}

func filingKey(companyID int64, period string) string {
	return fmt.Sprintf("vat.filed.%d.%s", companyID, period)
}

// VATFilings vráti zaznamenané podania za obdobie (najstaršie prvé).
func (a *App) VATFilings(ctx context.Context, companyID int64, period string) ([]VATFiling, error) {
	if err := a.Require(ctx, companyID, auth.PermVATRead); err != nil {
		return nil, err
	}
	p, err := vatreturn.ParsePeriod(period)
	if err != nil {
		return nil, &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "period", Message: err.Error()}}}
	}
	out := []VATFiling{}
	raw, err := a.Store.GetSetting(ctx, filingKey(companyID, p.String()))
	if errors.Is(err, store.ErrNotFound) || raw == "" {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	return out, json.Unmarshal([]byte(raw), &out)
}

// RecordVATFiling zapíše, že priznanie (s aktuálnymi údajmi a zvoleným
// druhom) bolo podané v deň filed.
func (a *App) RecordVATFiling(ctx context.Context, companyID int64, period string, excess money.Dec, o vatreturn.Options, filed invoice.Date) ([]VATFiling, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return nil, err
	}
	r, _, err := a.VATReturnWith(ctx, companyID, period, excess, o)
	if err != nil {
		return nil, err
	}
	if filed.IsZero() {
		filed = a.Today()
	}
	list, err := a.VATFilings(ctx, companyID, period)
	if err != nil {
		return nil, err
	}
	kv := r.KV
	list = append(list, VATFiling{Kind: r.Kind, Filed: filed, Result: r.Result(), R35: r.Rows["r35"], R37: r.Rows["r37"], KV: &kv,
		User: actorName(ctx), Recorded: a.clock().UTC()})
	b, _ := json.Marshal(list)
	if err := a.Store.SetSetting(ctx, filingKey(companyID, r.Period.String()), string(b)); err != nil {
		return nil, err
	}
	a.record(ctx, companyID, audit.CatTaxes, "filed", "vat", 0,
		fmt.Sprintf("Podané %s priznanie k DPH za %s (%s), výsledok %s €", r.Kind, r.Label, filed.Display(), r.Result().Display()), nil, list[len(list)-1])
	return list, nil
}
