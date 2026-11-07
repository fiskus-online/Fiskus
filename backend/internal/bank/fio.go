package bank

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
)

// Fio API (API bankovníctvo Fio banky): pohyby za obdobie vo formáte JSON,
// https://fioapi.fio.cz/v1/rest/periods/{token}/{od}/{do}/transactions.json.
// Každý stĺpec je objekt {value, name, id} alebo null.

// FioURL je adresa API Fio banky (v testoch sa nahradí).
const FioURL = "https://fioapi.fio.cz/v1/rest"

// FioPeriodURL je adresa pohybov za obdobie. Token je tajný — adresu nikdy
// nezapisovať do logu ani chybových hlásení.
func FioPeriodURL(base, token string, from, to invoice.Date) string {
	return fmt.Sprintf("%s/periods/%s/%s/%s/transactions.json", strings.TrimRight(base, "/"), token, from.String(), to.String())
}

type fioColumn struct {
	Value json.RawMessage `json:"value"`
}

type fioDoc struct {
	AccountStatement struct {
		Info struct {
			AccountID string `json:"accountId"`
			BankID    string `json:"bankId"`
			Currency  string `json:"currency"`
			IBAN      string `json:"iban"`
		} `json:"info"`
		TransactionList struct {
			Transaction []map[string]*fioColumn `json:"transaction"`
		} `json:"transactionList"`
	} `json:"accountStatement"`
}

func (c *fioColumn) text() string {
	if c == nil || len(c.Value) == 0 || string(c.Value) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(c.Value, &s) == nil {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(string(c.Value)) // číslo ako text (ID, suma)
}

// ParseFio prečíta odpoveď Fio API (JSON) na pohyby.
func ParseFio(data []byte) ([]Transaction, error) {
	var doc fioDoc
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&doc); err != nil {
		return nil, ErrFormat
	}
	info := doc.AccountStatement.Info
	account := strings.ToUpper(strings.ReplaceAll(info.IBAN, " ", ""))
	if account == "" {
		account = info.AccountID
	}
	if account == "" {
		return nil, ErrFormat
	}
	out := []Transaction{}
	seen := map[string]int{}
	for i, row := range doc.AccountStatement.TransactionList.Transaction {
		col := func(n int) string { return row["column"+strconv.Itoa(n)].text() }
		d, err := invoice.ParseDate(strings.SplitN(col(0), "+", 2)[0])
		if err != nil {
			return nil, fmt.Errorf("pohyb %d: neplatný dátum %q", i+1, col(0))
		}
		amount, err := money.Parse(col(1))
		if err != nil {
			return nil, fmt.Errorf("pohyb %d: neplatná suma %q", i+1, col(1))
		}
		t := Transaction{Account: account, Date: d, Amount: amount, Direction: In, Currency: strings.ToUpper(col(14)),
			CounterName: col(10), KS: strings.TrimLeft(col(4), "0"), VS: NormalizeVS(col(5)), SS: strings.TrimLeft(col(6), "0"),
			BankRef: col(22), Status: StatusOpen}
		if t.Currency == "" {
			t.Currency = strings.ToUpper(info.Currency)
		}
		if amount.Sign() < 0 {
			t.Amount, t.Direction = amount.Neg(), Out
		}
		// protiúčet: IBAN alebo číslo účtu s kódom banky
		if cp := strings.ToUpper(strings.ReplaceAll(col(2), " ", "")); isIBANText(cp) {
			t.CounterIBAN = cp
		} else if cp != "" && col(3) != "" {
			t.CounterIBAN = cp + "/" + col(3)
		}
		if t.CounterName == "" {
			t.CounterName = col(7) // užívateľská identifikácia
		}
		var msg []string
		for _, n := range []int{16, 7, 25} { // správa pre príjemcu, identifikácia, komentár
			if s := col(n); s != "" && !contains(msg, s) {
				msg = append(msg, s)
			}
		}
		t.Message = strings.Join(msg, " · ")
		if t.VS == "" || t.SS == "" || t.KS == "" {
			vs, ss, ks := ParseSymbols(t.Message)
			t.VS, t.SS, t.KS = firstOf(t.VS, vs), firstOf(t.SS, ss), firstOf(t.KS, ks)
		}
		key := fingerprint(t, 0)
		seen[key]++
		t.Fingerprint = fingerprint(t, seen[key]-1)
		out = append(out, t)
	}
	return out, nil
}

func isIBANText(s string) bool {
	return len(s) >= 15 && s[0] >= 'A' && s[0] <= 'Z' && s[1] >= 'A' && s[1] <= 'Z' && s[2] >= '0' && s[2] <= '9'
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func firstOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
