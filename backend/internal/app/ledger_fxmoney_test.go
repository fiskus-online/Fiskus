package app

import (
	"strings"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/bank"
	"github.com/fiskus-online/fiskus/backend/internal/cash"
	"github.com/fiskus-online/fiskus/backend/internal/chart"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/fx"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/ledger"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
)

func TestFXRevaluationMoney(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	if err := a.Store.SaveRates(ctx, []fx.Rate{rate("USD", "2026-03-02", "1.1"), rate("USD", "2026-04-01", "1.25")}); err != nil {
		t.Fatal(err)
	}
	regs, err := a.SaveCashRegisters(ctx, c.ID, []cash.Register{{Name: "Pokladnica"}, {ID: -1, Name: "Pokladnica USD", Currency: "USD", Prefix: "US"}})
	if err != nil {
		t.Fatal(err)
	}
	var usd int64
	for _, r := range regs {
		if r.Currency == "USD" {
			usd = r.ID
		}
	}
	// 1 100 USD kurzom 1,10 = 1 000 €
	cd, err := a.CreateCash(ctx, c.ID, cash.Document{Kind: cash.In, RegisterID: usd, Date: invoice.MustDate("2026-03-03"), Amount: m.MustParse("1100"), Purpose: "Vklad USD"})
	if err != nil || cd.AmountEUR.String() != "1000.00" {
		t.Fatalf("doklad USD: %+v %v", cd, err)
	}
	// bankový účet v USD: 500 USD pripísaných, zaúčtovaných ručne v eurách (400 €)
	const iban = "SK6911000000002600000999"
	for _, n := range []string{"221.002", "211.002"} {
		if _, err := a.SaveAccount(ctx, c.ID, "", chart.Account{Number: n, Name: "USD " + n, Active: true}); err != nil {
			t.Fatal(err)
		}
	}
	txs, _, err := a.Store.SaveBankTransactions(ctx, c.ID, []bank.Transaction{{Account: iban, Date: invoice.MustDate("2026-04-02"), Amount: m.MustParse("500"),
		Currency: "USD", Direction: bank.In, Message: "Úhrada", CounterName: "ACME Inc.", Fingerprint: "u1", Status: bank.StatusOpen}})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.SaveRates(ctx, []fx.Rate{rate("USD", "2026-12-31", "1.2")}); err != nil {
		t.Fatal(err)
	}

	warn := func(rv FXRevaluation, s string) bool {
		for _, w := range rv.Warnings {
			if strings.Contains(w, s) {
				return true
			}
		}
		return false
	}
	rv, err := a.FXRevaluationFor(ctx, c.ID, 2026)
	if err != nil || !warn(rv, "nemá priradený analytický účet") {
		t.Fatalf("bez analytického účtu: %+v %v", rv.Warnings, err)
	}
	if err := a.SetMoneyAccounts(ctx, c.ID, map[string]string{"bank:" + iban: "221002", "cash:" + itoa(int(usd)): "211002"}); err != nil {
		t.Fatal(err)
	}
	if rv, _ = a.FXRevaluationFor(ctx, c.ID, 2026); !warn(rv, "nie je zaúčtovaných") {
		t.Fatalf("nezaúčtovaný pohyb: %+v", rv.Warnings)
	}
	if err := a.SetPosting(ctx, c.ID, 2026, "bank:"+itoa(int(txs[0].ID)), "", []ledger.Line{{Account: "221002", Debit: m.MustParse("400")}, {Account: "663", Credit: m.MustParse("400")}}, "", ledger.Dims{}); err != nil {
		t.Fatal(err)
	}
	if err := a.SetPosting(ctx, c.ID, 2026, "cash:"+itoa(int(cd.ID)), "", []ledger.Line{{Account: "211002", Debit: m.MustParse("1000")}, {Account: "491", Credit: m.MustParse("1000")}}, "", ledger.Dims{}); err != nil {
		t.Fatal(err)
	}
	rv, err = a.FXRevaluationFor(ctx, c.ID, 2026)
	if err != nil || len(rv.Warnings) != 0 {
		t.Fatalf("návrh: %+v %v", rv.Warnings, err)
	}
	got := map[string]string{}
	for _, it := range rv.Items {
		got[it.Key] = it.Account + " " + it.Remaining.String() + " " + it.BookEUR.String() + "→" + it.NewEUR.String() + " " + it.Diff.String()
	}
	// pokladnica 1 100 USD / 1,2 = 916,67 € (strata 83,33); banka 500 / 1,2 = 416,67 € (zisk 16,67)
	if got["cash:"+itoa(int(usd))] != "211002 1100.00 1000.00→916.67 -83.33" || got["bank:"+iban] != "221002 500.00 400.00→416.67 16.67" {
		t.Fatalf("položky: %v", got)
	}
	if lineStr(rv.Lines) != "563 MD 83.33, 211002 D 83.33, 221002 MD 16.67, 663 D 16.67" {
		t.Fatalf("riadky: %s", lineStr(rv.Lines))
	}
	// zostatok podľa výpisu
	st := m.MustParse("520")
	if err := a.SetFXStatement(ctx, c.ID, 2026, "bank:"+iban, &st); err != nil {
		t.Fatal(err)
	}
	if _, err := a.PostFXRevaluation(ctx, c.ID, 2026); err != nil {
		t.Fatal(err)
	}
	// po zaúčtovaní sa návrh nemení (vlastný doklad sa nepočíta do sumy v účtovníctve)
	rv, _ = a.FXRevaluationFor(ctx, c.ID, 2026)
	for _, it := range rv.Items {
		if it.Key == "bank:"+iban && (it.NewEUR.String() != "433.33" || it.Diff.String() != "33.33") {
			t.Fatalf("banka podľa výpisu: %+v", it)
		}
	}
	book, _ := a.Ledger(ctx, c.ID, 2026)
	for _, acc := range book.Accounts {
		switch acc.Account {
		case "211002":
			if acc.Closing.String() != "916.67" {
				t.Errorf("211002 k 31. 12.: %s", acc.Closing)
			}
		case "221002":
			if acc.Closing.String() != "433.33" {
				t.Errorf("221002 k 31. 12.: %s", acc.Closing)
			}
		}
	}
	if err := a.SetFXStatement(ctx, c.ID, 2026, "sale:1", &st); !fieldErr(err, "key") {
		t.Fatalf("výpis pri faktúre: %v", err)
	}
}
