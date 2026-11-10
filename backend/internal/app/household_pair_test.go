package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/ekasa"
	"github.com/fiskus-online/fiskus/backend/internal/household"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// bloček z eKasy ako ho zapíše mobil (účet Hotovosť)
func receiptEntry(id, date, shop, total string, cashID int64) household.Entry {
	amt := money.MustParse(total)
	return household.Entry{Date: invoice.MustDate(date), AccountID: cashID, Amount: amt.Neg(), Category: "Potraviny", Payee: shop,
		Source:  "ekasa:" + id,
		Split:   []household.Split{{Category: "Potraviny", Amount: amt.Neg().Add(money.MustParse("2.00")), Note: "chlieb"}, {Category: "Drogéria a domácnosť", Amount: money.MustParse("-2.00"), Note: "mydlo"}},
		Receipt: &ekasa.Receipt{ID: id, Seller: ekasa.Seller{Name: shop}, Total: amt}}
}

func TestHouseholdReceiptPairing(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	h, _ := a.CreateCompany(ctx, store.Company{Name: "Domácnosť", Bookkeeping: store.BookkeepingHousehold})
	accs, _ := a.HouseholdAccounts(ctx, h.ID)
	cash := accs[0].ID
	bank, _ := a.SaveHouseholdAccount(ctx, h.ID, household.Account{Name: "Fio", Kind: household.Bank})

	// bloček skôr, platba z banky neskôr (o deň)
	lidl, err := a.SaveHouseholdEntry(ctx, h.ID, receiptEntry("L1", "2026-10-01", "LIDL Slovenská republika, v.o.s.", "45.10", cash))
	if err != nil || lidl.Paired != "" {
		t.Fatalf("bloček: %+v %v", lidl, err)
	}
	// dva rovnaké bločky v jeden deň v rôznych obchodoch, platba bez názvu obchodu → nevieme ktorý
	_, _ = a.SaveHouseholdEntry(ctx, h.ID, receiptEntry("A1", "2026-10-02", "Kaufland", "9.99", cash))
	_, _ = a.SaveHouseholdEntry(ctx, h.ID, receiptEntry("A2", "2026-10-02", "Tesco", "9.99", cash))
	// zaplatený v hotovosti, v banke nič
	_, _ = a.SaveHouseholdEntry(ctx, h.ID, receiptEntry("C1", "2026-10-03", "Pekáreň", "3.20", cash))

	csv := "Dátum;Suma;Mena;Protistrana;Správa\n" +
		"02.10.2026;-45,10;EUR;;Nákup: LIDL SK 0123, Banská Bystrica\n" +
		"02.10.2026;-9,99;EUR;;Platba kartou\n" +
		"10.10.2026;-45,10;EUR;LIDL SR;neskôr ako 4 dni → nový pohyb\n" +
		"03.10.2026;1800,00;EUR;Zamestnavatel s.r.o.;mzda\n"
	res, err := a.ImportHouseholdBank(ctx, h.ID, bank.ID, []byte(csv))
	if err != nil || res.Paired != 1 || res.Added != 3 {
		t.Fatalf("import: %+v %v", res, err)
	}
	got, _ := a.Store.GetHHEntry(ctx, h.ID, lidl.ID)
	if got.AccountID != bank.ID || got.Paired == "" || got.Receipt == nil || len(got.Split) != 2 || got.Source != "ekasa:L1" {
		t.Fatalf("spárovaný bloček: %+v", got)
	}
	again, err := a.ImportHouseholdBank(ctx, h.ID, bank.ID, []byte(csv))
	if err != nil || again.Added != 0 || again.Paired != 0 || again.Skipped != 4 {
		t.Fatalf("opakovaný import: %+v %v", again, err)
	}

	// platba z banky skôr, bloček až potom: bloček nahradí platbu
	if _, err := a.ImportHouseholdBank(ctx, h.ID, bank.ID, []byte("Dátum;Suma;Mena;Protistrana;Správa\n06.10.2026;-12,90;EUR;BILLA s.r.o.;karta\n")); err != nil {
		t.Fatal(err)
	}
	before, _ := a.Store.ListHHEntries(ctx, h.ID, 0)
	billa, err := a.SaveHouseholdEntry(ctx, h.ID, receiptEntry("B1", "2026-10-05", "BILLA s.r.o.", "12.90", cash))
	if err != nil || billa.AccountID != bank.ID || billa.Paired == "" {
		t.Fatalf("bloček k platbe: %+v %v", billa, err)
	}
	after, _ := a.Store.ListHHEntries(ctx, h.ID, 0)
	if len(after) != len(before) {
		t.Fatalf("platba z banky sa nenahradila: %d → %d", len(before), len(after))
	}
	// ten istý výpis znova: platba je v bločku, nevznikne druhýkrát
	res, _ = a.ImportHouseholdBank(ctx, h.ID, bank.ID, []byte("Dátum;Suma;Mena;Protistrana;Správa\n06.10.2026;-12,90;EUR;BILLA s.r.o.;karta\n"))
	if res.Added != 0 || res.Skipped != 1 {
		t.Fatalf("po spárovaní: %+v", res)
	}
	// úprava bločku párovanie neruší
	billa.Note = "nákup"
	if billa, err = a.SaveHouseholdEntry(ctx, h.ID, billa); err != nil || billa.Paired == "" {
		t.Fatalf("úprava: %+v %v", billa, err)
	}
}

