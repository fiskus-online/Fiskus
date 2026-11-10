package app

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/job"
	"github.com/fiskus-online/fiskus/backend/internal/link"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/stock"
	"github.com/fiskus-online/fiskus/backend/internal/store"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

// Výkaz práce a materiál na zákazke. Materiál z cenníka vedený na sklade
// sa vydá výdajkou zákazky (jedna výdajka na deň, upravuje sa so zákazkou);
// náklad je z ocenenia skladu. Nevyfakturovanú prácu a materiál možno
// jedným krokom preniesť do konceptu faktúry — riadky faktúry potom nie
// sú viazané na sklad, lebo materiál je už vydaný.

// jobStockLink je LinkKind výdajok zákazky.
const jobStockLink = "job"

// releaseJob zruší vyfakturovanie záznamov, ktorých faktúra už neplatí
// (zmazaný koncept, storno), a doplní čísla faktúr (koncept dostane číslo
// až pri vystavení). Vráti true, ak sa niečo zmenilo.
func (a *App) releaseJob(ctx context.Context, companyID int64, j *job.Job) bool {
	numbers := map[int64]string{}
	valid := func(id int64) bool {
		if _, ok := numbers[id]; ok {
			return true
		}
		inv, err := a.Store.GetInvoice(ctx, companyID, id)
		if errors.Is(err, store.ErrNotFound) || (err == nil && inv.Status == invoice.StatusCancelled) {
			return false
		}
		numbers[id] = "koncept"
		if err == nil && inv.Number != "" {
			numbers[id] = inv.Number
		}
		return true
	}
	changed := j.Release(valid)
	fix := func(b *job.Billing) {
		if n, ok := numbers[b.InvoiceID]; b.Billed() && ok && n != b.InvoiceNumber {
			b.InvoiceNumber, changed = n, true
		}
	}
	for i := range j.Work {
		fix(&j.Work[i].Billing)
	}
	for i := range j.Material {
		fix(&j.Material[i].Billing)
	}
	return changed
}

// prepareJobMaterial doplní k materiálu z cenníka názov a jednotku
// a označí položky vedené na sklade.
func (a *App) prepareJobMaterial(ctx context.Context, companyID int64, j *job.Job) error {
	if len(j.Material) == 0 {
		return nil
	}
	cfg, err := a.loadStock(ctx, companyID)
	if err != nil {
		return err
	}
	for i := range j.Material {
		m := &j.Material[i]
		m.Stock = false
		if m.ItemID == 0 {
			continue
		}
		it, err := a.Store.GetItem(ctx, companyID, m.ItemID)
		if err != nil {
			return jobErr([]job.FieldError{{Field: fmt.Sprintf("material[%d].itemId", i), Message: "Položka nie je v cenníku"}})
		}
		m.Name, m.Unit = it.Name, it.Unit
		if _, ok := cfg.item(m.ItemID); ok && len(cfg.Warehouses) > 0 {
			m.Stock, m.UnitCost = true, money.Zero // náklad z ocenenia skladu
		}
	}
	return nil
}

