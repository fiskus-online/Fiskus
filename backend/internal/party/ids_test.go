package party

import "testing"

func TestValidICO(t *testing.T) {
	cases := map[string]bool{
		"31322832":   true, // Slovnaft, a.s.
		"31 322 832": true,
		"31322833":   false, // zlá kontrolná číslica
		"3132283":    false,
		"3132283A":   false,
		"":           false,
	}
	for in, want := range cases {
		if got := ValidICO(in); got != want {
			t.Errorf("ValidICO(%q) = %v, chcem %v", in, got, want)
		}
	}
}

func TestValidICDPH(t *testing.T) {
	cases := map[string]bool{
		"SK2020372640":  true, // Slovnaft, a.s.
		"sk 2020372640": true,
		"SK2020372641":  false, // nie je deliteľné 11
		"CZ2020372640":  false,
		"SK0020372640":  false,
		"SK20203726":    false,
	}
	for in, want := range cases {
		if got := ValidICDPH(in); got != want {
			t.Errorf("ValidICDPH(%q) = %v, chcem %v", in, got, want)
		}
	}
}

func TestValidDIC(t *testing.T) {
	if !ValidDIC("2020372640") || ValidDIC("202037264") || ValidDIC("0020372640") {
		t.Fatal("ValidDIC")
	}
}

func TestValidIBAN(t *testing.T) {
	cases := map[string]bool{
		"SK31 1200 0000 1987 4263 7541": true, // vzorový IBAN z ECBS
		"sk3112000000198742637541":      true,
		"SK31 1200 0000 1987 4263 7542": false, // zlý kontrolný súčet
		"SK31 1200 0000 1987 4263 754":  false, // krátky
		"DE89 3704 0044 0532 0130 00":   true,
		"CZ65 0800 0000 1920 0014 5399": true,
		"XX00":                          false,
		"":                              false,
	}
	for in, want := range cases {
		if got := ValidIBAN(in); got != want {
			t.Errorf("ValidIBAN(%q) = %v, chcem %v", in, got, want)
		}
	}
}

func TestIBANFromAccount(t *testing.T) {
	tests := []struct {
		name, country, in, want string
	}{
		{"slovenský účet z Fio", "SK", "1234568003/5600", "SK4756000000001234568003"},
		{"s predčíslím", "SK", "19-1987426375/1200", "SK4312000000191987426375"},
		{"už IBAN s medzerami", "SK", "SK47 5600 0000 0012 3456 8003", "SK4756000000001234568003"},
		{"český účet", "CZ", "19-2000145399/0800", "CZ6508000000192000145399"},
		{"zlý tvar", "SK", "abc/12", ""},
		{"iná krajina", "DE", "1234568003/5600", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IBANFromAccount(tc.country, tc.in); got != tc.want {
				t.Fatalf("%q, chcem %q", got, tc.want)
			}
		})
	}
}
