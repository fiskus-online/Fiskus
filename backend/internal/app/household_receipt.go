package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/ekasa"
	"github.com/palo-krnac/fiscus/backend/internal/extract"
	"github.com/palo-krnac/fiscus/backend/internal/household"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Bloček v domácnosti: z QR kódu eKasa (položky z Finančnej správy) alebo
// z fotky (Claude API) vznikne návrh pohybu rozdeleného na položky
// s kategóriami. Návrh sa neukladá — používateľ ho skontroluje, kategórie
// položiek upraví a uloží ako pohyb (s fotkou).

// ReceiptDraft je návrh pohybu z bločku.
type ReceiptDraft struct {
	Entry    household.Entry `json:"entry"` // Amount a Split záporné (výdavok), Source na kontrolu duplicity
	From     string          `json:"from"`  // "ekasa" | "photo"
	Warnings []string        `json:"warnings"`
}

// receiptDraft doplní kategórie položiek a zostaví pohyb.
func (a *App) receiptDraft(ctx context.Context, companyID int64, date invoice.Date, shop string, total money.Dec, items []household.ReceiptItem, source string) (ReceiptDraft, error) {
	s, err := a.householdSettings(ctx, companyID)
	if err != nil {
		return ReceiptDraft{}, err
	}
	hist, err := a.Store.ListHHEntries(ctx, companyID, 0)
	if err != nil {
		return ReceiptDraft{}, err
	}
	for _, e := range hist {
		if source != "" && e.Source == source {
			return ReceiptDraft{}, vErr("qr", fmt.Sprintf("Tento bloček už je zapísaný (%s, %s €)", e.Date.Display(), e.Amount.Neg().Display()))
		}
	}
	have := map[string]bool{}
	for _, c := range s.Categories {
		if !c.Income {
			have[c.Name] = true
		}
	}
	shopCat := household.Suggest(shop, false, hist, s.Categories)
	if shopCat == "" {
		shopCat = fallbackCategory(s.Categories, false)
	}
	for i := range items {
		it := &items[i]
		if !have[it.Category] {
			it.Category = ""
		}
		if c := household.SuggestItem(it.Name, hist, s.Categories); c != "" && (it.Category == "" || hasItemHistory(it.Name, hist)) {
			it.Category = c // história má prednosť aj pred návrhom AI
		}
	}
	split, amt := household.ReceiptSplit(items, total, shopCat)
	d := ReceiptDraft{Entry: household.Entry{Date: date, Amount: amt, Payee: shop, Split: split, Source: source}, Warnings: []string{}}
	d.Entry.Normalize()
	if len(split) == 0 {
		d.Entry.Category = shopCat
		d.Warnings = append(d.Warnings, "Na bločku sa nenašli položky — zapíše sa celá suma")
	}
	for _, x := range split {
		if strings.HasPrefix(x.Note, "Rozdiel") {
			d.Warnings = append(d.Warnings, "Súčet položiek nesedí s celkovou sumou — rozdiel je samostatná položka, skontrolujte ju")
		}
	}
	if date.IsZero() {
		d.Entry.Date = a.Today()
		d.Warnings = append(d.Warnings, "Dátum nákupu sa nenašiel — je nastavený dnešný")
	}
	// predvolený účet: hotovosť, inak prvý otvorený
	if accs, err := a.Store.ListHHAccounts(ctx, companyID); err == nil {
		for _, x := range accs {
			if x.Closed {
				continue
			}
			if d.Entry.AccountID == 0 {
				d.Entry.AccountID = x.ID
			}
			if x.Kind == household.Cash {
				d.Entry.AccountID = x.ID
				break
			}
		}
	}
	return d, nil
}

func hasItemHistory(name string, hist []household.Entry) bool {
	k := strings.ToLower(strings.TrimSpace(name))
	for _, e := range hist {
		for _, x := range e.Split {
			if strings.ToLower(strings.TrimSpace(x.Note)) == k {
				return true
			}
		}
	}
	return false
}

