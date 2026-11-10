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
	"github.com/fiskus-online/fiskus/backend/internal/link"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/porder"
	"github.com/fiskus-online/fiskus/backend/internal/stock"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// Objednávky dodávateľom: rozpracovaná → odoslaná → (čiastočne) prijatá,
// alebo zrušená. Príjem vytvorí príjemku na skladové položky v nákupnej
// cene z objednávky a zapíše ju ako poslednú nákupnú cenu položky.
// Prehľad „na objednanie“ porovná minimálnu zásobu so stavom skladu
// a s tým, čo už je objednané.

// porderSeries je číselný rad objednávok (OB20260001).
const porderSeries = "OB"

// porderStockLink je LinkKind príjemky z objednávky.
const porderStockLink = "porder"

func porderErr(errs []porder.FieldError) error {
	if len(errs) == 0 {
		return nil
	}
	fe := make([]invoice.FieldError, len(errs))
	for i, e := range errs {
		fe[i] = invoice.FieldError{Field: e.Field, Message: e.Message}
	}
	return &invoice.ValidationError{Fields: fe}
}

// PurchaseOrders vráti objednávky dodávateľom, najnovšie prvé.
func (a *App) PurchaseOrders(ctx context.Context, companyID int64) ([]porder.Order, error) {
	if err := a.Require(ctx, companyID, auth.PermPurchasesRead); err != nil {
		return nil, err
	}
	return a.Store.ListPurchaseOrders(ctx, companyID)
}

// PurchaseOrder vráti objednávku.
func (a *App) PurchaseOrder(ctx context.Context, companyID, id int64) (porder.Order, error) {
	if err := a.Require(ctx, companyID, auth.PermPurchasesRead); err != nil {
		return porder.Order{}, err
	}
	return a.Store.GetPurchaseOrder(ctx, companyID, id)
}

// preparePOrder doplní dodávateľa z adresára a riadky z cenníka.
func (a *App) preparePOrder(ctx context.Context, companyID int64, o *porder.Order) error {
	if o.ContactID != 0 {
		ct, err := a.Store.GetContact(ctx, companyID, o.ContactID)
		if errors.Is(err, store.ErrNotFound) {
			return porderErr([]porder.FieldError{{Field: "contactId", Message: "Dodávateľ sa v adresári nenašiel"}})
		} else if err != nil {
			return err
		}
		o.Supplier = ct.Name
	}
	for i := range o.Lines {
		l := &o.Lines[i]
		if l.ItemID == 0 {
			continue
		}
		it, err := a.Store.GetItem(ctx, companyID, l.ItemID)
		if err != nil {
			return porderErr([]porder.FieldError{{Field: fmt.Sprintf("lines[%d].itemId", i), Message: "Položka nie je v cenníku"}})
		}
		l.Name, l.Unit = it.Name, it.Unit
		if l.SupplierCode == "" {
			l.SupplierCode = it.SupplierCode
		}
	}
	return nil
}

// SavePurchaseOrder založí (ID 0) alebo upraví otvorenú objednávku.
// Prijaté množstvá sa z klienta neberú; riadok s príjmom nemožno zmazať.
func (a *App) SavePurchaseOrder(ctx context.Context, companyID int64, o porder.Order) (porder.Order, error) {
	if err := a.Require(ctx, companyID, auth.PermPurchasesWrite); err != nil {
		return o, err
	}
	var before *porder.Order
	if o.ID != 0 {
		old, err := a.Store.GetPurchaseOrder(ctx, companyID, o.ID)
		if err != nil {
			return o, err
		}
		if !old.Editable() {
			return o, porderErr([]porder.FieldError{{Field: "status", Message: "Prijatú alebo zrušenú objednávku nemožno meniť"}})
		}
		before = &old
		o.Number, o.Status, o.Sent = old.Number, old.Status, old.Sent
		for i := range o.Lines {
			o.Lines[i].Received = money.Zero
			if i < len(old.Lines) {
				o.Lines[i].Received = old.Lines[i].Received
			}
		}
		for i := len(o.Lines); i < len(old.Lines); i++ {
			if old.Lines[i].Received.Sign() > 0 {
				return o, porderErr([]porder.FieldError{{Field: "lines", Message: old.Lines[i].Name + " je čiastočne prijatá, riadok nemožno zmazať"}})
			}
		}
	} else {
		o.Status, o.Sent = porder.Draft, invoice.Date{}
		for i := range o.Lines {
			o.Lines[i].Received = money.Zero
		}
		if o.Date.IsZero() {
			o.Date = a.Today()
		}
	}
	if err := a.preparePOrder(ctx, companyID, &o); err != nil {
		return o, err
	}
	o.Normalize()
	if err := porderErr(o.Validate()); err != nil {
		return o, err
	}
	if before == nil {
		seq, err := a.Store.NextSequence(ctx, companyID, porderSeries, o.Date.Year())
		if err != nil {
			return o, err
		}
		o.Number = fmt.Sprintf("OB%04d%04d", o.Date.Year(), seq)
	}
	if err := a.Store.SavePurchaseOrder(ctx, companyID, &o); err != nil {
		return o, err
	}
	action, what := "create", "Nová objednávka "
	if before != nil {
		action, what = "update", "Úprava objednávky "
	}
	a.record(ctx, companyID, audit.CatPurchases, action, "porder", o.ID, what+o.Number+" pre "+o.Supplier, before, o)
	return o, nil
}

