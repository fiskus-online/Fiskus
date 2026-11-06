package dpfo

import (
	"bytes"
	_ "embed"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// Šablóna je prázdne XML uložené z formulára DPFOBv25 na portáli finančnej
// správy — poradie a tvar prvkov sa tak zhodujú s tým, čo formulár načíta.
//
//go:embed template_dpfobv25.xml
var templateV25 []byte

// node je prvok XML s poradím detí (encoding/xml poradie inak nezachová).
type node struct {
	name     string
	text     string
	cdata    bool
	children []*node
}

func parse(data []byte) (*node, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	var stack []*node
	var root *node
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &node{name: t.Name.Local}
			if len(stack) > 0 {
				p := stack[len(stack)-1]
				p.children = append(p.children, n)
			} else {
				root = n
			}
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 && strings.TrimSpace(string(t)) != "" {
				stack[len(stack)-1].text += string(t)
			}
		}
	}
	// prvky, ktoré formulár zapisuje ako CDATA
	var mark func(n *node)
	mark = func(n *node) {
		if n.name == "riadok" || n.name == "zaznamy" {
			n.cdata = true
		}
		for _, c := range n.children {
			mark(c)
		}
	}
	mark(root)
	return root, nil
}

// find vráti prvok podľa cesty "telo/tabulka1/t1r2/s1" (prvý výskyt).
func (n *node) find(path string) *node {
	cur := n
	for _, part := range strings.Split(path, "/") {
		var next *node
		for _, c := range cur.children {
			if c.name == part {
				next = c
				break
			}
		}
		if next == nil {
			return nil
		}
		cur = next
	}
	return cur
}

func (n *node) write(b *bytes.Buffer, depth int) {
	ind := strings.Repeat("\t", depth)
	if len(n.children) == 0 {
		b.WriteString(ind + "<" + n.name + ">")
		if n.cdata {
			b.WriteString("<![CDATA[" + strings.ReplaceAll(n.text, "]]>", "]]]]><![CDATA[>") + "]]>")
		} else {
			_ = xml.EscapeText(b, []byte(n.text))
		}
		b.WriteString("</" + n.name + ">\n")
		return
	}
	b.WriteString(ind + "<" + n.name + ">\n")
	for _, c := range n.children {
		c.write(b, depth+1)
	}
	b.WriteString(ind + "</" + n.name + ">\n")
}

// amount je suma vo formáte formulára (bodka, dve desatinné miesta); nula
// a prázdna hodnota sa nevypĺňajú.
func amount(d money.Dec) string {
	if d.IsZero() {
		return ""
	}
	return d.RoundCents().String()
}