// syncJobStock zosúladí výdajky zákazky so skladovým materiálom.
func (a *App) syncJobStock(ctx context.Context, companyID int64, j job.Job) error {
	stockMu.Lock()
	defer stockMu.Unlock()
	cfg, err := a.loadStock(ctx, companyID)
	if err != nil {
		return err
	}
	docs, err := a.Store.ListStockDocs(ctx, companyID)
	if err != nil {
		return err
	}
	existing := map[invoice.Date]stock.Doc{}
	for _, d := range docs {
		if d.Auto && d.LinkKind == jobStockLink && d.LinkID == j.ID {
			if _, dup := existing[d.Date]; dup {
				if err := a.Store.DeleteStockDoc(ctx, companyID, d.ID); err != nil {
					return err
				}
				continue
			}
			existing[d.Date] = d
		}
	}
	byDate := map[invoice.Date][]stock.Line{}
	var dates []invoice.Date
	for _, m := range j.Material {
		if !m.Stock {
			continue
		}
		if _, ok := byDate[m.Date]; !ok {
			dates = append(dates, m.Date)
		}
		byDate[m.Date] = append(byDate[m.Date], stock.Line{ItemID: m.ItemID, Quantity: m.Quantity})
	}
	for _, day := range dates {
		d := stock.Doc{Kind: stock.Issue, Date: day, Partner: j.Customer, Note: "Materiál na zákazku " + j.Number,
			LinkKind: jobStockLink, LinkID: j.ID, LinkNumber: j.Number, Auto: true, Lines: byDate[day]}
		if len(cfg.Warehouses) > 0 {
			d.Warehouse = cfg.Warehouses[0].Code
		}
		action, summary := "create", ""
		var before any
		if old, ok := existing[day]; ok {
			d.ID, d.Number, d.Warehouse, action, before = old.ID, old.Number, old.Warehouse, "update", old
			delete(existing, day)
		}
		if err := a.checkStockDoc(ctx, companyID, cfg, &d); err != nil {
			return err
		}
		if d.ID == 0 {
			d.Number = stock.NextNumber(docs, d.Kind, d.Date.Year())
			docs = append(docs, d)
		}
		if err := a.Store.SaveStockDoc(ctx, companyID, &d); err != nil {
			return err
		}
		summary = stockDocSummary(d) + " (zákazka " + j.Number + ")"
		a.record(ctx, companyID, audit.CatSettings, action, "stock_doc", d.ID, summary, before, d)
	}
	for _, old := range existing {
		if err := a.Store.DeleteStockDoc(ctx, companyID, old.ID); err != nil {
			return err
		}
		a.record(ctx, companyID, audit.CatSettings, "delete", "stock_doc", old.ID, "Zmazaný "+stockDocSummary(old)+" (zákazka "+j.Number+")", old, nil)
	}
	return nil
}

// jobMaterialCosts doplní náklad materiálu: skladový z ocenenia výdajok
// zákazky, ostatný ako množstvo × nákupná cena.
func (a *App) jobMaterialCosts(ctx context.Context, companyID int64, j *job.Job) {
	var costed []stock.Doc
	if hasStock(j) {
		cfg, err := a.loadStock(ctx, companyID)
		if err == nil {
			if docs, err := a.Store.ListStockDocs(ctx, companyID); err == nil {
				_, costed, _ = stock.Valuate(docs, cfg.Method)
			} else {
				log.Printf("sklad k zákazke %s: %v", j.Number, err)
			}
		}
	}
	lines := map[invoice.Date][]stock.Line{}
	for _, d := range costed {
		if d.Auto && d.LinkKind == jobStockLink && d.LinkID == j.ID {
			lines[d.Date] = d.Lines
		}
	}
	used := map[invoice.Date]int{}
	for i := range j.Material {
		m := &j.Material[i]
		if !m.Stock {
			m.Cost = m.Quantity.Mul(m.UnitCost).RoundCents()
			continue
		}
		m.Cost = money.Zero
		ls, k := lines[m.Date], used[m.Date]
		if k < len(ls) && ls[k].ItemID == m.ItemID {
			m.Cost = ls[k].Total
		}
		used[m.Date] = k + 1
	}
}

func hasStock(j *job.Job) bool {
	for _, m := range j.Material {
		if m.Stock {
			return true
		}
	}
	return false
}

