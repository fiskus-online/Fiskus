package app

import (
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

func TestTaxReturn(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	c.Name, c.DIC, c.Street, c.Zip, c.City = "Pavol Krnáč – elektroservis", "1012345678", "Námestie SNP 1", "974 01", "Banská Bystrica"
	if err := a.Store.UpdateCompany(ctx, c); err != nil {
		t.Fatal(err)
	}
	tr, err := a.TaxReturnFor(ctx, c.ID, 2026)
	if err != nil {
		t.Fatal(err)
	}
	tp := tr.Input.Taxpayer
	if tp.Name != "Pavol" || tp.Surname != "Krnáč" || tp.Street != "Námestie SNP" || tp.Number != "1" || tp.DIC != "1012345678" || tr.Form != "DPFOBv25" {
		t.Fatalf("predvyplnenie %+v", tp)
	}
	if len(tr.Years) < 2 || tr.Years[0] != 2026 {
		t.Errorf("roky %v", tr.Years)
	}
	// chyby polí
	bad := tr.Input
	bad.Taxpayer.BirthDate, bad.Taxpayer.NACE, bad.NGOICO, bad.Prepaid = "1.1.80", "432", "12", money.MustParse("-1")
	_, err = a.SaveTaxReturn(ctx, c.ID, 2026, bad)
	for _, f := range []string{"taxpayer.birthDate", "taxpayer.nace", "ngoIco", "prepaid"} {
		if !fieldErr(err, f) {
			t.Errorf("chýba chyba %s: %v", f, err)
		}
	}
	if _, err := a.SaveTaxReturn(ctx, c.ID, 2019, tr.Input); !fieldErr(err, "year") {
		t.Errorf("rok bez zákonných hodnôt: %v", err)
	}
	// uloženie: sumy z kníh sa neukladajú, zadané údaje áno
	in := tr.Input
	in.Taxpayer.NACE, in.Prepaid, in.Income = "43210", money.MustParse("500"), money.MustParse("999999")
	saved, err := a.SaveTaxReturn(ctx, c.ID, 2026, in)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Input.Prepaid.Equal(money.MustParse("500")) || !saved.Input.Income.IsZero() || saved.Input.Taxpayer.NACE != "43210" {
		t.Fatalf("uložené %+v", saved.Input)
	}
	// bez príjmov je daň nula a preddavok sa vráti
	if !saved.Rows["136"].Equal(money.MustParse("500")) {
		t.Errorf("preplatok %s", saved.Rows["136"])
	}
	data, name, err := a.TaxReturnXML(ctx, c.ID, 2026)
	if err != nil || name != "DPFOB-2026.xml" || !strings.Contains(string(data), "<k1>43</k1>") || !strings.Contains(string(data), "<r133>500.00</r133>") {
		t.Fatalf("XML %s %v", name, err)
	}
}
