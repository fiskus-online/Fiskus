package app

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/calendar"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/payroll"
)

// Mesačný výkaz poistného a príspevkov pre Sociálnu poisťovňu (MVPP), XML
// podľa schémy http://socpoist.sk/xsd/mvpp2026 (obdobia od 01/2026).
// Výkaz sa skladá z uzavretého mesiaca: vymeriavacie základy a poistné po
// fondoch sú hotové z výpočtu miezd, sumárny blok je ich súčet.

// MVPPNamespace je menný priestor schémy MVPP v2026.
const MVPPNamespace = "http://socpoist.sk/xsd/mvpp2026"

// MVPPError: výkaz sa nedá zostaviť (chýbajúce údaje).
type MVPPError struct{ Problems []string }

func (e *MVPPError) Error() string { return strings.Join(e.Problems, " ") }

// fondy v poradí výkazu → fond výpočtu miezd
var mvppFunds = []struct{ code, fund string }{
	{"np", payroll.FundSickness}, {"sp", payroll.FundOldAge}, {"ip", payroll.FundDisability},
	{"pvn", payroll.FundUnemployment}, {"up", payroll.FundAccident}, {"gp", payroll.FundGuarantee},
	{"rfs", payroll.FundReserve},
}

type mvppDoc struct {
	XMLName    xml.Name `xml:"mvpp"`
	Xmlns      string   `xml:"xmlns,attr"`
	TypDoc     string   `xml:"typDoc"`
	Obdobie    string   `xml:"obdobie"`
	DenVyplaty string   `xml:"denVyplaty"`
	TypVykazu  string   `xml:"typVykazu"`
	Zamestnav  struct {
		Identifikacia struct {
			VS            string `xml:"variabilnySymbol"`
			Identifikator struct {
				ICO string `xml:"ico,omitempty"`
				DIC string `xml:"dic,omitempty"`
			} `xml:"identifikator"`
			Nazov string `xml:"nazov"`
			Tel   string `xml:"tel,omitempty"`
			Mail  string `xml:"mail,omitempty"`
		} `xml:"identifikacia"`
		Ucet *struct {
			IBAN string `xml:"iban"`
		} `xml:"ucet,omitempty"`
	} `xml:"zamestnavatel"`
	Poistne struct {
		Funds []mvppFund `xml:",any"`
		PFP   *struct {
			Zamtel string `xml:"pfpZamtel"`
		} `xml:"pfp,omitempty"`
		Spolu string `xml:"spoluPoistne"`
	} `xml:"poistne"`
	FOsoba struct {
		Priezvisko string `xml:"priezvisko"`
		Meno       string `xml:"meno"`
		Tel        string `xml:"tel"`
		Mail       string `xml:"mail"`
	} `xml:"fOsoba"`
	Vystavenie struct {
		Zostavil string `xml:"zostavil"`
		Datum    string `xml:"datum,omitempty"`
	} `xml:"vystavenie"`
	Priloha struct {
		Zamestnanci []mvppEmployee `xml:"poistneZamestnancov>poistneZamestnanca"`
	} `xml:"priloha"`
}

// mvppFund je súhrn fondu (np, sp …): poistné zamestnávateľa a zamestnanca.
type mvppFund struct {
	XMLName xml.Name
	Items   []mvppValue
}

type mvppValue struct {
	XMLName xml.Name
	Value   string `xml:",chardata"`
}

func (f mvppFund) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name = f.XMLName
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	for _, v := range f.Items {
		if err := e.EncodeElement(v.Value, xml.StartElement{Name: v.XMLName}); err != nil {
			return err
		}
	}
	return e.EncodeToken(start.End())
}

// mvppEmployee je záznam prílohy; všetky údaje sú atribúty.
type mvppEmployee struct {
	Attrs []xml.Attr
}

func (m mvppEmployee) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Attr = m.Attrs
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	return e.EncodeToken(start.End())
}

func amt(d money.Dec) string { return d.RoundCents().String() }

// skDate je dátum v tvare DD.MM.RRRR.
func skDate(d invoice.Date) string {
	return fmt.Sprintf("%02d.%02d.%04d", d.Day(), d.Month(), d.Year())
}

// mvppType je typ zamestnanca podľa schémy (ZEC, ZECD1 …).
func mvppType(e payroll.Employee, vynimka bool) (string, error) {
	if e.Contract == payroll.TPP {
		return "ZEC", nil
	}
	code := map[payroll.Contract]string{payroll.DoVP: "ZECD1", payroll.DoPC: "ZECD2", payroll.DoBPS: "ZECD3"}[e.Contract]
	regular := e.Regular
	switch {
	case vynimka && e.Contract == payroll.DoBPS && regular:
		return "ZECD3V", nil
	case vynimka && e.Contract == payroll.DoBPS:
		return "ZECD3NV", nil
	case vynimka && regular:
		return code + "V", nil
	case vynimka:
		return "", fmt.Errorf("%s: odvodová výnimka pri dohode s nepravidelným príjmom — výkaz ju nepozná", e.Name())
	case regular:
		return code, nil
	}
	return code + "N", nil
}

