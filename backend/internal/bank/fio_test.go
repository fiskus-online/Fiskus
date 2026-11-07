package bank

import (
	"os"
	"strings"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
)

func TestParseFio(t *testing.T) {
	data, err := os.ReadFile("testdata/fio.json")
	if err != nil {
		t.Fatal(err)
	}
	txs, err := ParseFio(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"SK2683300000002801234567 2026-11-03 in 492.00 EUR VS=20260042 SK3112000000198742637541 Hotel Lesná s.r.o. | Faktúra FA20260042 | 26123456780",
		"SK2683300000002801234567 2026-11-04 out 123.13 EUR VS=20261187 2900123456/2010 Nákup materiálu | Nákup materiálu · Platba VS 20261187 | 26123456781",
		"SK2683300000002801234567 2026-11-05 in 298.00 EUR VS= | Platby kartou 4.11. | 26123456782",
	}
	if len(txs) != len(want) {
		t.Fatalf("pohyby: %d", len(txs))
	}
	fps := map[string]bool{}
	for i, x := range txs {
		got := strings.Join(strings.Fields(strings.Join([]string{x.Account, x.Date.String(), string(x.Direction), x.Amount.String(), x.Currency, "VS=" + x.VS, x.CounterIBAN, x.CounterName, "|", x.Message, "|", x.BankRef}, " ")), " ")
		if got != want[i] {
			t.Errorf("pohyb %d:\n got %s\nwant %s", i, got, want[i])
		}
		if fps[x.Fingerprint] || x.Fingerprint == "" {
			t.Errorf("odtlačok %d", i)
		}
		fps[x.Fingerprint] = true
	}
}

func TestParseFioErrors(t *testing.T) {
	for _, in := range []string{"", "<xml/>", `{"accountStatement":{"info":{}}}`, `{"accountStatement":{"info":{"iban":"SK1"},"transactionList":{"transaction":[{"column0":{"value":"x"}}]}}}`} {
		if _, err := ParseFio([]byte(in)); err == nil {
			t.Errorf("%q: chýba chyba", in)
		}
	}
	u := FioPeriodURL("https://fioapi.fio.cz/v1/rest/", "TOKEN", invoice.MustDate("2026-11-01"), invoice.MustDate("2026-11-07"))
	if u != "https://fioapi.fio.cz/v1/rest/periods/TOKEN/2026-11-01/2026-11-07/transactions.json" {
		t.Fatal(u)
	}
}
