package app

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func TestContactPersonsAddresses(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	tests := []struct {
		name      string
		persons   []store.ContactPerson
		addresses []store.ContactAddress
		errField  string
		wantP     int
		wantA     int
	}{
		{"prázdne riadky sa vynechajú", []store.ContactPerson{{}, {Name: " Peter "}}, []store.ContactAddress{{}}, "", 1, 0},
		{"osoba bez mena", []store.ContactPerson{{Email: "a@b.sk"}}, nil, "persons[0].name", 0, 0},
		{"zlý e-mail osoby", []store.ContactPerson{{Name: "Eva", Email: "eva@"}}, nil, "persons[0].email", 0, 0},
		{"adresa bez obce", nil, []store.ContactAddress{{Street: "Hlavná 1"}}, "addresses[0].street", 0, 0},
		{"neznámy druh", nil, []store.ContactAddress{{Kind: "x", Street: "Hlavná 1", City: "Nitra"}}, "addresses[0].kind", 0, 0},
		{"druh a štát doplnené", nil, []store.ContactAddress{{Street: "Hlavná 1", City: "Nitra", Zip: "949 01"}}, "", 0, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := a.SaveContact(ctx, c.ID, store.Contact{Name: "Firma " + tc.name, Persons: tc.persons, Addresses: tc.addresses})
			if tc.errField != "" {
				if !fieldErr(err, tc.errField) {
					t.Fatalf("chcem chybu %s, mám %v", tc.errField, err)
				}
				return
			}
			if err != nil || len(got.Persons) != tc.wantP || len(got.Addresses) != tc.wantA {
				t.Fatalf("%v %+v %+v", err, got.Persons, got.Addresses)
			}
			if tc.wantA == 1 && (got.Addresses[0].Kind != store.AddressDelivery || got.Addresses[0].Country != "SK" || got.Addresses[0].Zip != "94901") {
				t.Errorf("adresa: %+v", got.Addresses[0])
			}
			if tc.wantP == 1 && got.Persons[0].Name != "Peter" {
				t.Errorf("osoba: %+v", got.Persons[0])
			}
		})
	}
	// predvolená iba jedna osoba a jedna dodacia adresa
	k, err := a.SaveContact(ctx, c.ID, store.Contact{Name: "Kováč",
		Persons: []store.ContactPerson{{Name: "A", Primary: true}, {Name: "B", Primary: true, Phone: "0900"}},
		Addresses: []store.ContactAddress{
			{Kind: store.AddressDelivery, Name: "Sklad 1", Street: "S1", City: "Nitra", Default: true},
			{Kind: store.AddressDelivery, Name: "Sklad 2", Street: "S2", City: "Nitra", Default: true},
			{Kind: store.AddressPostal, Street: "P.O. Box 1", City: "Nitra", Default: true},
		}})
	if err != nil {
		t.Fatal(err)
	}
	if !k.Persons[0].Primary || k.Persons[1].Primary || !k.Addresses[0].Default || k.Addresses[1].Default || !k.Addresses[2].Default {
		t.Errorf("predvolené: %+v %+v", k.Persons, k.Addresses)
	}
	d, p := DocExtras(k)
	if d == nil || d.Name != "Sklad 1" || p == nil || p.Name != "A" {
		t.Errorf("na doklad: %+v %+v", d, p)
	}
	if d, p := DocExtras(store.Contact{}); d != nil || p != nil {
		t.Error("prázdny kontakt")
	}
}

func TestContactAccounts(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	const main, food, fees = "SK3112000000198742637541", "SK2411000000002612345678", "SK8975000000000012345671"
	tests := []struct {
		name     string
		accounts []store.ContactAccount
		errField string
		want     []store.ContactAccount
	}{
		{"prázdne sa vynechajú, IBAN po štvoriciach", []store.ContactAccount{{}, {IBAN: "sk24 1100000000 2612345678", Label: " stravné "}},
			"", []store.ContactAccount{{IBAN: "SK24 1100 0000 0026 1234 5678", Label: "stravné"}}},
		{"rovnaký ako hlavný alebo dvakrát iba raz", []store.ContactAccount{{IBAN: main}, {IBAN: fees, Label: "poplatky"}, {IBAN: fees}},
			"", []store.ContactAccount{{IBAN: "SK89 7500 0000 0000 1234 5671", Label: "poplatky"}}},
		{"zlý IBAN", []store.ContactAccount{{IBAN: food}, {IBAN: "SK2411000000002612345679"}}, "accounts[1].iban", nil},
		{"účel bez IBAN", []store.ContactAccount{{Label: "rodičovský"}}, "accounts[0].iban", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := a.SaveContact(ctx, c.ID, store.Contact{Name: "Škola " + tc.name, IBAN: main, Accounts: tc.accounts})
			if tc.errField != "" {
				if !fieldErr(err, tc.errField) {
					t.Fatalf("chcem chybu %s, mám %v", tc.errField, err)
				}
				return
			}
			if err != nil || len(got.Accounts) != len(tc.want) {
				t.Fatalf("%v %+v", err, got.Accounts)
			}
			for i := range tc.want {
				if got.Accounts[i] != tc.want[i] {
					t.Fatalf("účet %d: %+v, chcem %+v", i, got.Accounts[i], tc.want[i])
				}
			}
		})
	}
}