// HouseholdReceiptQR načíta bloček eKasa podľa QR kódu a vráti návrh pohybu.
func (a *App) HouseholdReceiptQR(ctx context.Context, companyID int64, qr string) (ReceiptDraft, error) {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return ReceiptDraft{}, err
	}
	id, err := ekasa.ParseQR(qr)
	if err != nil {
		return ReceiptDraft{}, vErr("qr", err.Error())
	}
	r, err := a.ekasaClient().Find(ctx, id)
	switch {
	case errors.Is(err, ekasa.ErrNotFound):
		return ReceiptDraft{}, vErr("qr", err.Error())
	case err != nil:
		return ReceiptDraft{}, &EshopError{err} // služba nedostupná → 502 so správou
	}
	items := make([]household.ReceiptItem, 0, len(r.Items))
	for _, it := range r.Items {
		q := ""
		if !it.Quantity.IsZero() && !it.Quantity.Equal(money.FromInt(1)) {
			q = strings.TrimRight(strings.TrimRight(strings.ReplaceAll(it.Quantity.String(), ".", ","), "0"), ",")
		}
		items = append(items, household.ReceiptItem{Name: it.Name, Quantity: q, Amount: it.Price})
	}
	var date invoice.Date
	if !r.Issued.IsZero() {
		date = invoice.NewDate(r.Issued.Year(), r.Issued.Month(), r.Issued.Day())
	}
	d, err := a.receiptDraft(ctx, companyID, date, r.Seller.Name, r.Total, items, "ekasa:"+id)
	d.From = "ekasa"
	return d, err
}

// HouseholdReceiptPhoto prečíta bloček z fotky cez Claude API a vráti návrh
// pohybu; model zaradí položky do kategórií domácnosti.
func (a *App) HouseholdReceiptPhoto(ctx context.Context, companyID int64, photo []byte) (ReceiptDraft, error) {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return ReceiptDraft{}, err
	}
	if len(photo) > MaxReceiptPhoto {
		return ReceiptDraft{}, vErr("photo", "Fotka je príliš veľká (najviac 3 MB)")
	}
	ai, err := a.loadAI(ctx)
	if err != nil {
		return ReceiptDraft{}, err
	}
	s, err := a.householdSettings(ctx, companyID)
	if err != nil {
		return ReceiptDraft{}, err
	}
	names := []string{}
	for _, c := range s.Categories {
		if !c.Income {
			names = append(names, c.Name)
		}
	}
	r, err := extract.Client{Settings: ai, BaseURL: a.AIBaseURL}.ExtractReceipt(ctx, photo, names)
	if errors.Is(err, extract.ErrOff) {
		return ReceiptDraft{}, vErr("photo", "Čítanie bločkov z fotky potrebuje kľúč Claude API (Nastavenia → Prístup pre AI). Bloček s QR kódom eKasa sa načíta aj bez neho.")
	}
	if err != nil {
		return ReceiptDraft{}, vErr("photo", err.Error())
	}
	items := make([]household.ReceiptItem, 0, len(r.Items))
	for _, it := range r.Items {
		amt, ok := extract.Num(it.Amount)
		if !ok {
			continue
		}
		items = append(items, household.ReceiptItem{Name: it.Name, Quantity: strings.TrimSpace(it.Quantity), Amount: amt, Category: it.Category})
	}
	total, _ := extract.Num(r.Total)
	date, _ := invoice.ParseDate(strings.TrimSpace(r.Date))
	sum := sha256.Sum256(photo)
	d, err := a.receiptDraft(ctx, companyID, date, strings.TrimSpace(r.Store), total, items, "photo:"+hex.EncodeToString(sum[:8]))
	d.From = "photo"
	if err == nil {
		a.record(ctx, companyID, audit.CatAI, "extract", "household_receipt", 0,
			fmt.Sprintf("Domácnosť: prečítaný bloček %s, %s €, %d položiek", d.Entry.Payee, d.Entry.Amount.Neg().Display(), len(d.Entry.Split)), nil, nil)
	}
	return d, err
}

// SaveHouseholdPhoto uloží fotku bločku k pohybu.
func (a *App) SaveHouseholdPhoto(ctx context.Context, companyID, entryID int64, contentType string, data []byte) error {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return err
	}
	if len(data) == 0 || len(data) > MaxReceiptPhoto {
		return vErr("photo", "Fotka je prázdna alebo príliš veľká (najviac 3 MB)")
	}
	if photoTypes[contentType] == "" {
		return vErr("photo", "Fotka musí byť JPEG, PNG alebo WebP")
	}
	return a.Store.SaveHHPhoto(ctx, store.HHPhoto{CompanyID: companyID, EntryID: entryID, ContentType: contentType, Data: data})
}

// HouseholdPhoto vráti fotku bločku pohybu.
func (a *App) HouseholdPhoto(ctx context.Context, companyID, entryID int64) (store.HHPhoto, error) {
	if err := a.requireHousehold(ctx, companyID, auth.PermLedgerRead); err != nil {
		return store.HHPhoto{}, err
	}
	return a.Store.GetHHPhoto(ctx, companyID, entryID)
}