// porderTransition zmení stav objednávky a zapíše ho do auditu.
func (a *App) porderTransition(ctx context.Context, companyID, id int64, action, what string, fn func(*porder.Order) error) (porder.Order, error) {
	if err := a.Require(ctx, companyID, auth.PermPurchasesWrite); err != nil {
		return porder.Order{}, err
	}
	o, err := a.Store.GetPurchaseOrder(ctx, companyID, id)
	if err != nil {
		return o, err
	}
	before := o.Status
	if err := fn(&o); err != nil {
		return o, err
	}
	if err := a.Store.SavePurchaseOrder(ctx, companyID, &o); err != nil {
		return o, err
	}
	type state struct {
		Status porder.Status `json:"status"`
	}
	a.record(ctx, companyID, audit.CatPurchases, action, "porder", o.ID, what+" "+o.Number, state{before}, state{o.Status})
	return o, nil
}

// SendPurchaseOrder označí rozpracovanú objednávku ako odoslanú.
func (a *App) SendPurchaseOrder(ctx context.Context, companyID, id int64) (porder.Order, error) {
	return a.porderTransition(ctx, companyID, id, "send", "Odoslaná objednávka", func(o *porder.Order) error {
		if o.Status != porder.Draft {
			return porderErr([]porder.FieldError{{Field: "status", Message: "Odoslať možno iba rozpracovanú objednávku"}})
		}
		o.Status, o.Sent = porder.Sent, a.Today()
		return nil
	})
}

// CancelPurchaseOrder zruší otvorenú objednávku (zostatok už nepríde).
func (a *App) CancelPurchaseOrder(ctx context.Context, companyID, id int64) (porder.Order, error) {
	return a.porderTransition(ctx, companyID, id, "cancel", "Zrušená objednávka", func(o *porder.Order) error {
		if !o.Status.Open() {
			return porderErr([]porder.FieldError{{Field: "status", Message: "Objednávka je už uzavretá"}})
		}
		o.Status = porder.Cancelled
		return nil
	})
}

// DeletePurchaseOrder zmaže objednávku, z ktorej sa ešte nič neprijalo.
func (a *App) DeletePurchaseOrder(ctx context.Context, companyID, id int64) error {
	if err := a.Require(ctx, companyID, auth.PermPurchasesWrite); err != nil {
		return err
	}
	o, err := a.Store.GetPurchaseOrder(ctx, companyID, id)
	if err != nil {
		return err
	}
	for _, l := range o.Lines {
		if l.Received.Sign() > 0 {
			return porderErr([]porder.FieldError{{Field: "status", Message: "Z objednávky sa už prijímalo, môžete ju iba zrušiť"}})
		}
	}
	links, err := a.Store.ListDocLinks(ctx, companyID, link.Ref{Kind: link.POrder, ID: id})
	if err != nil {
		return err
	}
	for _, l := range links {
		if err := a.Store.DeleteDocLink(ctx, companyID, l.ID); err != nil {
			return err
		}
	}
	if err := a.Store.DeletePurchaseOrder(ctx, companyID, id); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatPurchases, "delete", "porder", id, "Zmazaná objednávka "+o.Number+" pre "+o.Supplier, o, nil)
	return nil
}

// POrderReceipt je príjem z objednávky: dátum, sklad a množstvá po riadkoch.
type POrderReceipt struct {
	Date      invoice.Date `json:"date"`
	Warehouse string       `json:"warehouse"`
	Quantity  []money.Dec  `json:"quantity"` // podľa riadkov objednávky (0 = nič)
}

// POrderReceived je výsledok príjmu: objednávka a príjemka (ak vznikla).
type POrderReceived struct {
	Order   porder.Order `json:"order"`
	StockID int64        `json:"stockId,omitempty"`
	Stock   string       `json:"stockNumber,omitempty"`
}

