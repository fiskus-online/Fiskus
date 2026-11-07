package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/dpfo"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Daňové priznanie k dani z príjmov FO typ B (živnostník v jednoduchom
// účtovníctve). Sumy z účtovníctva berie z výkazov roka, ostatné údaje
// (osobné údaje, preddavky, zamestnanie, DDS, podiel dane) zadá daňovník
// a uložia sa pre firmu a rok.

// TaxReturn je priznanie na obrazovku: zadané údaje, údaje z účtovníctva a výpočet.
type TaxReturn struct {
	Year         int                  `json:"year"`
	Years        []int                `json:"years"` // roky so zákonnými hodnotami
	Input        dpfo.Input           `json:"input"`
	Rows         map[string]money.Dec `json:"rows"`
	Notes        []string             `json:"notes"`
	Form         string               `json:"form"`
	Unclassified int                  `json:"unclassified"` // nerozpísané pohyby v denníku — sumy nie sú úplné
}

func taxReturnKey(companyID int64, year int) string {
	return fmt.Sprintf("dpfo.%d.%d", companyID, year)
}

var dateRe = regexp.MustCompile(`^\d{2}\.\d{2}\.\d{4}$`)

// fillFromBooks doplní sumy z účtovníctva a predvolené údaje z firmy.
func (a *App) fillFromBooks(ctx context.Context, companyID int64, in *dpfo.Input) (int, error) {
	st, err := a.StatementsFor(ctx, companyID, in.Year)
	if err != nil {
		return 0, err
	}
	in.Income, in.Expenses, in.Insurance = st.TaxB.Income, st.TaxB.Expenses, st.TaxB.Insur
	for i, r := range st.TaxB.Table1a {
		if i < len(in.Table1a) {
			in.Table1a[i] = [2]money.Dec{r.Previous, r.Current}
		}
	}
	co, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return 0, err
	}
	tp := &in.Taxpayer
	if tp.DIC == "" {
		tp.DIC = co.DIC
	}
	if tp.Surname == "" && tp.Name == "" {
		tp.Name, tp.Surname = splitPersonName(co.Name)
	}
	if tp.Street == "" && tp.City == "" {
		tp.Street, tp.Number = splitStreet(co.Street)
		tp.Zip, tp.City = co.Zip, co.City
	}
	if tp.Country == "" {
		tp.Country = "Slovenská republika"
	}
	return st.Unclassified, nil
}

// splitPersonName rozdelí obchodné meno živnostníka „Ján Novák – elektroservis“
// na meno a priezvisko (dodatok za pomlčkou sa vynechá).
func splitPersonName(s string) (name, surname string) {
	for _, sep := range []string{" – ", " - ", ",", " — "} {
		if i := strings.Index(s, sep); i > 0 {
			s = s[:i]
		}
	}
	f := strings.Fields(s)
	if len(f) < 2 {
		return "", strings.TrimSpace(s)
	}
	return strings.Join(f[:len(f)-1], " "), f[len(f)-1]
}

// splitStreet oddelí číslo domu od ulice („Námestie SNP 1“ → „Námestie SNP“, „1“).
func splitStreet(s string) (street, number string) {
	s = strings.TrimSpace(s)
	i := strings.LastIndexByte(s, ' ')
	if i > 0 && strings.ContainsAny(s[i+1:i+2], "0123456789") {
		return s[:i], s[i+1:]
	}
	return s, ""
}

// TaxReturnFor vráti priznanie za rok (uložené údaje + účtovníctvo + výpočet).
func (a *App) TaxReturnFor(ctx context.Context, companyID int64, year int) (TaxReturn, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerRead); err != nil {
		return TaxReturn{}, err
	}
	out := TaxReturn{Year: year, Years: dpfo.Years()}
	in := dpfo.Input{Year: year}
	if v, err := a.Store.GetSetting(ctx, taxReturnKey(companyID, year)); err == nil {
		if err := json.Unmarshal([]byte(v), &in); err != nil {
			return out, err
		}
		in.Year = year
	} else if !errors.Is(err, store.ErrNotFound) {
		return out, err
	}
	un, err := a.fillFromBooks(ctx, companyID, &in)
	if err != nil {
		return out, err
	}
	if in.Date == "" {
		in.Date = a.now().Format("02.01.2006")
	}
	out.Input, out.Unclassified = in, un
	res, err := dpfo.Compute(in)
	if err != nil {
		if errors.Is(err, dpfo.ErrYear) {
			return out, mfaFieldErr("year", err.Error())
		}
		return out, err
	}
	out.Rows, out.Notes, out.Form = res.Rows, res.Notes, res.Params.Form
	if un > 0 {
		out.Notes = append(out.Notes, fmt.Sprintf("V peňažnom denníku je %d nerozpísaných pohybov — príjmy a výdavky nie sú úplné", un))
	}
	return out, nil
}

