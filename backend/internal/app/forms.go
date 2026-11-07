package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/cash"
	"github.com/fiskus-online/fiskus/backend/internal/forms"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// Tlačivá: vstavané fungujú aj bez firmy (vyplniť a vytlačiť). Vo firme sa
// vyplnené tlačivá ukladajú, z uložených sa robia vzory a pokladničné
// doklady sa zapíšu do pokladne ako účtovný doklad.

func formDefsKey(companyID int64) string { return fmt.Sprintf("forms.defs.%d", companyID) }

// FormCatalog sú tlačivá na výber a predvyplnenie hlavičky firmy.
type FormCatalog struct {
	Forms  []forms.Form `json:"forms"`
	Issuer string       `json:"issuer"` // „Organizácia“ z údajov firmy
	Today  invoice.Date `json:"today"`
}

// FormCatalog vráti vstavané tlačivá a vo firme aj vlastné (companyID 0 = bez firmy).
func (a *App) FormCatalog(ctx context.Context, companyID int64) (FormCatalog, error) {
	out := FormCatalog{Forms: forms.Builtin(), Today: a.Today()}
	if companyID == 0 {
		return out, nil
	}
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return out, err
	}
	own, err := a.customForms(ctx, companyID)
	if err != nil {
		return out, err
	}
	out.Forms = append(out.Forms, own...)
	c, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return out, err
	}
	out.Issuer = issuerText(c)
	return out, nil
}

// issuerText je hlavička firmy na tlačivo: názov, adresa, IČO, DIČ.
func issuerText(c store.Company) string {
	lines := []string{c.Name, c.Street, strings.TrimSpace(c.Zip + " " + c.City)}
	var ids []string
	if c.ICO != "" {
		ids = append(ids, "IČO "+c.ICO)
	}
	if c.DIC != "" {
		ids = append(ids, "DIČ "+c.DIC)
	}
	if c.ICDPH != "" {
		ids = append(ids, "IČ DPH "+c.ICDPH)
	}
	lines = append(lines, strings.Join(ids, ", "))
	var out []string
	for _, l := range lines {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

func (a *App) customForms(ctx context.Context, companyID int64) ([]forms.Form, error) {
	out := []forms.Form{}
	raw, err := a.Store.GetSetting(ctx, formDefsKey(companyID))
	if errors.Is(err, store.ErrNotFound) || (err == nil && raw == "") {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	return out, json.Unmarshal([]byte(raw), &out)
}

func (a *App) formByKey(ctx context.Context, companyID int64, key string) (forms.Form, error) {
	if f, ok := forms.Find(forms.Builtin(), key); ok {
		return f, nil
	}
	if companyID != 0 {
		own, err := a.customForms(ctx, companyID)
		if err != nil {
			return forms.Form{}, err
		}
		if f, ok := forms.Find(own, key); ok {
			return f, nil
		}
	}
	return forms.Form{}, vErr("formKey", "Tlačivo neexistuje")
}

// SaveFormDef uloží vlastné tlačivo (nové dostane kľúč z názvu).
func (a *App) SaveFormDef(ctx context.Context, companyID int64, f forms.Form) (forms.Form, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return f, err
	}
	own, err := a.customForms(ctx, companyID)
	if err != nil {
		return f, err
	}
	f.Normalize()
	idx := -1
	for i, o := range own {
		if o.Key == f.Key && f.Key != "" {
			idx = i
		}
	}
	if idx < 0 { // nové: kľúč z názvu, nesmie kolidovať so vstavaným ani iným
		base := "vlastne-" + forms.Slug(f.Name)
		if forms.Slug(f.Name) == "" {
			base = "vlastne"
		}
		taken := func(k string) bool {
			_, b := forms.Find(forms.Builtin(), k)
			_, o := forms.Find(own, k)
			return b || o
		}
		f.Key = base
		for n := 2; taken(f.Key); n++ {
			f.Key = fmt.Sprintf("%s-%d", base, n)
		}
	}
	if err := f.Validate(); err != nil {
		return f, err
	}
	var before any
	if idx >= 0 {
		before = own[idx]
		own[idx] = f
	} else {
		own = append(own, f)
	}
	raw, _ := json.Marshal(own)
	if err := a.Store.SetSetting(ctx, formDefsKey(companyID), string(raw)); err != nil {
		return f, err
	}
	a.record(ctx, companyID, audit.CatSettings, "update", "form_def", 0, "Vlastné tlačivo "+f.Name, before, f)
	return f, nil
}

// DeleteFormDef zmaže vlastné tlačivo, ak podľa neho nie je nič uložené.
func (a *App) DeleteFormDef(ctx context.Context, companyID int64, key string) error {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return err
	}
	own, err := a.customForms(ctx, companyID)
	if err != nil {
		return err
	}
	f, ok := forms.Find(own, key)
	if !ok {
		return store.ErrNotFound
	}
	docs, err := a.Store.ListFormDocs(ctx, companyID)
	if err != nil {
		return err
	}
	for _, d := range docs {
		if d.FormKey == key {
			return vErr("key", "Podľa tlačiva "+f.Name+" sú uložené vyplnené tlačivá alebo vzory — najprv ich zmažte")
		}
	}
	rest := own[:0]
	for _, o := range own {
		if o.Key != key {
			rest = append(rest, o)
		}
	}
	raw, _ := json.Marshal(rest)
	if err := a.Store.SetSetting(ctx, formDefsKey(companyID), string(raw)); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatSettings, "delete", "form_def", 0, "Zmazané vlastné tlačivo "+f.Name, f, nil)
	return nil
}

