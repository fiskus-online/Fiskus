package vatreturn

import (
	"strings"
	"testing"
)

func TestSupplementKV(t *testing.T) {
	a := KVLine{Partner: "SK2020000001", Name: "Odberateľ", Number: "2026001", Date: d("2026-10-05"), Base: m("100"), VAT: m("23"), Rate: 23, ID: 1}
	b := KVLine{Partner: "SK2020000002", Number: "2026002", Date: d("2026-10-06"), Base: m("200"), VAT: m("46"), Rate: 23, ID: 2}
	c := KVLine{Partner: "SK2020000003", Number: "2026003", Date: d("2026-10-07"), Base: m("50"), VAT: m("11.50"), Rate: 23, ID: 3}
	bFixed := b
	bFixed.Base, bFixed.VAT = m("210"), m("48.30")
	renamed := a
	renamed.Name, renamed.ID = "Iný názov", 99 // iba zobrazenie — nie je oprava
	d2 := KVSum{Base: m("10"), VAT: m("2.30"), Count: 1}
	d2b := KVSum{Base: m("20"), VAT: m("4.60"), Count: 2}

	cases := []struct {
		name   string
		filed  KV
		now    KV
		a1     []string // "číslo:kód"
		d2     string   // "storno/nový" počty
		noDiff bool
	}{
		{"bez zmeny", KV{A1: []KVLine{a, b}}, KV{A1: []KVLine{b, renamed}}, nil, "", true},
		{"chýbajúca faktúra", KV{A1: []KVLine{a}}, KV{A1: []KVLine{a, c}}, []string{"2026003:2"}, "", false},
		{"faktúra navyše", KV{A1: []KVLine{a, c}}, KV{A1: []KVLine{a}}, []string{"2026003:1"}, "", false},
		{"opravená suma", KV{A1: []KVLine{a, b}}, KV{A1: []KVLine{a, bFixed}}, []string{"2026002:1", "2026002:2"}, "", false},
		{"rovnaké riadky dvakrát", KV{A1: []KVLine{a, a}}, KV{A1: []KVLine{a}}, []string{"2026001:1"}, "", false},
		{"D.2 zmenená", KV{D2: d2}, KV{D2: d2b}, nil, "1/1", false},
		{"D.2 pribudla", KV{}, KV{D2: d2}, nil, "0/1", false},
		{"D.2 zmizla", KV{D2: d2}, KV{}, nil, "1/0", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SupplementKV(tc.filed, tc.now)
			if tc.noDiff {
				if err != ErrKVUnchanged {
					t.Fatalf("chcem ErrKVUnchanged: %v %+v", err, got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var a1 []string
			for _, l := range got.A1 {
				a1 = append(a1, l.Number+":"+string(rune('0'+l.Fix)))
			}
			if strings.Join(a1, ",") != strings.Join(tc.a1, ",") {
				t.Fatalf("A1 %v, chcem %v", a1, tc.a1)
			}
			if tc.d2 != "" {
				old, cur := 0, 0
				if got.D2Old != nil && got.D2Old.Fix == FixCancel {
					old = 1
				}
				if got.D2.Count > 0 && got.D2.Fix == FixNew {
					cur = 1
				}
				if s := string(rune('0'+old)) + "/" + string(rune('0'+cur)); s != tc.d2 {
					t.Fatalf("D2 %s, chcem %s", s, tc.d2)
				}
			}
		})
	}
}

func TestSupplementKVXML(t *testing.T) {
	a := KVLine{Partner: "SK2020000001", Number: "2026001", Date: d("2026-10-05"), Base: m("100"), VAT: m("23"), Rate: 23}
	fixed := a
	fixed.Base, fixed.VAT = m("110"), m("25.30")
	b31 := &KVB3{Base: m("80"), VAT: m("16.20"), Deducted: m("16.20"), Count: 3}
	b31b := &KVB3{Base: m("90"), VAT: m("18.50"), Deducted: m("18.50"), Count: 4}
	kv, err := SupplementKV(KV{A1: []KVLine{a}, B31: b31}, KV{A1: []KVLine{fixed}, B31: b31b})
	if err != nil {
		t.Fatal(err)
	}
	r := Return{Period: Period{Year: 2026, Month: 10}, Kind: KindSupplementary, KV: kv}
	x := string(KVXML(r, Taxpayer{Name: "Firma", ICDPH: "SK2020372640", City: "Zvolen"}))
	for _, want := range []string{
		"<Druh>D</Druh>",
		`<A1 Odb="SK2020000001" F="2026001" Den="2026-10-05" Z="100.00" D="23.00" S="23" KOpr="1"/>`,
		`<A1 Odb="SK2020000001" F="2026001" Den="2026-10-05" Z="110.00" D="25.30" S="23" KOpr="2"/>`,
		`<B31 Z="80.00" D="16.20" O="16.20" KOpr="1"/><B31 Z="90.00" D="18.50" O="18.50" KOpr="2"/>`,
	} {
		if !strings.Contains(x, want) {
			t.Fatalf("chýba %s v\n%s", want, x)
		}
	}
	if strings.Contains(x, "<D1") || strings.Contains(x, "<D2") {
		t.Fatalf("nezmenené súhrny nemajú byť v dodatočnom KV:\n%s", x)
	}
	// riadny KV kód opravy nemá
	r.Kind, r.KV = KindRegular, KV{A1: []KVLine{a}}
	if x := string(KVXML(r, Taxpayer{Name: "Firma", ICDPH: "SK2020372640", City: "Zvolen"})); strings.Contains(x, "KOpr") || !strings.Contains(x, "<Druh>R</Druh>") {
		t.Fatalf("riadny KV:\n%s", x)
	}
}