// SaveTaxReturn uloží údaje, ktoré zadáva daňovník (sumy z účtovníctva nie).
func (a *App) SaveTaxReturn(ctx context.Context, companyID int64, year int, in dpfo.Input) (TaxReturn, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return TaxReturn{}, err
	}
	if _, err := dpfo.ParamsFor(year); err != nil {
		return TaxReturn{}, mfaFieldErr("year", err.Error())
	}
	var fe []invoice.FieldError
	bad := func(f, msg string) { fe = append(fe, invoice.FieldError{Field: f, Message: msg}) }
	tp := &in.Taxpayer
	for _, s := range []*string{&tp.DIC, &tp.BirthDate, &tp.Surname, &tp.Name, &tp.Title, &tp.TitleAft, &tp.Street, &tp.Number, &tp.Zip, &tp.City, &tp.Country, &tp.NACE, &tp.Activity,
		&in.NGOICO, &in.NGOName, &in.RefundIBAN, &in.Date} {
		*s = strings.TrimSpace(*s)
		if len([]rune(*s)) > 200 {
			*s = string([]rune(*s)[:200])
		}
	}
	if d := digitsOnly(tp.DIC); tp.DIC != "" && len(d) != 10 {
		bad("taxpayer.dic", "DIČ má 10 číslic")
	}
	if tp.BirthDate != "" && !dateRe.MatchString(tp.BirthDate) {
		bad("taxpayer.birthDate", "Dátum v tvare DD.MM.RRRR")
	}
	if in.Date != "" && !dateRe.MatchString(in.Date) {
		bad("date", "Dátum v tvare DD.MM.RRRR")
	} else if in.Date != "" {
		if _, err := time.Parse("02.01.2006", in.Date); err != nil {
			bad("date", "Neplatný dátum")
		}
	}
	if n := digitsOnly(tp.NACE); tp.NACE != "" && len(n) != 5 {
		bad("taxpayer.nace", "Kód SK NACE má 5 číslic (napr. 43210)")
	}
	if i := digitsOnly(in.NGOICO); in.NGOICO != "" && (len(i) < 6 || len(i) > 8) {
		bad("ngoIco", "IČO prijímateľa má 6 až 8 číslic")
	}
	for _, x := range []struct {
		f string
		v money.Dec
	}{{"empIncome", in.EmpIncome}, {"empInsurance", in.EmpInsurance}, {"empPrepaid", in.EmpPrepaid}, {"dds", in.DDS}, {"prepaid", in.Prepaid}} {
		if x.v.Sign() < 0 {
			bad(x.f, "Suma nemôže byť záporná")
		}
	}
	// deti, manžel/ka, straty
	if len(in.Children) > 10 {
		bad("children", "Najviac 10 detí")
	}
	kids := in.Children[:0]
	for i, c := range in.Children {
		c.Name, c.RC = strings.TrimSpace(c.Name), strings.TrimSpace(c.RC)
		if c.Name == "" && c.RC == "" {
			continue
		}
		if c.Name == "" {
			bad(fmt.Sprintf("children.%d.name", i), "Zadajte meno a priezvisko dieťaťa")
		}
		if _, err := dpfo.BirthDate(c.RC); err != nil {
			bad(fmt.Sprintf("children.%d.rc", i), "Rodné číslo: "+err.Error())
		}
		kids = append(kids, c)
	}
	in.Children = kids
	if sp := in.Spouse; sp != nil {
		sp.Name, sp.RC = strings.TrimSpace(sp.Name), strings.TrimSpace(sp.RC)
		switch {
		case sp.Name == "" && sp.Months == 0:
			in.Spouse = nil
		case sp.Name == "":
			bad("spouse.name", "Zadajte meno a priezvisko manžela/ky")
		}
		if in.Spouse != nil {
			if sp.Months < 0 || sp.Months > 12 {
				bad("spouse.months", "Počet mesiacov je 0 až 12")
			}
			if sp.OwnIncome.Sign() < 0 {
				bad("spouse.ownIncome", "Suma nemôže byť záporná")
			}
			if sp.RC != "" {
				if _, err := dpfo.BirthDate(sp.RC); err != nil {
					bad("spouse.rc", "Rodné číslo: "+err.Error())
				}
			}
		}
	}
	losses := in.Losses[:0]
	for i, l := range in.Losses {
		if l.Amount.IsZero() && l.Used.IsZero() {
			continue
		}
		if l.Year < year-5 || l.Year >= year {
			bad(fmt.Sprintf("losses.%d.year", i), fmt.Sprintf("Strata sa odpočítava iba za roky %d až %d", year-5, year-1))
		}
		if l.Amount.Sign() < 0 || l.Used.Sign() < 0 || l.Used.Cmp(l.Amount) > 0 {
			bad(fmt.Sprintf("losses.%d.used", i), "Odpočítané skôr nemôže byť viac ako strata")
		}
		losses = append(losses, l)
	}
	in.Losses = losses
	if in.EmpBonus.Sign() < 0 {
		bad("empBonus", "Suma nemôže byť záporná")
	}
	if in.Attachments < 0 || in.Attachments > 99 {
		bad("attachments", "Počet príloh je 0 až 99")
	}
	if len(fe) > 0 {
		return TaxReturn{}, &invoice.ValidationError{Fields: fe}
	}
	in.Year = year
	// sumy z účtovníctva sa neukladajú — vždy sa berú z kníh
	in.Income, in.Expenses, in.Insurance = money.Zero, money.Zero, money.Zero
	in.Table1a = [6][2]money.Dec{}
	b, _ := json.Marshal(in)
	if err := a.Store.SetSetting(ctx, taxReturnKey(companyID, year), string(b)); err != nil {
		return TaxReturn{}, err
	}
	a.record(ctx, companyID, audit.CatLedger, "update", "tax_return", int64(year), fmt.Sprintf("Daňové priznanie typ B za %d — údaje daňovníka", year), nil, nil)
	return a.TaxReturnFor(ctx, companyID, year)
}

// TaxReturnXML vytvorí XML na načítanie do formulára DPFOB na portáli.
func (a *App) TaxReturnXML(ctx context.Context, companyID int64, year int) ([]byte, string, error) {
	tr, err := a.TaxReturnFor(ctx, companyID, year)
	if err != nil {
		return nil, "", err
	}
	res, err := dpfo.Compute(tr.Input)
	if err != nil {
		return nil, "", err
	}
	data, err := dpfo.XML(tr.Input, res)
	if err != nil {
		return nil, "", err
	}
	return data, fmt.Sprintf("DPFOB-%d.xml", year), nil
}

func digitsOnly(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}