// FormPreviewRequest je tlačivo na prepočet: kľúč, alebo rozpracovaná
// definícia vlastného tlačiva (náhľad pred uložením).
type FormPreviewRequest struct {
	FormKey string       `json:"formKey"`
	Form    *forms.Form  `json:"form,omitempty"`
	Values  forms.Values `json:"values"`
	Items   []forms.Item `json:"items"`
}

// FormPreview sú dopočítané sumy a chyby polí (náhľad sa ukáže aj s chybami).
type FormPreview struct {
	Computed forms.Computed       `json:"computed"`
	Errors   []invoice.FieldError `json:"errors"`
}

// PreviewForm overí hodnoty a dopočíta sumy (DPH, položky). Bez firmy iba vstavané tlačivá.
func (a *App) PreviewForm(ctx context.Context, companyID int64, req FormPreviewRequest) (FormPreview, error) {
	out := FormPreview{Errors: []invoice.FieldError{}}
	if companyID != 0 {
		if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
			return out, err
		}
	}
	var f forms.Form
	if req.Form != nil {
		f = *req.Form
		f.Normalize()
	} else {
		var err error
		if f, err = a.formByKey(ctx, companyID, req.FormKey); err != nil {
			return out, err
		}
	}
	c, err := forms.Compute(f, req.Values, req.Items)
	out.Computed = c
	var ve *invoice.ValidationError
	if errors.As(err, &ve) {
		out.Errors = ve.Fields
	} else if err != nil {
		return out, err
	}
	return out, nil
}

// FormDocs vráti uložené tlačivá a vzory firmy.
func (a *App) FormDocs(ctx context.Context, companyID int64) ([]forms.Doc, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return nil, err
	}
	return a.Store.ListFormDocs(ctx, companyID)
}

// FormDoc vráti jedno uložené tlačivo.
func (a *App) FormDoc(ctx context.Context, companyID, id int64) (forms.Doc, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesRead); err != nil {
		return forms.Doc{}, err
	}
	return a.Store.GetFormDoc(ctx, companyID, id)
}

func errFormPosted(d forms.Doc) error {
	return vErr("cashId", "Tlačivo je zapísané v pokladni ako "+d.CashNumber+" — opravuje sa v Pokladni stornom")
}

// SaveFormDoc uloží vyplnené tlačivo alebo vzor. Rozpracované tlačivo môže
// mať nevyplnené povinné polia; zapísané do pokladne sa už nemení.
func (a *App) SaveFormDoc(ctx context.Context, companyID int64, d forms.Doc) (forms.Doc, error) {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return d, err
	}
	var before *forms.Doc
	if d.ID != 0 {
		old, err := a.Store.GetFormDoc(ctx, companyID, d.ID)
		if err != nil {
			return d, err
		}
		if old.CashID != 0 {
			return d, errFormPosted(old)
		}
		before = &old
	}
	f, err := a.formByKey(ctx, companyID, d.FormKey)
	if err != nil {
		return d, err
	}
	d.CashID, d.CashNumber = 0, ""
	if d.Values == nil {
		d.Values = forms.Values{}
	}
	if d.Items == nil {
		d.Items = []forms.Item{}
	}
	if !f.Items {
		d.Items = []forms.Item{}
	}
	d.Title = strings.TrimSpace(d.Title)
	if d.Title == "" {
		d.Title = defaultTitle(f, d.Values)
	}
	d.Updated = a.Today()
	if err := a.Store.SaveFormDoc(ctx, companyID, &d); err != nil {
		return d, err
	}
	what := "Tlačivo"
	if d.Template {
		what = "Vzor tlačiva"
	}
	a.record(ctx, companyID, audit.CatInvoices, "update", "form_doc", d.ID, fmt.Sprintf("%s %s: %s", what, f.Name, d.Title), before, d)
	return d, nil
}

