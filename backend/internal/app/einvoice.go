package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/postman"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/ubl"
)

func einvoiceKey(companyID int64) string { return fmt.Sprintf("einvoice.%d", companyID) }

func (a *App) loadEInvoice(ctx context.Context, companyID int64) (postman.Settings, error) {
	var s postman.Settings
	raw, err := a.Store.GetSetting(ctx, einvoiceKey(companyID))
	if errors.Is(err, store.ErrNotFound) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	err = json.Unmarshal([]byte(raw), &s)
	return s, err
}

func publicEInvoice(s postman.Settings) postman.Settings {
	s.TokenSet, s.Token = s.Token != "", ""
	return s
}

// EInvoiceSettings vráti pripojenie firmy k digitálnemu poštárovi (bez tokenu).
func (a *App) EInvoiceSettings(ctx context.Context, companyID int64) (postman.Settings, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return postman.Settings{}, err
	}
	s, err := a.loadEInvoice(ctx, companyID)
	return publicEInvoice(s), err
}

// SetEInvoiceSettings uloží pripojenie. Prázdny token ponechá uložený.
// Priečinky na serveri smie nastaviť iba správca inštalácie (prístup k disku servera).
func (a *App) SetEInvoiceSettings(ctx context.Context, companyID int64, s postman.Settings) (postman.Settings, error) {
	if err := a.Require(ctx, companyID, auth.PermSettingsWrite); err != nil {
		return postman.Settings{}, err
	}
	s.Normalize()
	if s.Mode == postman.ModeFolder && a.Caps.Mode == edition.Server {
		if err := requireSuperAdmin(ctx); err != nil {
			return postman.Settings{}, err
		}
	}
	old, err := a.loadEInvoice(ctx, companyID)
	if err != nil {
		return postman.Settings{}, err
	}
	// uložený token ide iba na tú istú adresu poskytovateľa
	if s.Token == "" && s.Mode == old.Mode && s.URL == old.URL {
		s.Token = old.Token
	}
	s.TokenSet = false
	if err := s.Validate(); err != nil {
		return postman.Settings{}, err
	}
	b, _ := json.Marshal(s)
	if err := a.Store.SetSetting(ctx, einvoiceKey(companyID), string(b)); err != nil {
		return postman.Settings{}, err
	}
	a.record(ctx, companyID, audit.CatSettings, "update", "einvoice", companyID,
		"Nastavenie e-faktúr: "+modeLabel(s), publicEInvoice(old), publicEInvoice(s))
	return publicEInvoice(s), nil
}

func modeLabel(s postman.Settings) string {
	switch s.Mode {
	case postman.ModeFolder:
		return "priečinky " + s.OutDir + " / " + s.InDir
	case postman.ModeHTTP:
		return "API " + s.URL
	case postman.ModeSAPI:
		return "SAPI-SK " + s.URL
	}
	return "vypnuté"
}

func (a *App) provider(ctx context.Context, companyID int64) (postman.Provider, error) {
	s, err := a.loadEInvoice(ctx, companyID)
	if err != nil {
		return nil, err
	}
	if co, err := a.Store.GetCompany(ctx, companyID); err == nil {
		s.Participant = postman.PeppolID(co.DIC)
	}
	if a.Postman != nil {
		return a.Postman(s)
	}
	return postman.New(s)
}

// CheckEInvoice overí uložené pripojenie k poštárovi (prihlásenie a schránku
// firmy) bez odoslania alebo prevzatia dokumentu.
func (a *App) CheckEInvoice(ctx context.Context, companyID int64) error {
	if err := a.Require(ctx, companyID, auth.PermSettingsWrite); err != nil {
		return err
	}
	p, err := a.provider(ctx, companyID)
	if err != nil {
		return err
	}
	if c, ok := p.(postman.Checker); ok {
		return c.Check(ctx)
	}
	_, err = p.Inbox(ctx)
	return err
}

