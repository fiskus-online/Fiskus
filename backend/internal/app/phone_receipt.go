package app

import (
	"context"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/ekasa"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// PhoneReceipt je bloček odfotený v telefóne mimo domu (sync.go). Telefón
// ho nespracuje — eKasa ani čítanie fotky cez AI vo Fiškuse v prehliadači
// nejdú —, iba ho pošle do fronty a spracuje ho počítač rovnako ako bloček
// z mobilu v sieti.
type PhoneReceipt struct {
	QR        string `json:"qr"`                  // kód bločku eKasa (O-…), ak sa prečítal
	Payment   string `json:"payment,omitempty"`   // firma: cash | card
	AccountID int64  `json:"accountId,omitempty"` // domácnosť: účet, z ktorého sa platilo
	Photo     []byte `json:"photo,omitempty"`     // JPEG (v JSON base64)
}

// PhoneReceiptResult: čo z bločku v počítači vzniklo.
type PhoneReceiptResult struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// SavePhoneReceipt spracuje bloček z telefónu: vo firme prijatá faktúra
// z eKasy (ScanReceipt), v domácnosti pohyb rozdelený na položky — z eKasy,
// keď je kód, inak z fotky cez AI; fotka sa priloží.
func (a *App) SavePhoneReceipt(ctx context.Context, companyID int64, in PhoneReceipt) (PhoneReceiptResult, error) {
	if len(in.Photo) > MaxReceiptPhoto {
		return PhoneReceiptResult{}, vErr("photo", "Fotka je príliš veľká (najviac 3 MB)")
	}
	c, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return PhoneReceiptResult{}, err
	}
	qr := strings.TrimSpace(in.QR)
	if c.Bookkeeping != store.BookkeepingHousehold {
		if qr == "" {
			return PhoneReceiptResult{}, vErr("qr", "Na fotke sa nenašiel QR kód bločku")
		}
		scan := ReceiptScan{QR: qr, Payment: invoice.PaymentMethod(in.Payment), Photo: in.Photo}
		if len(in.Photo) > 0 {
			scan.PhotoType = "image/jpeg"
		}
		r, err := a.ScanReceipt(ctx, companyID, scan)
		if err != nil {
			return PhoneReceiptResult{}, err
		}
		return PhoneReceiptResult{ID: r.Purchase.ID, Title: r.Purchase.Supplier.Name}, nil
	}
	var d ReceiptDraft
	if _, perr := ekasa.ParseQR(qr); qr != "" && perr == nil {
		d, err = a.HouseholdReceiptQR(ctx, companyID, qr)
	} else if len(in.Photo) > 0 {
		d, err = a.HouseholdReceiptPhoto(ctx, companyID, in.Photo)
	} else {
		return PhoneReceiptResult{}, vErr("photo", "Chýba fotka bločku")
	}
	if err != nil {
		return PhoneReceiptResult{}, err
	}
	if in.AccountID > 0 {
		d.Entry.AccountID = in.AccountID
	}
	saved, err := a.SaveHouseholdEntry(ctx, companyID, d.Entry)
	if err != nil {
		return PhoneReceiptResult{}, err
	}
	if len(in.Photo) > 0 {
		_ = a.SaveHouseholdPhoto(ctx, companyID, saved.ID, "image/jpeg", in.Photo) // pohyb je hlavný, fotka navyše
	}
	return PhoneReceiptResult{ID: saved.ID, Title: saved.Payee}, nil
}
