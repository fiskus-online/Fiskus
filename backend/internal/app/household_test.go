package app

import (
	"errors"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/cash"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/household"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/journal"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func TestHouseholdCompany(t *testing.T) {
	a, firm := newApp(t, edition.Desktop)
	h, err := a.CreateCompany(ctx, store.Company{Name: "Domácnosť Novákovci", Bookkeeping: store.BookkeepingHousehold})
	if err != nil || !h.SetupDone {
		t.Fatalf("domácnosť: %+v %v", h, err)
	}
	cases := []struct {
		name string
		c    store.Company
	}{
		{"domácnosť na firmu", func() store.Company { c := h; c.Bookkeeping = store.BookkeepingSimple; return c }()},
		{"firma na domácnosť", func() store.Company { c := firm; c.Bookkeeping = store.BookkeepingHousehold; return c }()},
		{"domácnosť platiteľ DPH", func() store.Company { c := h; c.VATPayer, c.ICDPH = true, "SK2020372640"; return c }()},
	}
	for _, c := range cases {
		if _, err := a.UpdateCompany(ctx, c.c); err == nil {
			t.Errorf("%s: chcem chybu", c.name)
		}
	}
	// operácie domácnosti nad firmou
	if _, err := a.HouseholdAccounts(ctx, firm.ID); !errors.Is(err, ErrNotHousehold) {
		t.Fatalf("účty domácnosti vo firme: %v", err)
	}
}