func flag(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// XML vytvorí súbor na načítanie do formulára DPFOB.
func XML(in Input, res Result) ([]byte, error) {
	root, err := parse(templateV25)
	if err != nil {
		return nil, err
	}
	set := func(path, v string) error {
		n := root.find(path)
		if n == nil {
			return fmt.Errorf("šablóna priznania nemá prvok %s", path)
		}
		n.text = v
		return nil
	}
	tp := in.Taxpayer
	nace := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, tp.NACE)
	vals := [][2]string{
		{"hlavicka/dic", digits(tp.DIC)},
		{"hlavicka/datumNarodenia", tp.BirthDate},
		{"hlavicka/zdanovacieObdobie/rok", fmt.Sprint(in.Year)},
		{"hlavicka/priezvisko", tp.Surname},
		{"hlavicka/meno", tp.Name},
		{"hlavicka/titul", tp.Title},
		{"hlavicka/titulZa", tp.TitleAft},
		{"hlavicka/adresaTrvPobytu/ulica", tp.Street},
		{"hlavicka/adresaTrvPobytu/cislo", tp.Number},
		{"hlavicka/adresaTrvPobytu/psc", strings.ReplaceAll(tp.Zip, " ", "")},
		{"hlavicka/adresaTrvPobytu/obec", tp.City},
		{"hlavicka/adresaTrvPobytu/stat", tp.Country},
		{"hlavicka/skNace/cinnost", tp.Activity},
		{"telo/tabulka1/t1r2/s1", amount(res.T1Income)},
		{"telo/tabulka1/t1r2/s2", amount(res.T1Expenses)},
		{"telo/tabulka1/t1r10/s1", amount(res.T1Income)},
		{"telo/tabulka1/t1r10/s2", amount(res.T1Expenses)},
		{"telo/vydavkyPar6ods11_ods1a2", flag(!in.FlatExpenses && !res.T1Income.IsZero())},
		{"telo/vydavkyPar6ods10_ods1a2", flag(in.FlatExpenses)},
		{"telo/datumVyhlasenia", in.Date},
		{"telo/r151/ico", digits(in.NGOICO)},
		{"telo/r151/splnam3per", flag(in.Volunteer && in.NGOICO != "")},
		{"telo/r151/neuplatnujemPar50", flag(in.NGOICO == "" || res.Rows["152"].IsZero())},
	}
	if len(nace) == 5 {
		vals = append(vals, [2]string{"hlavicka/skNace/k1", nace[:2]}, [2]string{"hlavicka/skNace/k2", nace[2:4]}, [2]string{"hlavicka/skNace/k3", nace[4:]})
	}
	if in.FlatExpenses {
		vals = append(vals, [2]string{"telo/vydavkyPoistPar6ods11_ods1a2", amount(in.Insurance)})
	}
	for i, row := range in.Table1a {
		vals = append(vals,
			[2]string{fmt.Sprintf("telo/tabulka1a/t1r%d/s1", i+1), amount(row[0])},
			[2]string{fmt.Sprintf("telo/tabulka1a/t1r%d/s2", i+1), amount(row[1])})
	}
	for _, code := range []string{"36", "37", "38", "39", "40", "41", "42", "45", "46", "51", "52", "53", "54", "54a", "55", "57", "72", "73", "74", "75", "77", "78", "80", "81",
		"90", "91", "92", "94", "95", "96", "105", "116", "116a", "117", "118", "119", "120", "121", "122", "124", "131", "133", "135", "136", "152"} {
		vals = append(vals, [2]string{"telo/r" + code, amount(res.Rows[code])})
	}
	// straty z predchádzajúcich rokov (r. 47 – 50a): rok, strata, odpočítaná, zostatok
	for i, row := range lossRows {
		r := row[0]
		vals = append(vals, [2]string{"telo/r" + r + "/rok", fmt.Sprint(in.Year - 5 + i)},
			[2]string{"telo/r" + r + "/udaje/r02", amount(res.Rows["l"+r+".02"])},
			[2]string{"telo/r" + r + "/udaje/r04", amount(res.Rows["l"+r+".04"])})
		if r != "50a" {
			vals = append(vals, [2]string{"telo/r" + r + "/udaje/r03", amount(res.Rows["l"+r+".03"])})
		}
	}
	// manžel/ka (r. 31, 32)
	if sp := in.Spouse; sp != nil && strings.TrimSpace(sp.Name) != "" {
		vals = append(vals, [2]string{"telo/r31/priezviskoMeno", strings.TrimSpace(sp.Name)}, [2]string{"telo/r31/rodneCislo", rcDigits(sp.RC)})
		if sp.Months > 0 {
			vals = append(vals, [2]string{"telo/r32/uplatnujemNCZDNaManzela", "1"},
				[2]string{"telo/r32/vlastnePrijmy", amount(sp.OwnIncome)},
				[2]string{"telo/r32/pocetMesiacov", fmt.Sprint(min(sp.Months, 12))})
			if sp.OwnIncome.IsZero() {
				vals = append(vals, [2]string{"telo/r32/vlastnePrijmy", "0.00"})
			}
		}
	}
	vals = append(vals, [2]string{"telo/r154", fmt.Sprint(max(in.Attachments, 0))})
	// daňový bonus na vyplatenie (r. 121)
	if res.BonusPaid.Sign() > 0 {
		vals = append(vals, [2]string{"telo/danovyPreplatokBonus/vyplatitDanovyBonus", "1"}, [2]string{"telo/danovyPreplatokBonus/datum", in.Date})
		if in.RefundIBAN != "" {
			vals = append(vals, [2]string{"telo/danovyPreplatokBonus/sposobPlatby/ucet", "1"},
				[2]string{"telo/danovyPreplatokBonus/bankovyUcet/IBAN", strings.ReplaceAll(strings.ToUpper(in.RefundIBAN), " ", "")})
		} else {
			vals = append(vals, [2]string{"telo/danovyPreplatokBonus/sposobPlatby/poukazka", "1"})
		}
	}
	// r. 95 (príjmy na účely sadzby 15 %) sa vypĺňa aj nulou, ak je vyplnený r. 39
	if res.Rows["95"].IsZero() && !res.T1Income.IsZero() {
		vals = append(vals, [2]string{"telo/r95", "0.00"})
	}
	if res.Refund.Sign() > 0 && in.RefundIBAN != "" {
		vals = append(vals,
			[2]string{"telo/danovyPreplatokBonus/vratitDanPreplatok", "1"},
			[2]string{"telo/danovyPreplatokBonus/sposobPlatby/ucet", "1"},
			[2]string{"telo/danovyPreplatokBonus/bankovyUcet/IBAN", strings.ReplaceAll(strings.ToUpper(in.RefundIBAN), " ", "")},
			[2]string{"telo/danovyPreplatokBonus/datum", in.Date})
	}
	for _, v := range vals {
		if err := set(v[0], v[1]); err != nil {
			return nil, err
		}
	}
	// deti (r. 33): najviac 4; celý rok = m00
	if kids := root.find("telo/r33"); kids != nil {
		slots := []*node{}
		for _, c := range kids.children {
			if c.name == "dieta" {
				slots = append(slots, c)
			}
		}
		for i, ch := range in.Children {
			if i >= len(slots) {
				break
			}
			slot := slots[i]
			put := func(name, v string) {
				for _, c := range slot.children {
					if c.name == name {
						c.text = v
					}
				}
			}
			put("priezviskoMeno", strings.TrimSpace(ch.Name))
			put("rodneCislo", rcDigits(ch.RC))
			if ch.allYear() {
				put("m00", "1")
			} else {
				for mo, on := range ch.Months {
					put(fmt.Sprintf("m%02d", mo+1), flag(on))
				}
			}
		}
	}
	if name := strings.TrimSpace(in.NGOName); name != "" && in.NGOICO != "" {
		if rows := root.find("telo/r151/obchodneMeno"); rows != nil && len(rows.children) > 0 {
			first, rest := splitLine(name, 37)
			rows.children[0].text = first
			if len(rows.children) > 1 {
				rows.children[1].text = rest
			}
		}
	}
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	root.write(&b, 0)
	return b.Bytes(), nil
}

// rcDigits: rodné číslo bez lomky.
func rcDigits(s string) string { return digits(s) }

func digits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

// splitLine rozdelí názov na dva riadky formulára po slovách.
func splitLine(s string, width int) (string, string) {
	r := []rune(s)
	if len(r) <= width {
		return s, ""
	}
	cut := width
	for i := width; i > 0; i-- {
		if r[i] == ' ' {
			cut = i
			break
		}
	}
	return strings.TrimSpace(string(r[:cut])), strings.TrimSpace(string(r[cut:]))
}
