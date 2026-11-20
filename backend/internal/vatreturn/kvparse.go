package vatreturn

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

// FiledKV je kontrolný výkaz načítaný zo súboru XML (KVDPH_2025), ako bol
// podaný v eDane.
type FiledKV struct {
	ICDPH  string
	Period Period
	Druh   string // R riadny, O opravný, D dodatočný (iba opravované riadky s KOpr)
	KV     KV
}

type xmlKV struct {
	XMLName xml.Name `xml:"KVDPH_2025"`
	Ident   struct {
		ICDPH   string `xml:"IcDphPlatitela"`
		Druh    string `xml:"Druh"`
		Obdobie struct {
			Rok      int `xml:"Rok"`
			Mesiac   int `xml:"Mesiac"`
			Stvrtrok int `xml:"Stvrtrok"`
		} `xml:"Obdobie"`
	} `xml:"Identifikacia"`
	T struct {
		A1  []xmlAttrs `xml:"A1"`
		A2  []xmlAttrs `xml:"A2"`
		B1  []xmlAttrs `xml:"B1"`
		B2  []xmlAttrs `xml:"B2"`
		B31 []xmlAttrs `xml:"B31"`
		B32 []xmlAttrs `xml:"B32"`
		C1  []xmlAttrs `xml:"C1"`
		C2  []xmlAttrs `xml:"C2"`
		D1  []xmlAttrs `xml:"D1"`
		D2  []xmlAttrs `xml:"D2"`
	} `xml:"Transakcie"`
}

// xmlAttrs sú atribúty riadku (všetky časti majú riadok ako prázdny element s atribútmi).
type xmlAttrs struct {
	Attrs []xml.Attr `xml:",any,attr"`
}

func (x xmlAttrs) get(name string) string {
	for _, a := range x.Attrs {
		if a.Name.Local == name {
			return strings.TrimSpace(a.Value)
		}
	}
	return ""
}

// ErrKVFile: súbor nie je kontrolný výkaz KVDPH_2025.
var ErrKVFile = errors.New("súbor nie je kontrolný výkaz k DPH vo formáte XML pre eDane (KVDPH_2025)")

// ParseKVXML načíta kontrolný výkaz zo súboru XML pre eDane.
func ParseKVXML(data []byte) (FiledKV, error) {
	var x xmlKV
	if err := xml.Unmarshal(data, &x); err != nil {
		return FiledKV{}, ErrKVFile
	}
	f := FiledKV{ICDPH: strings.ToUpper(strings.ReplaceAll(x.Ident.ICDPH, " ", "")), Druh: strings.TrimSpace(x.Ident.Druh)}
	o := x.Ident.Obdobie
	switch {
	case o.Rok == 0:
		return f, ErrKVFile
	case o.Stvrtrok > 0:
		f.Period = Period{Year: o.Rok, Quarter: o.Stvrtrok}
	default:
		f.Period = Period{Year: o.Rok, Month: o.Mesiac}
	}
	if f.Druh != "R" && f.Druh != "O" && f.Druh != "D" {
		return f, fmt.Errorf("neznámy druh kontrolného výkazu %q", f.Druh)
	}
	var perr error
	amt := func(x xmlAttrs, name string) money.Dec {
		v := x.get(name)
		if v == "" {
			return money.Zero
		}
		d, err := money.Parse(v)
		if err != nil && perr == nil {
			perr = fmt.Errorf("neplatná suma %s=%q", name, v)
		}
		return d
	}
	optAmt := func(x xmlAttrs, name string) *money.Dec {
		if x.get(name) == "" {
			return nil
		}
		d := amt(x, name)
		return &d
	}
	date := func(x xmlAttrs) invoice.Date {
		d, err := invoice.ParseDate(x.get("Den"))
		if err != nil && perr == nil {
			perr = fmt.Errorf("neplatný dátum %q", x.get("Den"))
		}
		return d
	}
	num := func(x xmlAttrs, name string) int {
		v := x.get(name)
		if v == "" {
			return 0
		}
		n, err := strconv.Atoi(v)
		if err != nil && perr == nil {
			perr = fmt.Errorf("neplatné číslo %s=%q", name, v)
		}
		return n
	}
	line := func(x xmlAttrs, partner string) KVLine {
		return KVLine{Partner: x.get(partner), Number: x.get("F"), Date: date(x), Base: amt(x, "Z"), VAT: amt(x, "D"),
			Rate: vat.Rate(num(x, "S")), Deducted: amt(x, "O"), Fix: num(x, "KOpr")}
	}
	for _, a := range x.T.A1 {
		f.KV.A1 = append(f.KV.A1, line(a, "Odb"))
	}
	for _, a := range x.T.A2 {
		f.KV.A2 = append(f.KV.A2, KVA2{Partner: a.get("Odb"), Number: a.get("F"), Date: date(a), Base: amt(a, "Z"),
			Code: a.get("TK"), Goods: a.get("TD"), Quantity: optAmt(a, "Mn"), Unit: a.get("MJ"), Fix: num(a, "KOpr")})
	}
	for _, a := range x.T.B1 {
		f.KV.B1 = append(f.KV.B1, line(a, "Dod"))
	}
	for _, a := range x.T.B2 {
		f.KV.B2 = append(f.KV.B2, line(a, "Dod"))
	}
	b3 := func(a xmlAttrs) KVB3 {
		return KVB3{Partner: a.get("Dod"), Base: amt(a, "Z"), VAT: amt(a, "D"), Deducted: amt(a, "O"), Fix: num(a, "KOpr")}
	}
	for _, a := range x.T.B31 {
		b := b3(a)
		if b.Fix == FixCancel {
			f.KV.B31Old = &b
		} else {
			f.KV.B31 = &b
		}
	}
	for _, a := range x.T.B32 {
		f.KV.B32 = append(f.KV.B32, b3(a))
	}
	corr := func(a xmlAttrs, partner string) KVCorrection {
		c := KVCorrection{Partner: a.get(partner), Number: a.get("FO"), Original: a.get("FP"), BaseDiff: amt(a, "ZR"),
			VATDiff: amt(a, "DR"), Rate: vat.Rate(num(a, "S")), DeductedDiff: amt(a, "OR"),
			Code: a.get("TK"), Goods: a.get("TD"), Quantity: optAmt(a, "Mn"), Unit: a.get("MJ"), Fix: num(a, "KOpr")}
		c.A2 = partner == "Odb" && a.get("DR") == "" && a.get("S") == ""
		return c
	}
	for _, a := range x.T.C1 {
		f.KV.C1 = append(f.KV.C1, corr(a, "Odb"))
	}
	for _, a := range x.T.C2 {
		f.KV.C2 = append(f.KV.C2, corr(a, "Dod"))
	}
	sum := func(list []xmlAttrs, cur *KVSum, old **KVSum) {
		for _, a := range list {
			s := KVSum{Turnover: amt(a, "SumaObratov"), Base: amt(a, "Z"), VAT: amt(a, "D"), BaseReduced: amt(a, "ZZn"),
				VATReduced: amt(a, "DZn"), Count: 1, Fix: num(a, "KOpr")}
			if s.Fix == FixCancel {
				*old = &s
			} else {
				*cur = s
			}
		}
	}
	sum(x.T.D1, &f.KV.D1, &f.KV.D1Old)
	sum(x.T.D2, &f.KV.D2, &f.KV.D2Old)
	if perr != nil {
		return f, perr
	}
	// riadny a opravný KV kódy opravy nemajú
	if f.Druh != "D" {
		clearFix(&f.KV)
	}
	return f, nil
}