func TestHouseholdEntries(t *testing.T) {
	a, firm := newApp(t, edition.Desktop)
	h, err := a.CreateCompany(ctx, store.Company{Name: "Domácnosť", Bookkeeping: store.BookkeepingHousehold})
	if err != nil {
		t.Fatal(err)
	}
	bank, err := a.SaveHouseholdAccount(ctx, h.ID, household.Account{Name: "Účet", Kind: household.Bank, Opening: m.MustParse("1000")})
	if err != nil {
		t.Fatal(err)
	}
	wallet, _ := a.SaveHouseholdAccount(ctx, h.ID, household.Account{Name: "Peňaženka", Kind: household.Cash})
	if _, err := a.SaveHouseholdAccount(ctx, h.ID, household.Account{Name: "účet"}); !fieldErr(err, "name") {
		t.Fatalf("rovnaký názov účtu: %v", err)
	}
	save := func(e household.Entry) (household.Entry, error) { return a.SaveHouseholdEntry(ctx, h.ID, e) }
	if _, err := save(household.Entry{Date: invoice.MustDate("2026-10-01"), AccountID: bank.ID, Amount: m.MustParse("-10"), Category: "Neexistuje"}); !fieldErr(err, "category") {
		t.Fatalf("neznáma kategória: %v", err)
	}
	if _, err := save(household.Entry{Date: invoice.MustDate("2026-10-01"), AccountID: 999, Amount: m.MustParse("-10"), Category: "Potraviny"}); !fieldErr(err, "accountId") {
		t.Fatalf("neznámy účet: %v", err)
	}
	if _, err := save(household.Entry{Date: invoice.MustDate("2026-10-02"), AccountID: bank.ID, Amount: m.MustParse("-62.40"), Category: "Potraviny", Payee: "Lidl"}); err != nil {
		t.Fatal(err)
	}
	if _, err := save(household.Entry{Date: invoice.MustDate("2026-10-03"), AccountID: bank.ID, ToAccountID: wallet.ID, Amount: m.MustParse("100")}); err != nil {
		t.Fatal(err)
	}
	if _, err := save(household.Entry{Date: invoice.MustDate("2026-10-04"), AccountID: wallet.ID, Amount: m.MustParse("-8"), Category: "Reštaurácie"}); err != nil {
		t.Fatal(err)
	}
	// rozpočet potravín 50 € → prečerpaný
	cats, _ := a.HouseholdCategories(ctx, h.ID)
	for i := range cats {
		if cats[i].Name == "Potraviny" {
			cats[i].Budget = m.MustParse("50")
		}
	}
	if _, err := a.SaveHouseholdCategories(ctx, h.ID, cats); err != nil {
		t.Fatal(err)
	}
	bad := []struct {
		name string
		cats []household.Category
	}{
		{"bez príjmov", []household.Category{{Name: "A"}}},
		{"duplicitná", []household.Category{{Name: "A", Income: true}, {Name: "B"}, {Name: "b"}}},
		{"záporný rozpočet", []household.Category{{Name: "A", Income: true}, {Name: "B", Budget: m.MustParse("-1")}}},
		{"prázdny názov", []household.Category{{Name: "A", Income: true}, {Name: " "}}},
	}
	for _, c := range bad {
		if _, err := a.SaveHouseholdCategories(ctx, h.ID, c.cats); err == nil {
			t.Errorf("%s: chcem chybu", c.name)
		}
	}
	ov, err := a.HouseholdSummary(ctx, h.ID, 2026, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ov.Month != 10 || !ov.TotalOut.Equal(m.MustParse("70.40")) || !ov.Total.Equal(m.MustParse("929.60")) {
		t.Fatalf("prehľad: mesiac %d výdavky %v spolu %v", ov.Month, ov.TotalOut, ov.Total)
	}
	over := false
	for _, b := range ov.Budget {
		over = over || (b.Name == "Potraviny" && b.Over)
	}
	if !over {
		t.Fatalf("rozpočet: %+v", ov.Budget)
	}
	if err := a.DeleteHouseholdAccount(ctx, h.ID, wallet.ID); !fieldErr(err, "id") {
		t.Fatalf("zmazanie účtu s pohybmi: %v", err)
	}
	if got, _ := a.HouseholdSuggest(ctx, h.ID, "LIDL SK", false); got != "Potraviny" {
		t.Fatalf("návrh kategórie: %q", got)
	}

	// výber zo živnosti: v peňažnom denníku výdavok mimo základu dane
	in, _ := a.CreateCash(ctx, firm.ID, cash.Document{Kind: cash.In, Date: invoice.MustDate("2026-10-01"), Amount: m.MustParse("900"), Purpose: "Tržba"})
	out, err := a.CreateCash(ctx, firm.ID, cash.Document{Kind: cash.Out, Date: invoice.MustDate("2026-10-05"), Amount: m.MustParse("500"), Purpose: "Výber podnikateľa"})
	if err != nil {
		t.Fatal(err)
	}
	_ = in
	key := "cash:" + itoa(int(out.ID))
	if err := a.ClassifyJournal(ctx, firm.ID, key, []journal.Line{{Column: journal.ExpNonTax, Amount: m.MustParse("500")}}, ""); err != nil {
		t.Fatal(err)
	}
	draws, err := a.BusinessDraws(ctx, h.ID, 2026)
	if err != nil || len(draws) != 1 || !draws[0].Amount.Equal(m.MustParse("500")) || draws[0].Imported {
		t.Fatalf("výbery zo živnosti: %+v %v", draws, err)
	}
	res, err := a.ImportBusinessDraws(ctx, h.ID, wallet.ID, 2026, []DrawRef{{CompanyID: firm.ID, Key: key}})
	if err != nil || res.Added != 1 || res.Entries[0].Category != "Mzda a príjem zo živnosti" {
		t.Fatalf("prenos zo živnosti: %+v %v", res, err)
	}
	res, _ = a.ImportBusinessDraws(ctx, h.ID, wallet.ID, 2026, []DrawRef{{CompanyID: firm.ID, Key: key}})
	if res.Added != 0 || res.Skipped != 1 {
		t.Fatalf("druhý prenos: %+v", res)
	}
	if draws, _ := a.BusinessDraws(ctx, h.ID, 2026); !draws[0].Imported {
		t.Fatal("prenesený výber nie je označený")
	}
}

func TestHouseholdBankImport(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	h, _ := a.CreateCompany(ctx, store.Company{Name: "Domácnosť", Bookkeeping: store.BookkeepingHousehold})
	acc, _ := a.SaveHouseholdAccount(ctx, h.ID, household.Account{Name: "Účet", IBAN: "SK31 1200 0000 1987 4263 7541"})
	// partner v adresári: pohyb z banky s rovnakým menom sa k nemu priradí
	boss, err := a.SaveContact(ctx, h.ID, store.Contact{Name: "Zamestnávateľ s.r.o."})
	if err != nil {
		t.Fatal(err)
	}
	csv := "Dátum;Suma;Mena;Protistrana;Správa\n" +
		"02.10.2026;-45,10;EUR;LIDL SR;nakup\n" +
		"03.10.2026;1800,00;EUR;Zamestnavatel s.r.o.;mzda 09\n" +
		"04.10.2026;-12,00;CZK;Shop CZ;\n"
	res, err := a.ImportHouseholdBank(ctx, h.ID, acc.ID, []byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 2 || res.Foreign != 1 || res.Entries[0].Category != "Potraviny" || res.Entries[1].Category != "Iné príjmy" {
		t.Fatalf("import: %+v", res)
	}
	if res.Entries[0].ContactID != 0 || res.Entries[1].ContactID != boss.ID {
		t.Fatalf("partneri: %d %d", res.Entries[0].ContactID, res.Entries[1].ContactID)
	}
	res, err = a.ImportHouseholdBank(ctx, h.ID, acc.ID, []byte(csv))
	if err != nil || res.Added != 0 || res.Skipped != 2 {
		t.Fatalf("opakovaný import: %+v %v", res, err)
	}
}