// defaultTitle: „Dodací list 12 – Hotel Lesná“ z čísla a prvého riadku partnera.
func defaultTitle(f forms.Form, v forms.Values) string {
	parts := []string{f.Name}
	for _, fl := range f.Fields {
		switch fl.Role {
		case "number":
			if s := strings.TrimSpace(v[fl.Key]); s != "" {
				parts[0] += " " + s
			}
		case "partner":
			if s := strings.TrimSpace(strings.SplitN(v[fl.Key], "\n", 2)[0]); s != "" {
				parts = append(parts, s)
			}
		}
	}
	return strings.Join(parts, " – ")
}

// DeleteFormDoc zmaže uložené tlačivo (nie zapísané do pokladne).
func (a *App) DeleteFormDoc(ctx context.Context, companyID, id int64) error {
	if err := a.Require(ctx, companyID, auth.PermInvoicesWrite); err != nil {
		return err
	}
	d, err := a.Store.GetFormDoc(ctx, companyID, id)
	if err != nil {
		return err
	}
	if d.CashID != 0 {
		return errFormPosted(d)
	}
	if err := a.Store.DeleteFormDoc(ctx, companyID, id); err != nil {
		return err
	}
	a.record(ctx, companyID, audit.CatInvoices, "delete", "form_doc", id, "Zmazané tlačivo "+d.Title, d, nil)
	return nil
}

// PostFormDoc zapíše pokladničné tlačivo do pokladne ako účtovný doklad
// (PPD/VPD s číslom z radu pokladne, DPH pri platiteľovi). Tlačivo dostane
// číslo dokladu a odkaz naň; ďalej sa opravuje iba v Pokladni.
func (a *App) PostFormDoc(ctx context.Context, companyID, id, registerID int64) (forms.Doc, error) {
	if err := a.Require(ctx, companyID, auth.PermCashWrite); err != nil {
		return forms.Doc{}, err
	}
	d, err := a.Store.GetFormDoc(ctx, companyID, id)
	if err != nil {
		return d, err
	}
	if d.CashID != 0 {
		return d, errFormPosted(d)
	}
	if d.Template {
		return d, vErr("template", "Vzor sa do pokladne nezapisuje — vytvorte z neho tlačivo")
	}
	f, err := a.formByKey(ctx, companyID, d.FormKey)
	if err != nil {
		return d, err
	}
	if f.Cash == "" {
		return d, vErr("formKey", f.Name+" nie je pokladničný doklad")
	}
	c, err := forms.Compute(f, d.Values, d.Items)
	if err != nil {
		return d, err
	}
	doc := cash.Document{Kind: cash.Kind(f.Cash), Amount: c.Total, RegisterID: registerID}
	for _, fl := range f.Fields {
		s := strings.TrimSpace(d.Values[fl.Key])
		switch fl.Role {
		case "date":
			doc.Date, _ = invoice.ParseDate(s)
		case "partner":
			doc.Partner = strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", ", ")), " ")
		case "purpose":
			doc.Purpose = s
		case "note":
			doc.Note = s
		}
	}
	if c.Rate != nil && *c.Rate > 0 {
		doc.Tax = &cash.Tax{Rows: []cash.VATRow{{Rate: *c.Rate, Gross: c.Total}}}
	}
	if doc.Amount.Sign() <= 0 {
		return d, vErr("amount", "Zadajte sumu")
	}
	saved, err := a.CreateCash(ctx, companyID, doc)
	if err != nil {
		return d, err
	}
	d.CashID, d.CashNumber = saved.ID, saved.Number
	for _, fl := range f.Fields {
		if fl.Role == "number" {
			d.Values[fl.Key] = saved.Number
		}
	}
	d.Title = defaultTitle(f, d.Values)
	d.Updated = a.Today()
	if err := a.Store.SaveFormDoc(ctx, companyID, &d); err != nil {
		return d, err
	}
	a.record(ctx, companyID, audit.CatCash, "update", "form_doc", d.ID, fmt.Sprintf("Tlačivo %s zapísané do pokladne ako %s (%s €)", f.Name, saved.Number, saved.Amount.Display()), nil, d)
	return d, nil
}
