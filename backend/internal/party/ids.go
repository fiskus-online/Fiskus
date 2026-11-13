// Package party overuje slovenské identifikátory: IČO, DIČ, IČ DPH.
package party

import (
	"fmt"
	"strings"
	"unicode"
)

// NormalizeID odstráni medzery a prevedie písmená na veľké.
func NormalizeID(s string) string {
	var b strings.Builder
	for _, r := range s {
		if !unicode.IsSpace(r) {
			b.WriteRune(unicode.ToUpper(r))
		}
	}
	return b.String()
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ValidICO overí IČO (8 číslic, kontrolná číslica modulo 11).
// Staršie 6-miestne IČO sa doplní nulami zľava.
func ValidICO(s string) bool {
	s = NormalizeID(s)
	if len(s) == 6 {
		s = "00" + s
	}
	if len(s) != 8 || !allDigits(s) {
		return false
	}
	sum := 0
	for i := 0; i < 7; i++ {
		sum += int(s[i]-'0') * (8 - i)
	}
	check := (11 - sum%11) % 10
	return int(s[7]-'0') == check
}

// ValidDIC overí formát DIČ (10 číslic).
func ValidDIC(s string) bool {
	s = NormalizeID(s)
	return len(s) == 10 && allDigits(s) && s[0] != '0'
}

// ValidICDPH overí slovenské IČ DPH: "SK" + 10 číslic deliteľných 11.
func ValidICDPH(s string) bool {
	s = NormalizeID(s)
	if !strings.HasPrefix(s, "SK") {
		return false
	}
	n := s[2:]
	if !ValidDIC(n) {
		return false
	}
	switch n[2] {
	case '2', '3', '4', '7', '8', '9':
	default:
		return false
	}
	rem := 0
	for _, r := range n {
		rem = (rem*10 + int(r-'0')) % 11
	}
	return rem == 0
}

// ValidIBAN overí IBAN kontrolným súčtom modulo 97 (ISO 13616).
// Pre slovenský IBAN navyše kontroluje dĺžku 24 znakov.
func ValidIBAN(s string) bool {
	s = NormalizeID(s)
	if len(s) < 15 || len(s) > 34 {
		return false
	}
	if strings.HasPrefix(s, "SK") && len(s) != 24 {
		return false
	}
	for i, r := range s {
		isDigit := r >= '0' && r <= '9'
		isLetter := r >= 'A' && r <= 'Z'
		if (i < 2 && !isLetter) || (i >= 2 && i < 4 && !isDigit) || (!isDigit && !isLetter) {
			return false
		}
	}
	rearranged := s[4:] + s[:4]
	rem := 0
	for _, r := range rearranged {
		if r >= 'A' {
			v := int(r-'A') + 10
			rem = (rem*100 + v) % 97
		} else {
			rem = (rem*10 + int(r-'0')) % 97
		}
	}
	return rem == 1
}

// IBANFromAccount prevedie slovenské alebo české číslo účtu v tvare
// „[predčíslie-]číslo/kód banky“ (napr. z výpisu Fio) na IBAN krajiny
// country (SK, CZ). Ak už je to IBAN, vráti ho bez medzier; inak "".
func IBANFromAccount(country, s string) string {
	s = strings.TrimSpace(s)
	if n := NormalizeID(s); ValidIBAN(n) {
		return n
	}
	country = strings.ToUpper(country)
	if country != "SK" && country != "CZ" {
		return ""
	}
	acc, bank, ok := strings.Cut(s, "/")
	if !ok || len(bank) != 4 || !digits(bank) {
		return ""
	}
	prefix, number, has := strings.Cut(acc, "-")
	if !has {
		prefix, number = "", acc
	}
	if !digits(prefix) || !digits(number) || number == "" || len(prefix) > 6 || len(number) > 10 {
		return ""
	}
	bban := bank + strings.Repeat("0", 6-len(prefix)) + prefix + strings.Repeat("0", 10-len(number)) + number
	// kontrolné číslice: (BBAN + krajina + 00) mod 97, 98 − zvyšok
	rem := 0
	for _, r := range bban + country + "00" {
		if r >= 'A' {
			v := int(r-'A') + 10
			rem = (rem*100 + v) % 97
		} else {
			rem = (rem*10 + int(r-'0')) % 97
		}
	}
	iban := country + fmt.Sprintf("%02d", 98-rem) + bban
	if !ValidIBAN(iban) {
		return ""
	}
	return iban
}

func digits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