// SendEInvoice odošle e-faktúru vystavenej faktúry cez digitálneho poštára
// a zapíše výsledok (aj neúspešný) do zoznamu e-faktúr.
func (a *App) SendEInvoice(ctx context.Context, companyID, id int64) (store.EInvoice, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return store.EInvoice{}, err
	}
	p, err := a.provider(ctx, companyID)
	if err != nil {
		return store.EInvoice{}, err
	}
	doc, err := a.InvoiceUBL(ctx, companyID, id)
	if err != nil {
		return store.EInvoice{}, err
	}
	inv, err := a.Store.GetInvoice(ctx, companyID, id)
	if err != nil {
		return store.EInvoice{}, err
	}
	co, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return store.EInvoice{}, err
	}
	receiver := postman.PeppolID(inv.Customer.DIC)
	e := store.EInvoice{Direction: "out", DocID: inv.ID, DocNumber: inv.Number, Partner: inv.Customer.Name, PeppolID: receiver,
		File: doc.Name, Created: a.now(), User: auth.ActorFrom(ctx).Name}
	sent, sendErr := p.Send(ctx, postman.Outgoing{Sender: postman.PeppolID(co.DIC), Receiver: receiver, FileName: doc.Name, Data: doc.Data})
	if sendErr != nil {
		e.Error = sendErr.Error()
	} else {
		e.MessageID, e.Status = sent.MessageID, sent.Status
	}
	if err := a.Store.AddEInvoice(ctx, companyID, &e); err != nil {
		return e, err
	}
	summary := fmt.Sprintf("E-faktúra %s → %s (%s)", inv.Number, inv.Customer.Name, receiver)
	if sendErr != nil {
		summary += " — neodoslaná: " + sendErr.Error()
	}
	a.record(ctx, companyID, audit.CatInvoices, "einvoice", "invoice", id, summary, nil, e)
	return e, sendErr
}

// EInvoices vráti zoznam odoslaných alebo prijatých e-faktúr.
func (a *App) EInvoices(ctx context.Context, companyID int64, f store.EInvoiceFilter) ([]store.EInvoice, error) {
	perm := auth.PermInvoicesRead
	if f.Direction == "in" {
		perm = auth.PermPurchasesRead
	}
	if err := a.Require(ctx, companyID, perm); err != nil {
		return nil, err
	}
	if f.Direction == "" && a.Require(ctx, companyID, auth.PermPurchasesRead) != nil {
		f.Direction = "out"
	}
	return a.Store.ListEInvoices(ctx, companyID, f)
}

// InboxItem je e-faktúra v schránke poštára s prehľadom (pred načítaním).
type InboxItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Received string `json:"received"`
	Supplier string `json:"supplier"`
	Number   string `json:"number"`
	Total    string `json:"total"`
	Currency string `json:"currency"`
	Error    string `json:"error,omitempty"` // súbor nie je čitateľná e-faktúra
}

// EInvoiceInbox vráti prijaté e-faktúry čakajúce v schránke poštára.
func (a *App) EInvoiceInbox(ctx context.Context, companyID int64) ([]InboxItem, error) {
	if err := a.Require(ctx, companyID, auth.PermPurchasesWrite); err != nil {
		return nil, err
	}
	p, err := a.provider(ctx, companyID)
	if err != nil {
		return nil, err
	}
	msgs, err := p.Inbox(ctx)
	if err != nil {
		return nil, err
	}
	out := []InboxItem{}
	for _, m := range msgs {
		it := InboxItem{ID: m.ID, Name: m.Name, Received: m.Received.Format("2006-01-02T15:04:05Z07:00")}
		if r, err := ubl.Read(m.Data, a.Today()); err != nil {
			it.Error = err.Error()
		} else {
			pu := r.Purchase
			pu.Normalize()
			it.Supplier, it.Number, it.Total = pu.Supplier.Name, pu.SupplierNumber, pu.Summary.Total.String()
			it.Currency = currencyOr(pu.Currency)
		}
		out = append(out, it)
	}
	return out, nil
}

// TakeEInvoice prevezme e-faktúru zo schránky: uloží ju do zoznamu prijatých
// (aj s XML, aby sa dala otvoriť znova), potvrdí poštárovi a vráti koncept
// prijatej faktúry na kontrolu.
func (a *App) TakeEInvoice(ctx context.Context, companyID int64, messageID string) (PurchaseDraft, error) {
	if err := a.Require(ctx, companyID, auth.PermPurchasesWrite); err != nil {
		return PurchaseDraft{}, err
	}
	p, err := a.provider(ctx, companyID)
	if err != nil {
		return PurchaseDraft{}, err
	}
	msgs, err := p.Inbox(ctx)
	if err != nil {
		return PurchaseDraft{}, err
	}
	for _, m := range msgs {
		if m.ID != messageID {
			continue
		}
		d, err := a.ImportUBL(ctx, companyID, m.Name, m.Data)
		if err != nil {
			return PurchaseDraft{}, err
		}
		e := store.EInvoice{Direction: "in", DocNumber: d.Purchase.SupplierNumber, Partner: d.Purchase.Supplier.Name,
			PeppolID: postman.PeppolID(d.Purchase.Supplier.DIC), File: m.Name, MessageID: m.ID, Status: "prevzatá",
			Created: a.now(), User: auth.ActorFrom(ctx).Name, Data: m.Data}
		if err := a.Store.AddEInvoice(ctx, companyID, &e); err != nil {
			return PurchaseDraft{}, err
		}
		if err := p.Ack(ctx, m.ID); err != nil {
			d.Warnings = append(d.Warnings, "Poštárovi sa nepodarilo potvrdiť prevzatie ("+err.Error()+") — e-faktúra môže zostať v schránke")
		}
		return d, nil
	}
	return PurchaseDraft{}, store.ErrNotFound
}