func TestPairScore(t *testing.T) {
	r := household.Entry{Date: invoice.MustDate("2026-10-01"), Amount: money.MustParse("-10.00"), Payee: "Kaufland Slovenská republika v.o.s.",
		Receipt: &ekasa.Receipt{Seller: ekasa.Seller{Name: "Kaufland Slovenská republika v.o.s."}}}
	for _, tc := range []struct {
		name, date, amount, text string
		want                     int
	}{
		{"v ten deň s obchodom", "2026-10-01", "-10.00", "KAUFLAND BB", 14},
		{"o 2 dni bez obchodu", "2026-10-03", "-10.00", "platba kartou", 2},
		{"iná suma", "2026-10-01", "-10.01", "Kaufland", -1},
		{"pred nákupom", "2026-09-30", "-10.00", "Kaufland", -1},
		{"o 5 dní", "2026-10-06", "-10.00", "Kaufland", -1},
		{"slovensko nestačí", "2026-10-01", "-10.00", "Lidl Slovenská republika", 4},
	} {
		p := household.Entry{Date: invoice.MustDate(tc.date), Amount: money.MustParse(tc.amount), Note: tc.text}
		if got := pairScore(r, p); got != tc.want {
			t.Errorf("%s: %d, chcem %d", tc.name, got, tc.want)
		}
	}
	if best([]int{-1, 3, 3}) != -1 || best([]int{2, 14, -1}) != 1 || best(nil) != -1 {
		t.Fatal("best")
	}
}

func TestHouseholdFio(t *testing.T) {
	data, err := os.ReadFile("../bank/testdata/fio.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(data) }))
	defer srv.Close()
	a, _ := newApp(t, edition.Desktop)
	a.FioURL = srv.URL
	h, _ := a.CreateCompany(ctx, store.Company{Name: "Domácnosť", Bookkeeping: store.BookkeepingHousehold})
	bank, _ := a.SaveHouseholdAccount(ctx, h.ID, household.Account{Name: "Fio", Kind: household.Bank})

	if _, err := a.SaveFioAccount(ctx, h.ID, FioAccountInput{Label: "Fio", Token: fioToken}); !fieldErr(err, "householdAccountId") {
		t.Fatalf("bez účtu domácnosti: %v", err)
	}
	list, err := a.SaveFioAccount(ctx, h.ID, FioAccountInput{Label: "Fio", Token: fioToken, HHAccount: bank.ID})
	if err != nil || list[0].HHAccount != bank.ID {
		t.Fatalf("napojenie: %+v %v", list, err)
	}
	res, err := a.SyncFio(ctx, h.ID, list[0].ID)
	if err != nil || res.Imported != 3 || res.Format != "Fio API" {
		t.Fatalf("stiahnutie: %+v %v", res, err)
	}
	es, _ := a.Store.ListHHEntries(ctx, h.ID, 0)
	n := 0
	for _, e := range es {
		if e.AccountID == bank.ID && e.Category != "" {
			n++
		}
	}
	if n != 3 {
		t.Fatalf("pohyby domácnosti: %d", n)
	}
	// firemná banka sa nezmenila
	if txs, _ := a.Store.ListBankTransactions(ctx, h.ID, store.BankFilter{}); len(txs) != 0 {
		t.Fatalf("pohyby vo firemnej banke: %d", len(txs))
	}
}