// mvppDays je počet kalendárnych dní poistenia v mesiaci (od 2026 vrátane PN a iných sociálnych udalostí).
func mvppDays(e payroll.Employee, year, month int) int {
	first := invoice.NewDate(year, time.Month(month), 1)
	last := invoice.NewDate(year, time.Month(month)+1, 0)
	from, to := first, last
	if e.Start != "" {
		if d, err := invoice.ParseDate(e.Start); err == nil && d.After(from) {
			from = d
		}
	}
	if e.End != "" {
		if d, err := invoice.ParseDate(e.End); err == nil && d.Before(to) {
			to = d
		}
	}
	if to.Before(from) {
		return 0
	}
	return from.DaysUntil(to) + 1
}

// mvppHours: hodiny, za ktoré patrí vymeriavací základ (odpracované aj
// náhrady). Hodinová mzda = hodiny z miezd; mesačná = pracovné dni
// pracovného vzťahu v mesiaci × týždenný čas / 5. Inak kód 999.
func mvppHours(e payroll.Employee, in payroll.Input, year, month int, days *calendar.Days) string {
	if e.Hourly && in.Hours.Sign() > 0 {
		return amt(in.Hours)
	}
	if e.Hourly || e.Hours.Sign() <= 0 {
		return "999"
	}
	first := invoice.NewDate(year, time.Month(month), 1)
	n := 0
	for d := first; d.Month() == month; d = d.AddDays(1) {
		if days.Workday(d) && e.ActiveIn(fmt.Sprintf("%04d-%02d", year, month)) && withinEmployment(e, d) {
			n++
		}
	}
	return amt(e.Hours.MulRatio(money.FromInt(int64(n)), money.FromInt(5)))
}

func withinEmployment(e payroll.Employee, d invoice.Date) bool {
	s := d.String()
	return (e.Start == "" || e.Start <= s) && (e.End == "" || e.End >= s)
}

