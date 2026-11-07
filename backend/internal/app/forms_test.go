package app

import (
	"strings"
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
	"github.com/fiskus-online/fiskus/backend/internal/forms"
	m "github.com/fiskus-online/fiskus/backend/internal/money"
)

func TestFormsWithoutCompany(t *testing.T) {
	a, _ := newApp(t, edition.Desktop)
	cat, err := a.FormCatalog(ctx, 0)
	if err != nil || len(cat.Forms) < 5 || cat.Issuer != "" {
		t.Fatalf("katalóg bez firmy: %+v %v", cat, err)
	}
	p, err := a.PreviewForm(ctx, 0, FormPreviewRequest{FormKey: "ppd", Values: forms.Values{"date": "2026-10-29", "partner": "Ján", "purpose": "Vklad", "amount": "123", "rate": "23"}})
	if err != nil || len(p.Errors) != 0 || p.Computed.VAT.String() != "23.00" {
		t.Fatalf("náhľad: %+v %v", p, err)
	}
	p, _ = a.PreviewForm(ctx, 0, FormPreviewRequest{FormKey: "ppd", Values: forms.Values{}})
	if len(p.Errors) != 4 {
		t.Fatalf("chyby nevyplneného: %+v", p.Errors)
	}
	if _, err := a.PreviewForm(ctx, 0, FormPreviewRequest{FormKey: "vlastne-x"}); !fieldErr(err, "formKey") {
		t.Fatalf("vlastné bez firmy: %v", err)
	}
	// rozpracovaná definícia vlastného tlačiva
	p, err = a.PreviewForm(ctx, 0, FormPreviewRequest{Form: &forms.Form{Name: "Protokol", Items: true}, Items: []forms.Item{{Desc: "A", Qty: m.MustParse("2"), Price: m.MustParse("3")}}})
	if err != nil || p.Computed.ItemsTotal.String() != "6.00" {
		t.Fatalf("náhľad definície: %+v %v", p, err)
	}
}

func TestFormsInCompany(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	cat, _ := a.FormCatalog(ctx, c.ID)
	if !strings.HasPrefix(cat.Issuer, "Moja firma") || !strings.Contains(cat.Issuer, "IČO 31322832") {
		t.Fatalf("hlavička: %q", cat.Issuer)
	}

	// vlastné tlačivo
	def, err := a.SaveFormDef(ctx, c.ID, forms.Form{Name: "Preberací protokol", Fields: []forms.Field{{Label: "Zákazka"}, {Label: "Dátum", Type: forms.Date, Required: true}}})
	if err != nil || def.Key != "vlastne-preberaci-protokol" || def.Fields[1].Key != "datum" {
		t.Fatalf("vlastné tlačivo: %+v %v", def, err)
	}
	if again, _ := a.SaveFormDef(ctx, c.ID, forms.Form{Name: "Preberací protokol", Fields: []forms.Field{{Label: "X"}}}); again.Key != "vlastne-preberaci-protokol-2" {
		t.Fatalf("druhé s rovnakým názvom: %s", again.Key)
	}
	if _, err := a.SaveFormDef(ctx, c.ID, forms.Form{Name: "Bez polí"}); !fieldErr(err, "fields") {
		t.Fatalf("bez polí: %v", err)
	}
	cat, _ = a.FormCatalog(ctx, c.ID)
	if _, ok := forms.Find(cat.Forms, def.Key); !ok {
		t.Fatal("vlastné tlačivo chýba v katalógu")
	}
	doc, err := a.SaveFormDoc(ctx, c.ID, forms.Doc{FormKey: def.Key, Values: forms.Values{"zakazka": "Z-12"}})
	if err != nil || doc.Title != "Preberací protokol" || doc.Updated.String() != "2026-10-29" {
		t.Fatalf("uloženie vlastného: %+v %v", doc, err)
	}
	if err := a.DeleteFormDef(ctx, c.ID, def.Key); !fieldErr(err, "key") {
		t.Fatalf("zmazanie použitého tlačiva: %v", err)
	}

	// vzor a PPD z neho
	tpl, err := a.SaveFormDoc(ctx, c.ID, forms.Doc{FormKey: "ppd", Title: "Tržba z predajne", Template: true, Values: forms.Values{"issuer": cat.Issuer, "purpose": "Tržba"}})
	if err != nil || !tpl.Template {
		t.Fatal(err)
	}
	if _, err := a.PostFormDoc(ctx, c.ID, tpl.ID, 0); !fieldErr(err, "template") {
		t.Fatalf("vzor do pokladne: %v", err)
	}
	vals := forms.Values{}
	for k, v := range tpl.Values {
		vals[k] = v
	}
	vals["date"], vals["partner"], vals["amount"], vals["rate"] = "2026-10-29", "Ján Novák\nHlavná 1, Zvolen", "123", "23"
	ppd, err := a.SaveFormDoc(ctx, c.ID, forms.Doc{FormKey: "ppd", Values: vals})
	if err != nil || ppd.Title != "Príjmový pokladničný doklad – Ján Novák" {
		t.Fatalf("PPD: %+v %v", ppd, err)
	}
	posted, err := a.PostFormDoc(ctx, c.ID, ppd.ID, 0)
	if err != nil || posted.CashID == 0 || posted.CashNumber == "" || posted.Values["number"] != posted.CashNumber {
		t.Fatalf("zápis do pokladne: %+v %v", posted, err)
	}
	cd, err := a.Store.GetCashDocument(ctx, c.ID, posted.CashID)
	if err != nil || cd.Partner != "Ján Novák, Hlavná 1, Zvolen" || cd.Amount.String() != "123.00" || cd.Tax == nil || cd.Tax.Rows[0].VAT.String() != "23.00" {
		t.Fatalf("pokladničný doklad: %+v %v", cd, err)
	}
	if _, err := a.SaveFormDoc(ctx, c.ID, posted); !fieldErr(err, "cashId") {
		t.Fatalf("úprava zapísaného: %v", err)
	}
	if err := a.DeleteFormDoc(ctx, c.ID, posted.ID); !fieldErr(err, "cashId") {
		t.Fatalf("zmazanie zapísaného: %v", err)
	}
	if _, err := a.PostFormDoc(ctx, c.ID, posted.ID, 0); !fieldErr(err, "cashId") {
		t.Fatalf("druhý zápis: %v", err)
	}

	// VPD nad zostatok pokladne neprejde
	vpd, _ := a.SaveFormDoc(ctx, c.ID, forms.Doc{FormKey: "vpd", Values: forms.Values{"date": "2026-10-29", "partner": "Dodávateľ", "purpose": "Nákup", "amount": "500"}})
	if _, err := a.PostFormDoc(ctx, c.ID, vpd.ID, 0); !fieldErr(err, "amount") {
		t.Fatalf("záporný zostatok: %v", err)
	}
	// dodací list nie je pokladničný doklad
	dl, _ := a.SaveFormDoc(ctx, c.ID, forms.Doc{FormKey: "dodaci-list", Values: forms.Values{}})
	if _, err := a.PostFormDoc(ctx, c.ID, dl.ID, 0); !fieldErr(err, "formKey") {
		t.Fatalf("dodací list do pokladne: %v", err)
	}
	if err := a.DeleteFormDoc(ctx, c.ID, dl.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := a.FormDocs(ctx, c.ID); len(list) != 4 {
		t.Fatalf("uložené: %d", len(list))
	}
}