// ReopenEInvoice otvorí znova prijatú e-faktúru zo zoznamu (ak koncept nebol uložený).
func (a *App) ReopenEInvoice(ctx context.Context, companyID, id int64) (PurchaseDraft, error) {
	if err := a.Require(ctx, companyID, auth.PermPurchasesWrite); err != nil {
		return PurchaseDraft{}, err
	}
	e, err := a.Store.GetEInvoice(ctx, companyID, id)
	if err != nil {
		return PurchaseDraft{}, err
	}
	if e.Direction != "in" || len(e.Data) == 0 {
		return PurchaseDraft{}, store.ErrNotFound
	}
	return a.ImportUBL(ctx, companyID, strings.TrimSpace(e.File), e.Data)
}

// finalDelivery: stav, ktorý sa už nezmení (netreba sa ďalej pýtať).
func finalDelivery(status string) bool {
	switch status {
	case "delivered", "failed", "rejected", "undeliverable", "nedoručiteľná":
		return true
	}
	return false
}

// trackEInvoice zistí stav jednej odoslanej e-faktúry a uloží ho.
func (a *App) trackEInvoice(ctx context.Context, companyID int64, e store.EInvoice) (store.EInvoice, error) {
	if e.Direction != "out" || e.MessageID == "" || e.Error != "" {
		return e, ErrNoTrack
	}
	p, err := a.provider(ctx, companyID)
	if err != nil {
		return e, err
	}
	t, ok := p.(postman.Tracker)
	if !ok {
		return e, postman.ErrNoTracking
	}
	d, err := t.Delivery(ctx, e.MessageID)
	if err != nil {
		return e, err
	}
	if d.Status == e.Status && d.Delivered == e.Delivered {
		return e, nil
	}
	if err := a.Store.SetEInvoiceStatus(ctx, companyID, e.ID, d.Status, d.Delivered); err != nil {
		return e, err
	}
	e.Status, e.Delivered = d.Status, d.Delivered
	if finalDelivery(d.Status) && d.Status != "delivered" {
		msg := "E-faktúra " + e.DocNumber + " nebola doručená (" + d.Status + ")"
		if d.Error != "" {
			msg += ": " + d.Error
		}
		a.record(ctx, companyID, audit.CatInvoices, "einvoice", "invoice", e.DocID, msg, nil, e)
	}
	return e, nil
}

// ErrNoTrack — záznam nie je odoslaná e-faktúra (stav sa nezisťuje).
var ErrNoTrack = errors.New("stav doručenia sa zisťuje iba pri odoslaných e-faktúrach")

// TrackEInvoice zistí u poštára stav doručenia odoslanej e-faktúry.
func (a *App) TrackEInvoice(ctx context.Context, companyID, id int64) (store.EInvoice, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return store.EInvoice{}, err
	}
	e, err := a.Store.GetEInvoice(ctx, companyID, id)
	if err != nil {
		return store.EInvoice{}, err
	}
	e.Data = nil
	return a.trackEInvoice(ctx, companyID, e)
}

// TrackAllEInvoices zistí stav nedoručených e-faktúr odoslaných za posledných
// 30 dní vo všetkých firmách (beží každú hodinu na pozadí).
func (a *App) TrackAllEInvoices(ctx context.Context) {
	ctx = auth.WithActor(ctx, auth.System)
	companies, err := a.Store.ListCompanies(ctx)
	if err != nil {
		return
	}
	since := a.now().AddDate(0, 0, -30)
	for _, co := range companies {
		s, err := a.loadEInvoice(ctx, co.ID)
		if err != nil || s.Mode != postman.ModeSAPI {
			continue
		}
		list, err := a.Store.ListEInvoices(ctx, co.ID, store.EInvoiceFilter{Direction: "out"})
		if err != nil {
			continue
		}
		for _, e := range list {
			if e.Created.Before(since) || finalDelivery(e.Status) || e.MessageID == "" || e.Error != "" {
				continue
			}
			if _, err := a.trackEInvoice(ctx, co.ID, e); err != nil {
				if !errors.Is(err, postman.ErrNoTracking) {
					log.Printf("stav e-faktúry %s (firma %d): %v", e.DocNumber, co.ID, err)
				}
				break // poskytovateľ stav nevie alebo neodpovedá — ďalšie e-faktúry tejto firmy nabudúce
			}
		}
	}
}
