package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
)

func TestVATSupplementAPI(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d", e.cid)
	e.do("PUT", base, `{"name":"Moja firma","vatPayer":true,"icdph":"SK2020372640","dic":"2020372640","city":"Zvolen"}`, 200, nil)
	var draft struct{ ID int64 }
	e.do("POST", base+"/invoices", `{"customer":{"name":"Odberateľ s.r.o.","icdph":"SK2020372640"},"issueDate":"2026-09-10","deliveryDate":"2026-09-10",
	  "lines":[{"description":"Montáž","quantity":"1","unit":"ks","unitPrice":"1000","vatRate":23}]}`, 201, &draft)
	e.do("POST", fmt.Sprintf("%s/invoices/%d/issue", base, draft.ID), nil, 200, nil)

	// riadne priznanie zaznamenané ako podané
	var filings []struct {
		Kind   string
		Result string
	}
	e.do("POST", base+"/vat/filings?period=2026-09&podane=2026-10-20", nil, 200, &filings)
	if len(filings) != 1 || filings[0].Kind != "riadne" || filings[0].Result != "230.00" {
		t.Fatalf("podanie: %+v", filings)
	}
	// ďalšia faktúra za september → dodatočné priznanie
	e.do("POST", base+"/invoices", `{"customer":{"name":"Odberateľ s.r.o.","icdph":"SK2020372640"},"issueDate":"2026-09-20","deliveryDate":"2026-09-20",
	  "lines":[{"description":"Servis","quantity":"1","unit":"ks","unitPrice":"100","vatRate":23}]}`, 201, &draft)
	e.do("POST", fmt.Sprintf("%s/invoices/%d/issue", base, draft.ID), nil, 200, nil)
	var ret struct {
		Kind string            `json:"kind"`
		Rows map[string]string `json:"rows"`
	}
	q := "/vat?period=2026-09&druh=dodatocne&zistene=2026-11-03&posledne=" + filings[0].Result
	e.do("GET", base+q, nil, 200, &ret)
	if ret.Kind != "dodatocne" || ret.Rows["r36"] != "23.00" || ret.Rows["r37"] != "23.00" {
		t.Fatalf("dodatočné: %+v", ret)
	}
	e.do("GET", base+"/vat?period=2026-09&druh=dodatocne", nil, 422, nil) // bez dátumu zistenia
	e.do("GET", base+"/vat?period=2026-09&druh=xyz", nil, 422, nil)
	e.do("GET", base+"/vat?period=2026-09&druh=dodatocne&zistene=x", nil, 400, nil)

	res, err := e.client.Get(e.srv.URL + base + "/vat/dph.xml" + strings.TrimPrefix(q, "/vat"))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), "<ddp>1</ddp>") || !strings.Contains(res.Header.Get("Content-Disposition"), "dph-2026-09-dodatocne.xml") {
		t.Fatalf("XML: %d %s", res.StatusCode, body)
	}
	// dodatočný KV: iba nová faktúra s kódom opravy 2
	res, err = e.client.Get(e.srv.URL + base + "/vat/kv.xml" + strings.TrimPrefix(q, "/vat"))
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(res.Body)
	res.Body.Close()
	kv := string(body)
	if res.StatusCode != 200 || !strings.Contains(kv, "<Druh>D</Druh>") || strings.Count(kv, "<A1 ") != 1 ||
		!strings.Contains(kv, `Z="100.00" D="23.00" S="23" KOpr="2"/>`) ||
		!strings.Contains(res.Header.Get("Content-Disposition"), "kv-dph-2026-09-dodatocne.xml") {
		t.Fatalf("dodatočný KV: %d %s", res.StatusCode, kv)
	}
	e.do("GET", base+"/vat/filings?period=2026-09", nil, 200, &filings)
	if len(filings) != 1 {
		t.Fatalf("zoznam: %+v", filings)
	}
	// po zázname dodatočného podania sa ďalší dodatočný KV nemá z čoho zostaviť
	e.do("POST", base+"/vat/filings"+strings.TrimPrefix(q, "/vat")+"&podane=2026-11-04", nil, 200, &filings)
	e.do("GET", base+"/vat/kv.xml"+strings.TrimPrefix(q, "/vat"), nil, 422, nil)
}