// ReceivePurchaseOrder zapíše príjem. Skladové položky sa prijmú
// príjemkou v nákupnej cene z objednávky a cena sa zapíše do cenníka
// ako posledná nákupná.
func (a *App) ReceivePurchaseOrder(ctx context.Context, companyID, id int64, r POrderReceipt) (POrderReceived, error) {
	if err := a.Require(ctx, companyID, auth.PermPurchasesWrite); err != nil {
		return POrderReceived{}, err
	}
	stockMu.Lock()
	defer stockMu.Unlock()
	o, err := a.Store.GetPurchaseOrder(ctx, companyID, id)
	if err != nil {
		return POrderReceived{}, err
	}
	if r.Date.IsZero() {
		r.Date = a.Today()
	}
	qty := map[int]money.Dec{}
	for i, q := range r.Quantity {
		qty[i] = q
	}
	before := o
	before.Lines = append([]porder.Line(nil), o.Lines...)
	if err := porderErr(o.Receive(qty)); err != nil {
		return POrderReceived{}, err
	}
	cfg, err := a.loadStock(ctx, companyID)
	if err != nil {
		return POrderReceived{}, err
	}
	var lines []stock.Line
	for i, q := range qty {
		if q.Sign() <= 0 || i >= len(o.Lines) {
			continue
		}
		l := o.Lines[i]
		if _, ok := cfg.item(l.ItemID); l.ItemID == 0 || !ok {
			continue
		}
		lines = append(lines, stock.Line{ItemID: l.ItemID, Quantity: q, UnitCost: l.UnitPrice})
	}
	sort.Slice(lines, func(i, k int) bool { return lines[i].ItemID < lines[k].ItemID })
	out := POrderReceived{}
	if len(lines) > 0 {
		if r.Warehouse == "" && len(cfg.Warehouses) > 0 {
			r.Warehouse = cfg.Warehouses[0].Code
		}
		d := stock.Doc{Kind: stock.Receipt, Date: r.Date, Warehouse: r.Warehouse, Partner: o.Supplier, Note: "Príjem z objednávky " + o.Number,
			LinkKind: porderStockLink, LinkID: o.ID, LinkNumber: o.Number, Lines: lines}
		if err := a.checkStockDoc(ctx, companyID, cfg, &d); err != nil {
			return POrderReceived{}, err
		}
		docs, err := a.Store.ListStockDocs(ctx, companyID)
		if err != nil {
			return POrderReceived{}, err
		}
		d.Number = stock.NextNumber(docs, d.Kind, d.Date.Year())
		if err := a.Store.SaveStockDoc(ctx, companyID, &d); err != nil {
			return POrderReceived{}, err
		}
		a.record(ctx, companyID, audit.CatSettings, "create", "stock_doc", d.ID, stockDocSummary(d)+" z objednávky "+o.Number, nil, d)
		out.StockID, out.Stock = d.ID, d.Number
		for _, l := range lines {
			a.setPurchasePrice(ctx, companyID, l.ItemID, l.UnitCost)
		}
	}
	if err := a.Store.SavePurchaseOrder(ctx, companyID, &o); err != nil {
		return out, err
	}
	a.record(ctx, companyID, audit.CatPurchases, "receive", "porder", o.ID, "Príjem z objednávky "+o.Number, before, o)
	out.Order = o
	return out, nil
}

// setPurchasePrice zapíše poslednú nákupnú cenu položky cenníka.
func (a *App) setPurchasePrice(ctx context.Context, companyID, itemID int64, price money.Dec) {
	it, err := a.Store.GetItem(ctx, companyID, itemID)
	if err != nil || price.Sign() <= 0 || it.PurchasePrice.Cmp(price) == 0 {
		return
	}
	before := it
	it.PurchasePrice = price
	if err := a.Store.SaveItem(ctx, companyID, &it); err == nil {
		a.record(ctx, companyID, audit.CatSettings, "update", "item", it.ID, "Nákupná cena "+it.Name+" "+price.String()+" € (objednávka)", before, it)
	}
}

