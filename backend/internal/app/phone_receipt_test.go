package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/household"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func TestSavePhoneReceipt(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, groceryJSON)
	}))
	defer srv.Close()
	a.EKasaURL = srv.URL
	h, _ := a.CreateCompany(ctx, store.Company{Name: "Domácnosť", Bookkeeping: store.BookkeepingHousehold})
	bank, _ := a.SaveHouseholdAccount(ctx, h.ID, household.Account{Name: "Účet", Kind: household.Bank})
	firm, _ := a.CreateCompany(ctx, store.Company{Name: "Firma"})
	const qr = "O-11112222333344445555666677778888"
	jpeg := []byte{0xff, 0xd8, 0xff, 0xe0}

	cases := []struct {
		name    string
		company int64
		in      PhoneReceipt
		field   string // očakávaná chyba poľa
	}{
		{"domácnosť bez kódu a fotky", h.ID, PhoneReceipt{}, "photo"},
		{"firma bez kódu", firm.ID, PhoneReceipt{Photo: jpeg}, "qr"},
		{"priveľká fotka", h.ID, PhoneReceipt{Photo: make([]byte, MaxReceiptPhoto+1)}, "photo"},
	}
	for _, c := range cases {
		if _, err := a.SavePhoneReceipt(ctx, c.company, c.in); !fieldErr(err, c.field) {
			t.Errorf("%s: %v", c.name, err)
		}
	}

	// domácnosť: bloček z eKasy na zvolený účet, s fotkou
	r, err := a.SavePhoneReceipt(ctx, h.ID, PhoneReceipt{QR: qr, AccountID: bank.ID, Photo: jpeg})
	if err != nil || r.ID == 0 || r.Title != "Lidl Slovenská republika, v.o.s." {
		t.Fatalf("domácnosť: %+v %v", r, err)
	}
	e, err := a.Store.GetHHEntry(ctx, h.ID, r.ID)
	if err != nil || e.AccountID != bank.ID || len(e.Split) != 5 {
		t.Fatalf("pohyb: %+v %v", e, err)
	}
	if p, err := a.HouseholdPhoto(ctx, h.ID, r.ID); err != nil || len(p.Data) != len(jpeg) {
		t.Fatalf("fotka pohybu: %v", err)
	}

	// firma: prijatá faktúra z eKasy
	r, err = a.SavePhoneReceipt(ctx, firm.ID, PhoneReceipt{QR: qr, Payment: "card", Photo: jpeg})
	if err != nil || r.ID == 0 {
		t.Fatalf("firma: %+v %v", r, err)
	}
	p, err := a.Store.GetPurchase(ctx, firm.ID, r.ID)
	if err != nil || p.Supplier.Name == "" {
		t.Fatalf("prijatá faktúra: %+v %v", p, err)
	}
}