// InvoiceCrmJob vytvorí koncept faktúry z nevyfakturovanej práce
// a materiálu zákazky, prepojí ho so zákazkou a záznamy označí.
func (a *App) InvoiceCrmJob(ctx context.Context, companyID, id int64) (invoice.Invoice, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return invoice.Invoice{}, err
	}
	c, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return invoice.Invoice{}, err
	}
	j, err := a.Store.GetJob(ctx, companyID, id)
	if err != nil {
		return invoice.Invoice{}, err
	}
	a.releaseJob(ctx, companyID, &j)
	if j.ContactID == 0 {
		return invoice.Invoice{}, jobErr([]job.FieldError{{Field: "contactId", Message: "Zákazka nemá zákazníka z adresára"}})
	}
	ct, err := a.Store.GetContact(ctx, companyID, j.ContactID)
	if err != nil {
		return invoice.Invoice{}, jobErr([]job.FieldError{{Field: "contactId", Message: "Zákazník sa v adresári nenašiel"}})
	}
	rate := vat.Rate0
	if c.VATPayer {
		rate = vat.Rate23
	}
	inv := jobInvoiceDraft(j, rate, a.Today())
	if len(inv.Lines) == 0 {
		return invoice.Invoice{}, jobErr([]job.FieldError{{Field: "work", Message: "Na zákazke nie je nevyfakturovaná práca ani materiál"}})
	}
	inv.Customer, inv.ContactID = PartyFromContact(ct), ct.ID
	saved, err := a.saveDraft(ctx, companyID, a.withDocExtras(ctx, companyID, inv))
	if err != nil {
		return saved, err
	}
	a.record(ctx, companyID, audit.CatInvoices, "create", "invoice", saved.ID,
		"Koncept faktúry zo zákazky "+j.Number+" pre "+saved.Customer.Name, nil, saved)
	before := j
	for i := range j.Work {
		if !j.Work[i].Billed() {
			j.Work[i].Billing = job.Billing{InvoiceID: saved.ID, InvoiceNumber: "koncept"}
		}
	}
	for i := range j.Material {
		if !j.Material[i].Billed() {
			j.Material[i].Billing = job.Billing{InvoiceID: saved.ID, InvoiceNumber: "koncept"}
		}
	}
	if err := a.Store.SaveJob(ctx, companyID, &j); err != nil {
		return saved, err
	}
	a.record(ctx, companyID, audit.CatInvoices, "invoice", "job", j.ID, "Vyfakturovaná práca a materiál zákazky "+j.Number, before, j)
	l, err := link.New(link.Ref{Kind: link.Job, ID: j.ID}, link.Ref{Kind: link.Invoice, ID: saved.ID})
	if err != nil {
		return saved, err
	}
	if err := a.Store.SaveDocLink(ctx, companyID, &l); err != nil && !errors.Is(err, store.ErrDuplicate) {
		return saved, err
	}
	return saved, nil
}

// jobInvoiceDraft zostaví koncept faktúry z nevyfakturovaných záznamov:
// práca ako hodiny so sadzbou, materiál bez väzby na sklad (už je vydaný).
func jobInvoiceDraft(j job.Job, rate vat.Rate, today invoice.Date) invoice.Invoice {
	inv := invoice.Invoice{Kind: invoice.KindInvoice, Status: invoice.StatusDraft, IssueDate: today, DeliveryDate: today,
		PriceMode: vat.PriceNet, Subject: j.Name, Note: "Zákazka " + j.Number + "."}
	var last invoice.Date
	for _, w := range j.Work {
		if w.Billed() {
			continue
		}
		desc := w.Description + " (" + w.Date.Display()
		if w.Person != "" {
			desc += ", " + w.Person
		}
		desc += ")"
		inv.Lines = append(inv.Lines, invoice.Line{Description: desc, Quantity: w.Hours, Unit: "hod", UnitPrice: w.Rate, Rate: rate})
		if w.Date.After(last) {
			last = w.Date
		}
	}
	for _, m := range j.Material {
		if m.Billed() {
			continue
		}
		inv.Lines = append(inv.Lines, invoice.Line{Description: m.Name, Quantity: m.Quantity, Unit: m.Unit, UnitPrice: m.UnitPrice, Rate: rate})
		if m.Date.After(last) {
			last = m.Date
		}
	}
	if !last.IsZero() && last.Before(today) {
		inv.DeliveryDate = last // dodanie = posledná práca alebo materiál
	}
	return inv
}
