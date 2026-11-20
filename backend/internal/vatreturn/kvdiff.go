package vatreturn

import (
	"encoding/json"
	"errors"
)

// Dodatočný kontrolný výkaz (§ 78a ods. 9 zákona o DPH, poučenie KVDPH):
// obsahuje iba opravované údaje voči poslednému platnému KV za obdobie.
//   - nesprávny riadok: pôvodný riadok s kódom opravy 1 (storno) a správny
//     riadok s kódom 2;
//   - riadok, ktorý tam nemal byť: pôvodný riadok s kódom 1;
//   - chýbajúci riadok: nový riadok s kódom 2.
// Fiškus porovná KV uložený pri zázname podania s KV z aktuálnych dokladov:
// riadky, ktoré sa nezmenili, vypadnú; zmenený riadok je storno pôvodného
// a nový riadok — to presne zodpovedá pravidlám vyššie.

// Kódy opravy (atribút KOpr).
const (
	FixCancel = 1 // storno pôvodného riadku
	FixNew    = 2 // nový alebo správny riadok
)

// ErrKVUnchanged: dodatočný KV nemá čo opraviť.
var ErrKVUnchanged = errors.New("kontrolný výkaz z aktuálnych dokladov sa zhoduje s podaným — dodatočný kontrolný výkaz netreba podávať")

// kvKey je porovnávací kľúč riadku: iba údaje, ktoré idú do XML (bez
// názvu partnera, ID dokladu, počtu dokladov a kódu opravy).
func kvKey(v any) string {
	switch x := v.(type) {
	case KVLine:
		x.Name, x.Kind, x.ID, x.Fix = "", "", 0, 0
		v = x
	case KVA2:
		x.Name, x.Label, x.ID, x.Fix = "", "", 0, 0
		v = x
	case KVB3:
		x.Name, x.Count, x.Fix = "", 0, 0
		v = x
	case KVCorrection:
		x.Name, x.ID, x.Fix = "", 0, 0
		v = x
	case KVSum:
		x.Count, x.Fix = 0, 0
		v = x
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// diffLines vráti storno riadkov, ktoré v novom KV nie sú, a nové riadky,
// ktoré v pôvodnom neboli (ako multimnožina — rovnaké riadky sa párujú).
func diffLines[T any](old, now []T, set func(*T, int)) []T {
	left := map[string]int{}
	for _, x := range now {
		left[kvKey(x)]++
	}
	out := []T{}
	for _, x := range old {
		k := kvKey(x)
		if left[k] > 0 {
			left[k]--
			continue
		}
		set(&x, FixCancel)
		out = append(out, x)
	}
	right := map[string]int{}
	for _, x := range old {
		right[kvKey(x)]++
	}
	for _, x := range now {
		k := kvKey(x)
		if right[k] > 0 {
			right[k]--
			continue
		}
		set(&x, FixNew)
		out = append(out, x)
	}
	return out
}

// SupplementKV vytvorí dodatočný KV: rozdiel medzi podaným (filed) a
// aktuálnym (now) kontrolným výkazom.
func SupplementKV(filed, now KV) (KV, error) {
	line := func(x *KVLine, f int) { x.Fix = f }
	a2 := func(x *KVA2, f int) { x.Fix = f }
	b3 := func(x *KVB3, f int) { x.Fix = f }
	corr := func(x *KVCorrection, f int) { x.Fix = f }
	out := KV{
		A1:  diffLines(filed.A1, now.A1, line),
		A2:  diffLines(filed.A2, now.A2, a2),
		B1:  diffLines(filed.B1, now.B1, line),
		B2:  diffLines(filed.B2, now.B2, line),
		B32: diffLines(filed.B32, now.B32, b3),
		C1:  diffLines(filed.C1, now.C1, corr),
		C2:  diffLines(filed.C2, now.C2, corr),
	}
	// súhrnné riadky: najviac storno pôvodného a nový
	if filed.B31 == nil || now.B31 == nil || kvKey(*filed.B31) != kvKey(*now.B31) {
		if filed.B31 != nil {
			x := *filed.B31
			x.Fix = FixCancel
			out.B31Old = &x
		}
		if now.B31 != nil {
			x := *now.B31
			x.Fix = FixNew
			out.B31 = &x
		}
	}
	sum := func(old, cur KVSum) (*KVSum, KVSum) {
		if (old.Count > 0) == (cur.Count > 0) && (old.Count == 0 || kvKey(old) == kvKey(cur)) {
			return nil, KVSum{}
		}
		var o *KVSum
		if old.Count > 0 {
			x := old
			x.Fix = FixCancel
			o = &x
		}
		n := KVSum{}
		if cur.Count > 0 {
			n = cur
			n.Fix = FixNew
		}
		return o, n
	}
	out.D1Old, out.D1 = sum(filed.D1, now.D1)
	out.D2Old, out.D2 = sum(filed.D2, now.D2)
	if out.Empty() {
		return out, ErrKVUnchanged
	}
	return out, nil
}

// Empty: KV bez jediného riadku.
func (k KV) Empty() bool {
	return len(k.A1)+len(k.A2)+len(k.B1)+len(k.B2)+len(k.B32)+len(k.C1)+len(k.C2) == 0 &&
		k.B31 == nil && k.B31Old == nil && k.D1.Count == 0 && k.D1Old == nil && k.D2.Count == 0 && k.D2Old == nil
}