func clearFix(k *KV) {
	for i := range k.A1 {
		k.A1[i].Fix = 0
	}
	for i := range k.A2 {
		k.A2[i].Fix = 0
	}
	for i := range k.B1 {
		k.B1[i].Fix = 0
	}
	for i := range k.B2 {
		k.B2[i].Fix = 0
	}
	for i := range k.B32 {
		k.B32[i].Fix = 0
	}
	for i := range k.C1 {
		k.C1[i].Fix = 0
	}
	for i := range k.C2 {
		k.C2[i].Fix = 0
	}
	if k.B31 != nil {
		k.B31.Fix = 0
	}
	k.D1.Fix, k.D2.Fix = 0, 0
	k.B31Old, k.D1Old, k.D2Old = nil, nil, nil
}

// applyLines použije dodatočné riadky na podaný stav: storno odoberie
// zhodný riadok, nový pridá.
func applyLines[T any](base, fix []T, fixOf func(T) int, clear func(*T)) ([]T, error) {
	out := append([]T(nil), base...)
	for _, f := range fix {
		if fixOf(f) == FixCancel {
			k := kvKey(f)
			found := false
			for i, b := range out {
				if kvKey(b) == k {
					out = append(out[:i], out[i+1:]...)
					found = true
					break
				}
			}
			if !found {
				return nil, errors.New("storno riadok dodatočného výkazu sa v predchádzajúcom výkaze nenašiel — načítajte najprv výkaz, ktorý opravuje")
			}
		}
	}
	for _, f := range fix {
		if fixOf(f) == FixNew {
			clear(&f)
			out = append(out, f)
		}
	}
	return out, nil
}

// ApplySupplement vráti stav KV po dodatočnom výkaze sup (iba riadky
// s kódom opravy) nad predchádzajúcim stavom base.
func ApplySupplement(base, sup KV) (KV, error) {
	var err error
	out := base
	set := func(e error) {
		if err == nil {
			err = e
		}
	}
	var e error
	out.A1, e = applyLines(base.A1, sup.A1, func(x KVLine) int { return x.Fix }, func(x *KVLine) { x.Fix = 0 })
	set(e)
	out.A2, e = applyLines(base.A2, sup.A2, func(x KVA2) int { return x.Fix }, func(x *KVA2) { x.Fix = 0 })
	set(e)
	out.B1, e = applyLines(base.B1, sup.B1, func(x KVLine) int { return x.Fix }, func(x *KVLine) { x.Fix = 0 })
	set(e)
	out.B2, e = applyLines(base.B2, sup.B2, func(x KVLine) int { return x.Fix }, func(x *KVLine) { x.Fix = 0 })
	set(e)
	out.B32, e = applyLines(base.B32, sup.B32, func(x KVB3) int { return x.Fix }, func(x *KVB3) { x.Fix = 0 })
	set(e)
	out.C1, e = applyLines(base.C1, sup.C1, func(x KVCorrection) int { return x.Fix }, func(x *KVCorrection) { x.Fix = 0 })
	set(e)
	out.C2, e = applyLines(base.C2, sup.C2, func(x KVCorrection) int { return x.Fix }, func(x *KVCorrection) { x.Fix = 0 })
	set(e)
	if sup.B31Old != nil {
		out.B31 = nil
	}
	if sup.B31 != nil {
		b := *sup.B31
		b.Fix = 0
		out.B31 = &b
	}
	sum := func(cur *KVSum, old *KVSum, next KVSum) {
		if old != nil {
			*cur = KVSum{}
		}
		if next.Fix == FixNew {
			next.Fix = 0
			*cur = next
		}
	}
	sum(&out.D1, sup.D1Old, sup.D1)
	sum(&out.D2, sup.D2Old, sup.D2)
	out.B31Old, out.D1Old, out.D2Old = nil, nil, nil
	return out, err
}