func TestVATSupplementKVWithoutFiling(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d", e.cid)
	e.do("PUT", base, `{"name":"Moja firma","vatPayer":true,"icdph":"SK2020372640","dic":"2020372640","city":"Zvolen"}`, 200, nil)
	var ret struct {
		KVFix     any    `json:"kvFix"`
		KVFixNote string `json:"kvFixNote"`
	}
	q := "?period=2026-09&druh=dodatocne&zistene=2026-11-03&posledne=0"
	e.do("GET", base+"/vat"+q, nil, 200, &ret)
	if ret.KVFix != nil || !strings.Contains(ret.KVFixNote, "nemá uložený podaný kontrolný výkaz") {
		t.Fatalf("%+v", ret)
	}
	e.do("GET", base+"/vat/kv.xml"+q, nil, 422, nil)
}

func TestVATLoadFiledKV(t *testing.T) {
	e := setup(t, edition.Desktop)
	base := fmt.Sprintf("/api/companies/%d", e.cid)
	e.do("PUT", base, `{"name":"Moja firma","vatPayer":true,"icdph":"SK2020372640","dic":"2020372640","city":"Zvolen"}`, 200, nil)
	var draft struct{ ID int64 }
	e.do("POST", base+"/invoices", `{"customer":{"name":"Odberateľ s.r.o.","icdph":"SK2120000003"},"issueDate":"2026-09-10","deliveryDate":"2026-09-10",
	  "lines":[{"description":"Montáž","quantity":"1","unit":"ks","unitPrice":"1000","vatRate":23}]}`, 201, &draft)
	e.do("POST", fmt.Sprintf("%s/invoices/%d/issue", base, draft.ID), nil, 200, nil)
	get := func(path string) string {
		res, err := e.client.Get(e.srv.URL + base + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return string(b)
	}
	// podaný riadny KV stiahnutý z Fiškusa (podanie sa vo Fiškuse nezaznamenalo)
	filed := get("/vat/kv.xml?period=2026-09")
	post := func(period, body string, status int) {
		req, _ := http.NewRequest("POST", e.srv.URL+base+"/vat/kv-podany?period="+period, strings.NewReader(body))
		req.Header.Set("X-Fiskus", "1")
		res, err := e.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != status {
			t.Fatalf("načítanie %s: %d %s", period, res.StatusCode, b)
		}
	}
	post("2026-08", filed, 422)                                                     // iné obdobie
	post("2026-09", strings.Replace(filed, "SK2020372640", "SK2020999999", 1), 422) // iná firma
	post("2026-09", "<html/>", 422)
	post("2026-09", filed, 200)
	// oprava: ďalšia faktúra → dodatočný KV s jedným novým riadkom
	e.do("POST", base+"/invoices", `{"customer":{"name":"Odberateľ s.r.o.","icdph":"SK2120000003"},"issueDate":"2026-09-20","deliveryDate":"2026-09-20",
	  "lines":[{"description":"Servis","quantity":"1","unit":"ks","unitPrice":"100","vatRate":23}]}`, 201, &draft)
	e.do("POST", fmt.Sprintf("%s/invoices/%d/issue", base, draft.ID), nil, 200, nil)
	q := "?period=2026-09&druh=dodatocne&zistene=2026-11-03&posledne=230.00"
	sup := get("/vat/kv.xml" + q)
	if !strings.Contains(sup, "<Druh>D</Druh>") || strings.Count(sup, "<A1 ") != 1 || !strings.Contains(sup, `KOpr="2"`) {
		t.Fatalf("dodatočný KV: %s", sup)
	}
	var ret struct {
		KVBase string `json:"kvBase"`
	}
	e.do("GET", base+"/vat"+q, nil, 200, &ret)
	if !strings.Contains(ret.KVBase, "súbor XML") {
		t.Fatalf("základ: %q", ret.KVBase)
	}
	// načítaný aj dodatočný → nič na opravu
	post("2026-09", sup, 200)
	e.do("GET", base+"/vat/kv.xml"+q, nil, 422, nil)
}
