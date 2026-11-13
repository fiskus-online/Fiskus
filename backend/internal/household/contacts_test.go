package household

import "testing"

func TestMatchPartner(t *testing.T) {
	ps := []Partner{
		{ID: 1, Name: "Západoslovenská energetika, a.s.", IBAN: "SK12 0900 0000 0001 2345 6789"},
		{ID: 2, Name: "Ján Novák"},
		{ID: 5, Name: "ZŠ Moyzesova", IBAN: "SK3112000000198742637541", IBANs: []string{"SK24 1100 0000 0026 1234 5678"}},
		{ID: 3, Name: "Mária Malá"},
		{ID: 4, Name: "Maria Mala"},
	}
	cases := []struct {
		name, iban, payee string
		want              int64
	}{
		{"podľa IBAN", "sk1209000000000123456789", "ZSE", 1},
		{"IBAN má prednosť pred menom", "SK1209000000000123456789", "Ján Novák", 1},
		{"podľa mena bez diakritiky", "", "JAN NOVAK", 2},
		{"ďalší účet partnera", "SK2411000000002612345678", "ZS MOYZESOVA STRAVNE", 5},
		{"neznámy IBAN, meno sedí", "SK9911000000000000000001", "Ján Novák", 2},
		{"dvaja s rovnakým menom", "", "Mária Malá", 0},
		{"nikto", "", "Lidl", 0},
		{"prázdne", "", "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := MatchPartner(ps, c.iban, c.payee); got != c.want {
				t.Fatalf("partner %d, čakal som %d", got, c.want)
			}
		})
	}
}

func TestPartnerEntries(t *testing.T) {
	p := Partner{ID: 7, Name: "Ján Novák"}
	es := []Entry{
		{ID: 1, Date: d(2025, 12, 20), AccountID: 1, Amount: m("-100"), ContactID: 7},
		{ID: 2, Date: d(2026, 2, 1), AccountID: 1, Amount: m("-50"), Payee: "jan novak"},               // bez väzby, podľa mena
		{ID: 3, Date: d(2026, 3, 1), AccountID: 1, Amount: m("30"), ContactID: 7},                      // príjem
		{ID: 4, Date: d(2026, 3, 2), AccountID: 1, Amount: m("-10"), ContactID: 8, Payee: "Ján Novák"}, // iný partner s rovnakým menom
		{ID: 5, Date: d(2026, 3, 3), AccountID: 1, ToAccountID: 2, Amount: m("-20"), ContactID: 7},     // prevod
		{ID: 6, Date: d(2026, 1, 5), AccountID: 1, Amount: m("-5"), Payee: "Lidl"},
	}
	got, tot := PartnerEntries(es, p, 2026)
	ids := []int64{}
	for _, e := range got {
		ids = append(ids, e.ID)
	}
	want := []int64{3, 2, 1}
	if len(ids) != len(want) {
		t.Fatalf("pohyby %v, čakal som %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("pohyby %v, čakal som %v", ids, want)
		}
	}
	checks := []struct {
		name string
		got  string
		want string
	}{
		{"zaplatené spolu", tot.Paid.String(), "150.00"},
		{"prijaté spolu", tot.Received.String(), "30.00"},
		{"zaplatené v roku", tot.YearPaid.String(), "50.00"},
		{"prijaté v roku", tot.YearReceived.String(), "30.00"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: %s, čakal som %s", c.name, c.got, c.want)
		}
	}
	if tot.Count != 3 || tot.Year != 2026 {
		t.Fatalf("počet %d rok %d", tot.Count, tot.Year)
	}
}

func TestObligationOfPartner(t *testing.T) {
	p := Partner{ID: 2, Name: "Home Credit"}
	cases := []struct {
		name string
		o    Obligation
		want bool
	}{
		{"väzba", Obligation{ContactID: 2}, true},
		{"iná väzba", Obligation{ContactID: 3, Creditor: "Home Credit"}, false},
		{"podľa veriteľa", Obligation{Creditor: "HOME CREDIT"}, true},
		{"iný veriteľ", Obligation{Creditor: "VÚB"}, false},
	}
	for _, c := range cases {
		if got := c.o.OfPartner(p); got != c.want {
			t.Errorf("%s: %v", c.name, got)
		}
	}
}
