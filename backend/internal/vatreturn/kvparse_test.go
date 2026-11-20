package vatreturn

import (
	"strings"
	"testing"
)

func sampleKV() KV {
	q := m("100")
	return KV{
		A1:  []KVLine{{Partner: "SK2020000001", Name: "Odberateľ", Number: "2026001", Date: d("2026-10-05"), Base: m("100"), VAT: m("23"), Rate: 23, Kind: "invoice", ID: 1}},
		A2:  []KVA2{{Partner: "SK2020000002", Number: "2026002", Date: d("2026-10-06"), Base: m("4000"), Goods: "MT", Quantity: &q, Unit: "ks", Label: "mobil", ID: 2}},
		B1:  []KVLine{{Partner: "DE123456789", Number: "R-9", Date: d("2026-10-07"), Base: m("50"), VAT: m("11.50"), Rate: 23, Deducted: m("11.50")}},
		B2:  []KVLine{{Partner: "SK2020000003", Number: "P/77", Date: d("2026-10-08"), Base: m("200"), VAT: m("46"), Rate: 23, Deducted: m("46")}},
		B31: &KVB3{Base: m("80"), VAT: m("16.20"), Deducted: m("16.20"), Count: 3},
		C1:  []KVCorrection{{Partner: "SK2020000001", Number: "DB1", Original: "2026001", BaseDiff: m("-10"), VATDiff: m("-2.30"), Rate: 23}},
		C2:  []KVCorrection{{Partner: "SK2020000003", Number: "DOB5", Original: "P/77", BaseDiff: m("-20"), VATDiff: m("-4.60"), Rate: 23, DeductedDiff: m("-4.60")}},
		D1:  KVSum{Turnover: m("149"), Base: m("100"), VAT: m("23"), BaseReduced: m("20"), VATReduced: m("1"), Count: 4},
		D2:  KVSum{Base: m("10"), VAT: m("2.30"), Count: 1},
	}
}

var tp = Taxpayer{Name: "Firma", ICDPH: "SK2020372640", City: "Zvolen"}

func TestParseKVXMLRoundTrip(t *testing.T) {
	p := Period{Year: 2026, Month: 10}
	kv := sampleKV()
	f, err := ParseKVXML(KVXML(Return{Period: p, Kind: KindRegular, KV: kv}, tp))
	if err != nil {
		t.Fatal(err)
	}
	if f.ICDPH != "SK2020372640" || f.Period != p || f.Druh != "R" {
		t.Fatalf("hlavička %+v", f)
	}
	if _, err := SupplementKV(f.KV, kv); err != ErrKVUnchanged {
		t.Fatalf("načítaný KV sa nezhoduje s pôvodným: %v", err)
	}
	// štvrťrok
	f, err = ParseKVXML(KVXML(Return{Period: Period{Year: 2026, Quarter: 3}, Kind: KindCorrective, KV: kv}, tp))
	if err != nil || f.Period.Quarter != 3 || f.Druh != "O" {
		t.Fatalf("štvrťrok %+v %v", f.Period, err)
	}
}

func TestParseKVXMLErrors(t *testing.T) {
	cases := []struct {
		name, xml, want string
	}{
		{"nie XML", "PDF", "nie je kontrolný výkaz"},
		{"iné tlačivo", `<DPHv25><a/></DPHv25>`, "nie je kontrolný výkaz"},
		{"zlý druh", `<KVDPH_2025><Identifikacia><Druh>X</Druh><Obdobie><Rok>2026</Rok><Mesiac>1</Mesiac></Obdobie></Identifikacia></KVDPH_2025>`, "druh"},
		{"zlá suma", `<KVDPH_2025><Identifikacia><Druh>R</Druh><Obdobie><Rok>2026</Rok><Mesiac>1</Mesiac></Obdobie></Identifikacia><Transakcie><A1 F="1" Den="2026-01-02" Z="x" D="1" S="23"/></Transakcie></KVDPH_2025>`, "suma"},
	}
	for _, c := range cases {
		if _, err := ParseKVXML([]byte(c.xml)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

func TestApplySupplementKV(t *testing.T) {
	filed := sampleKV()
	now := sampleKV()
	now.A1[0].Base, now.A1[0].VAT = m("110"), m("25.30")
	now.A1 = append(now.A1, KVLine{Partner: "SK2020000009", Number: "2026009", Date: d("2026-10-20"), Base: m("1"), VAT: m("0.23"), Rate: 23})
	now.B2 = nil
	now.D2 = KVSum{Base: m("30"), VAT: m("6.90"), Count: 2}
	now.B31 = nil
	sup, err := SupplementKV(filed, now)
	if err != nil {
		t.Fatal(err)
	}
	// dodatočný KV cez XML a späť, potom použitý na podaný stav = aktuálny stav
	f, err := ParseKVXML(KVXML(Return{Period: Period{Year: 2026, Month: 10}, Kind: KindSupplementary, KV: sup}, tp))
	if err != nil || f.Druh != "D" {
		t.Fatalf("%v %s", err, f.Druh)
	}
	after, err := ApplySupplement(filed, f.KV)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SupplementKV(after, now); err != ErrKVUnchanged {
		t.Fatalf("po dodatočnom sa stav nezhoduje: %v", err)
	}
	// storno riadok, ktorý v podanom KV nie je
	if _, err := ApplySupplement(KV{}, f.KV); err == nil {
		t.Fatal("chcem chybu pri storne neexistujúceho riadku")
	}
}
