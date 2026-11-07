package app

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/bank"
	"github.com/fiskus-online/fiskus/backend/internal/cash"
	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/journal"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

func closing(number, card string, rows ...cash.VATRow) cash.Closing {
	return cash.Closing{Date: invoice.MustDate("2026-10-12"), ORP: "88812345678900001", Number: number, Receipts: 40, Rows: rows, Card: m.MustParse(card), Goods: true}
}

func row(rate int, gross string) cash.VATRow {
	return cash.VATRow{Rate: vat.Rate(rate), Gross: m.MustParse(gross)}
}

func TestCashClosing(t *testing.T) {
	a, c := newApp(t, edition.Desktop)

	// náhľad: základ a DPH podľa sadzieb, hotovosť
	p, err := a.PreviewCashClosing(ctx, c.ID, closing("41", "100", row(23, "123"), row(5, "105"), row(23, "123")))
	if err != nil || len(p.Errors) != 0 || len(p.Rows) != 2 || p.Rows[0].VAT.String() != "46.00" || p.Rows[1].VAT.String() != "5.00" || p.Total.String() != "351.00" || p.CashPart.String() != "251.00" {
		t.Fatalf("náhľad: %+v %v", p, err)
	}
	for _, tt := range []struct {
		name  string
		c     cash.Closing
		field string
	}{
		{"bez obratu", closing("1", "0"), "rows"},
		{"karta nad tržbu", closing("1", "200", row(23, "123")), "card"},
		{"zlá sadzba", closing("1", "0", row(21, "100")), "rows.0.vatRate"},
	} {
		if _, err := a.CreateCashClosing(ctx, c.ID, tt.c); !fieldErr(err, tt.field) {
			t.Errorf("%s: %v", tt.name, err)
		}
	}

	cl, err := a.CreateCashClosing(ctx, c.ID, closing("41", "100", row(23, "246"), row(5, "105")))
	if err != nil || cl.InNumber == "" || cl.OutNumber == "" {
		t.Fatalf("uzávierka: %+v %v", cl, err)
	}
	in, _ := a.Store.GetCashDocument(ctx, c.ID, cl.InID)
	out, _ := a.Store.GetCashDocument(ctx, c.ID, cl.OutID)
	if in.Kind != cash.In || in.Amount.String() != "351.00" || in.Tax == nil || !in.Tax.Goods || len(in.Tax.Rows) != 2 || in.LinkKind != cash.LinkClosing {
		t.Fatalf("PPD: %+v", in)
	}
	if out.Kind != cash.Out || out.Amount.String() != "100.00" || out.Tax != nil || out.LinkNumber != "Tržba e-kasa, uzávierka 41 (ORP 88812345678900001)" {
		t.Fatalf("VPD: %+v", out)
	}
	if _, err := a.CreateCashClosing(ctx, c.ID, closing("41", "0", row(23, "1"))); !fieldErr(err, "number") {
		t.Fatalf("dvakrát tá istá uzávierka: %v", err)
	}
	if _, err := a.CancelCash(ctx, c.ID, in.ID); !fieldErr(err, "linkId") {
		t.Fatalf("storno PPD mimo uzávierky: %v", err)
	}
	if _, err := a.CreateCash(ctx, c.ID, cash.Document{Kind: cash.Out, Date: in.Date, Amount: m.MustParse("1"), Purpose: "x", LinkKind: cash.LinkClosing, LinkID: cl.ID}); !fieldErr(err, "linkId") {
		t.Fatalf("ďalší doklad k uzávierke: %v", err)
	}

	// DPH: tržba v D.1 a r. 03/04, 05/06
	ret, _, err := a.VATReturn(ctx, c.ID, "2026-10", m.Zero)
	if err != nil || ret.KV.D1.Count != 1 || ret.KV.D1.VAT.String() != "46.00" || ret.KV.D1.VATReduced.String() != "5.00" {
		t.Fatalf("DPH: %+v %v", ret.KV.D1, err)
	}

	// podvojné: 211/604/343 a 261/211; pripísanie z terminálu 221/261
	txs, _, err := a.Store.SaveBankTransactions(ctx, c.ID, []bank.Transaction{{Account: "SK3112000000198742637541", Date: invoice.MustDate("2026-10-14"),
		Amount: m.MustParse("100"), Currency: "EUR", Direction: bank.In, Message: "Platby kartou 12.10.", CounterName: "Global Payments", Fingerprint: "k1", Status: bank.StatusOpen}})
	if err != nil {
		t.Fatal(err)
	}
	book, err := a.Ledger(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"cash:" + itoa(int(in.ID)):     "P-TRZBA-TOVAR 211 MD 351.00, 604 D 300.00, 343 D 51.00",
		"cash:" + itoa(int(out.ID)):    "P-KARTA 261 MD 100.00, 211 D 100.00",
		"bank:" + itoa(int(txs[0].ID)): "B-KARTA 221 MD 100.00, 261 D 100.00",
	}
	for _, ps := range book.Postings {
		if w, ok := want[ps.Key]; ok {
			if got := ps.Template + " " + lineStr(ps.Lines); got != w {
				t.Errorf("%s: %s, chcem %s", ps.Key, got, w)
			}
			delete(want, ps.Key)
		}
	}
	if len(want) != 0 {
		t.Errorf("chýbajú zápisy: %v", want)
	}

	// peňažný denník: tržba d1 + DPH, karta priebežne c (aj pripísanie)
	jb, err := a.Journal(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	cols := map[string]string{}
	for _, e := range jb.Entries {
		s := ""
		for _, l := range e.Lines {
			s += string(l.Column) + "=" + l.Amount.String() + " "
		}
		cols[e.Key] = s
	}
	if cols["cash:"+itoa(int(in.ID))] != "d1=300.00 f.dph=51.00 " || cols["cash:"+itoa(int(out.ID))] != string(journal.TransitOut)+"=100.00 " ||
		cols["bank:"+itoa(int(txs[0].ID))] != string(journal.TransitIn)+"=100.00 " {
		t.Fatalf("denník: %v", cols)
	}

	// storno uzávierky stornuje oba doklady
	if _, err := a.CancelCashClosing(ctx, c.ID, cl.ID); err != nil {
		t.Fatal(err)
	}
	in, _ = a.Store.GetCashDocument(ctx, c.ID, cl.InID)
	out, _ = a.Store.GetCashDocument(ctx, c.ID, cl.OutID)
	if !in.Cancelled || !out.Cancelled {
		t.Fatal("doklady uzávierky nie sú stornované")
	}
	if list, _ := a.CashClosings(ctx, c.ID, 2026); len(list) != 1 || !list[0].Cancelled {
		t.Fatalf("zoznam: %+v", list)
	}
	// po storne sa dá zapísať znova
	if _, err := a.CreateCashClosing(ctx, c.ID, closing("41", "0", row(23, "246"))); err != nil {
		t.Fatal(err)
	}
}

func TestCashClosingNonPayer(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	c.VATPayer, c.ICDPH = false, ""
	if err := a.Store.UpdateCompany(ctx, c); err != nil {
		t.Fatal(err)
	}
	cl := closing("7", "20", row(23, "50"), row(5, "30"))
	cl.Goods = false
	saved, err := a.CreateCashClosing(ctx, c.ID, cl)
	if err != nil || len(saved.Rows) != 1 || saved.Rows[0].Rate != 0 || saved.Rows[0].Gross.String() != "80.00" {
		t.Fatalf("neplatiteľ: %+v %v", saved, err)
	}
	in, _ := a.Store.GetCashDocument(ctx, c.ID, saved.InID)
	if in.Tax != nil || in.Amount.String() != "80.00" {
		t.Fatalf("PPD neplatiteľa: %+v", in)
	}
	jb, _ := a.Journal(ctx, c.ID, 2026)
	for _, e := range jb.Entries {
		if e.Key == "cash:"+itoa(int(in.ID)) && (len(e.Lines) != 1 || e.Lines[0].Column != journal.IncServices || e.Lines[0].Amount.String() != "80.00") {
			t.Fatalf("denník neplatiteľa: %+v", e)
		}
	}
}