// PayrollMVPP zostaví mesačný výkaz poistného za uzavretý mesiac.
func (a *App) PayrollMVPP(ctx context.Context, cid int64, year, month int) (string, []byte, error) {
	if err := a.requireBusiness(ctx, cid, auth.PermLedgerRead); err != nil {
		return "", nil, err
	}
	if year < 2026 {
		return "", nil, &MVPPError{[]string{"Výkaz vo formáte 2026 je pre obdobia od januára 2026."}}
	}
	co, err := a.Store.GetCompany(ctx, cid)
	if err != nil {
		return "", nil, err
	}
	v, err := a.Payroll(ctx, cid, year, month)
	if err != nil {
		return "", nil, err
	}
	s, err := a.payrollPaySettings(ctx, cid)
	if err != nil {
		return "", nil, err
	}
	var probs []string
	if !v.Closed {
		probs = append(probs, fmt.Sprintf("Mzdy za %d/%d najprv uzavrite.", month, year))
	}
	if len(s.SocialVS) != 10 {
		probs = append(probs, "Doplňte 10-miestny variabilný symbol zamestnávateľa v Platobných údajoch (okno Príkaz na úhradu).")
	}
	if s.PayDay <= 0 {
		probs = append(probs, "Doplňte deň výplaty v Platobných údajoch (okno Príkaz na úhradu).")
	}
	if s.ContactLast == "" || s.ContactFirst == "" || s.ContactPhone == "" || s.ContactEmail == "" {
		probs = append(probs, "Doplňte osobu, ktorá výkaz zostavila (meno, priezvisko, telefón, e-mail), v Platobných údajoch (okno Príkaz na úhradu).")
	}
	ico := onlyDigitsOf(co.ICO)
	if ico == "" && co.DIC == "" {
		probs = append(probs, "Firma nemá IČO ani DIČ.")
	}
	b, err := a.employeeBook(ctx, cid)
	if err != nil {
		return "", nil, err
	}
	rcNow := map[int64]string{} // rodné číslo doplnené po uzavretí mesiaca
	for _, e := range b.List {
		rcNow[e.ID] = e.RC
	}
	days := calendar.NewDays(a.legalParams().Holidays)
	doc := mvppDoc{Xmlns: MVPPNamespace, TypDoc: "MVP00001", Obdobie: fmt.Sprintf("%02d%04d", month, year), TypVykazu: "R"}
	pay := invoice.NewDate(year, time.Month(month)+1, 1)
	doc.DenVyplaty = skDate(payDate(pay.Year(), time.Month(pay.Month()), s.PayDay))
	id := &doc.Zamestnav.Identifikacia
	id.VS, id.Nazov = s.SocialVS, truncRunes(co.Name, 144)
	if ico != "" {
		id.Identifikator.ICO = ico
	} else {
		id.Identifikator.DIC = co.DIC
	}
	if iban := strings.ReplaceAll(co.IBAN, " ", ""); iban != "" {
		doc.Zamestnav.Ucet = &struct {
			IBAN string `xml:"iban"`
		}{iban}
	}
	doc.FOsoba.Priezvisko, doc.FOsoba.Meno = truncRunes(s.ContactLast, 36), truncRunes(s.ContactFirst, 24)
	doc.FOsoba.Tel, doc.FOsoba.Mail = truncRunes(s.ContactPhone, 50), truncRunes(s.ContactEmail, 255)
	doc.Vystavenie.Zostavil = truncRunes(s.ContactFirst+" "+s.ContactLast, 30)
	doc.Vystavenie.Datum = skDate(a.Today())

	er, ee := map[string]money.Dec{}, map[string]money.Dec{}
	var pfp, total money.Dec
	hasPFP := false
	for i, r := range v.Rows {
		e, res := r.Employee, r.Result
		rc := onlyDigitsOf(e.RC)
		if rc == "" {
			rc = onlyDigitsOf(rcNow[e.ID])
		}
		if len(rc) < 9 || len(rc) > 10 {
			probs = append(probs, e.Name()+": chýba rodné číslo.")
		}
		// odvodová výnimka (§ 227a): rovnaká podmienka ako vo výpočte miezd
		vynimka := e.OOP && (e.Contract == payroll.DoBPS || (e.Contract.Agreement() && e.Pension != payroll.PensionNone))
		typ, err := mvppType(e, vynimka)
		if err != nil {
			probs = append(probs, err.Error())
		}
		fund := map[string]payroll.Fund{}
		for _, f := range res.Social {
			fund[f.Fund] = f
		}
		at := func(name, val string) xml.Attr { return xml.Attr{Name: xml.Name{Local: name}, Value: val} }
		attrs := []xml.Attr{at("pc", fmt.Sprint(i+1)), at("rc", rc), at("pocDni", fmt.Sprint(mvppDays(e, year, month))),
			at("typZec", typ), at("vynimkaVZ", map[bool]string{true: "1", false: "0"}[vynimka])}
		for _, f := range mvppFunds {
			attrs = append(attrs, at("vz"+strings.ToUpper(f.code[:1])+f.code[1:], amt(fund[f.fund].Base)))
		}
		for _, f := range mvppFunds[:4] {
			attrs = append(attrs, at("vzZec"+strings.ToUpper(f.code[:1])+f.code[1:], amt(fund[f.fund].Base)))
		}
		for j, f := range mvppFunds {
			x := fund[f.fund]
			er[f.code], ee[f.code] = er[f.code].Add(x.Employer), ee[f.code].Add(x.Employee)
			total = total.Add(x.Employer).Add(x.Employee)
			attrs = append(attrs, at(f.code+"Zamtel", amt(x.Employer)))
			if j < 4 { // úrazové, garančné a RFS platí iba zamestnávateľ
				attrs = append(attrs, at(f.code+"Zamnec", amt(x.Employee)))
			}
		}
		if x, ok := fund[payroll.FundShortTime]; ok {
			hasPFP = true
			pfp = pfp.Add(x.Employer)
			total = total.Add(x.Employer)
			attrs = append(attrs, at("vzPfp", amt(x.Base)), at("pfpZamtel", amt(x.Employer)))
		}
		attrs = append(attrs, at("pocetHodin", mvppHours(e, r.Input, year, month, days)))
		doc.Priloha.Zamestnanci = append(doc.Priloha.Zamestnanci, mvppEmployee{Attrs: attrs})
	}
	if len(v.Rows) == 0 && v.Closed {
		probs = append(probs, "V mesiaci nie sú žiadni zamestnanci.")
	}
	if len(probs) > 0 {
		return "", nil, &MVPPError{probs}
	}
	for j, f := range mvppFunds {
		items := []mvppValue{{XMLName: xml.Name{Local: f.code + "Zamtel"}, Value: amt(er[f.code])}}
		if j < 4 {
			items = append(items, mvppValue{XMLName: xml.Name{Local: f.code + "Zamnec"}, Value: amt(ee[f.code])})
		}
		doc.Poistne.Funds = append(doc.Poistne.Funds, mvppFund{XMLName: xml.Name{Local: f.code}, Items: items})
	}
	if hasPFP {
		doc.Poistne.PFP = &struct {
			Zamtel string `xml:"pfpZamtel"`
		}{amt(pfp)}
	}
	doc.Poistne.Spolu = amt(total)
	out, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", nil, err
	}
	data := append([]byte(xml.Header), out...)
	a.record(ctx, cid, audit.CatPayroll, "export", "payroll", int64(year*100+month),
		fmt.Sprintf("Mesačný výkaz poistného %d/%d (%d zamestnancov, %s €)", month, year, len(v.Rows), total.Display()), nil, nil)
	return fmt.Sprintf("mvpp-%04d-%02d.xml", year, month), data, nil
}

func onlyDigitsOf(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func truncRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