// ReorderRow je položka pod minimálnou zásobou.
type ReorderRow struct {
	ItemID        int64     `json:"itemId"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	Unit          string    `json:"unit"`
	Supplier      string    `json:"supplier"`
	SupplierCode  string    `json:"supplierCode"`
	PurchasePrice money.Dec `json:"purchasePrice"`
	Min           money.Dec `json:"min"`
	Stock         money.Dec `json:"stock"`   // stav na všetkých skladoch
	OnOrder       money.Dec `json:"onOrder"` // objednané, ešte neprijaté
	Suggested     money.Dec `json:"suggested"`
}

// Reorder vráti skladové položky, ktorých stav aj s objednaným je pod
// minimálnou zásobou, s navrhovaným množstvom (doplnenie na minimum).
func (a *App) Reorder(ctx context.Context, companyID int64) ([]ReorderRow, error) {
	if err := a.Require(ctx, companyID, auth.PermPurchasesRead); err != nil {
		return nil, err
	}
	cfg, err := a.loadStock(ctx, companyID)
	if err != nil {
		return nil, err
	}
	docs, err := a.Store.ListStockDocs(ctx, companyID)
	if err != nil {
		return nil, err
	}
	cards, _, _ := stock.Valuate(docs, cfg.Method)
	have := map[int64]money.Dec{}
	for k, c := range cards {
		have[k.ItemID] = have[k.ItemID].Add(c.Quantity)
	}
	orders, err := a.Store.ListPurchaseOrders(ctx, companyID)
	if err != nil {
		return nil, err
	}
	onOrder := map[int64]money.Dec{}
	for _, o := range orders {
		if !o.Status.Open() {
			continue
		}
		for _, l := range o.Lines {
			if l.ItemID != 0 {
				onOrder[l.ItemID] = onOrder[l.ItemID].Add(l.Remaining())
			}
		}
	}
	out := []ReorderRow{}
	for _, si := range cfg.Items {
		if si.Min.Sign() <= 0 {
			continue
		}
		missing := si.Min.Sub(have[si.ItemID]).Sub(onOrder[si.ItemID])
		if missing.Sign() <= 0 {
			continue
		}
		it, err := a.Store.GetItem(ctx, companyID, si.ItemID)
		if err != nil || it.Archived {
			continue
		}
		out = append(out, ReorderRow{ItemID: it.ID, Code: it.Code, Name: it.Name, Unit: it.Unit, Supplier: it.Supplier, SupplierCode: it.SupplierCode,
			PurchasePrice: it.PurchasePrice, Min: si.Min, Stock: have[si.ItemID], OnOrder: onOrder[si.ItemID], Suggested: ceilWhole(missing)})
	}
	sort.Slice(out, func(i, k int) bool {
		if out[i].Supplier != out[k].Supplier {
			return strings.ToLower(out[i].Supplier) < strings.ToLower(out[k].Supplier)
		}
		return out[i].Name < out[k].Name
	})
	return out, nil
}

// ReorderPick je položka a množstvo na objednanie.
type ReorderPick struct {
	ItemID   int64     `json:"itemId"`
	Quantity money.Dec `json:"quantity"`
}

// OrdersFromReorder založí rozpracované objednávky, jednu na dodávateľa
// (podľa karty produktu; dodávateľ sa nájde v adresári podľa názvu).
func (a *App) OrdersFromReorder(ctx context.Context, companyID int64, picks []ReorderPick) ([]porder.Order, error) {
	if err := a.Require(ctx, companyID, auth.PermPurchasesWrite); err != nil {
		return nil, err
	}
	contacts, err := a.Store.ListContacts(ctx, companyID, "")
	if err != nil {
		return nil, err
	}
	byName := map[string]int64{}
	for _, c := range contacts {
		byName[strings.ToLower(strings.TrimSpace(c.Name))] = c.ID
	}
	groups := map[string]*porder.Order{}
	var keys []string
	for i, p := range picks {
		if p.Quantity.Sign() <= 0 {
			continue
		}
		it, err := a.Store.GetItem(ctx, companyID, p.ItemID)
		if err != nil {
			return nil, porderErr([]porder.FieldError{{Field: fmt.Sprintf("picks[%d].itemId", i), Message: "Položka nie je v cenníku"}})
		}
		sup := strings.TrimSpace(it.Supplier)
		if sup == "" {
			sup = "Bez dodávateľa"
		}
		o := groups[sup]
		if o == nil {
			o = &porder.Order{Supplier: sup, ContactID: byName[strings.ToLower(sup)], Date: a.Today()}
			groups[sup] = o
			keys = append(keys, sup)
		}
		o.Lines = append(o.Lines, porder.Line{ItemID: it.ID, Quantity: p.Quantity, UnitPrice: it.PurchasePrice})
	}
	if len(keys) == 0 {
		return nil, porderErr([]porder.FieldError{{Field: "picks", Message: "Vyberte položky na objednanie"}})
	}
	sort.Slice(keys, func(i, k int) bool { return strings.ToLower(keys[i]) < strings.ToLower(keys[k]) })
	out := make([]porder.Order, 0, len(keys))
	for _, k := range keys {
		o, err := a.SavePurchaseOrder(ctx, companyID, *groups[k])
		if err != nil {
			return out, err
		}
		out = append(out, o)
	}
	return out, nil
}

// ceilWhole zaokrúhli množstvo nahor na celé jednotky.
func ceilWhole(d money.Dec) money.Dec {
	r := d.Rat()
	q := new(big.Int).Quo(r.Num(), r.Denom())
	if new(big.Rat).SetInt(q).Cmp(r) < 0 {
		q.Add(q, big.NewInt(1))
	}
	return money.FromInt(q.Int64())
}
