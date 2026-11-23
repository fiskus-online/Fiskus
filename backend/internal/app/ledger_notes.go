package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/ledger"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/stock"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// Poznámky k účtovnej závierke malej účtovnej jednotky (Úč POD 3-01,
// príloha č. 1 k opatreniu MF/23378/2014-74, čl. I – VII). Väčšinu textu
// píše účtovná jednotka; Fiškus doplní, čo vie: údaje o firme, zásady
// oceňovania podľa nastavení programu a prehľad položiek súvahy a výkazu
// ziskov a strát s porovnaním s minulým rokom. Prázdna časť sa do
// poznámok neuvádza (§ 4 ods. 3 opatrenia).

// NotesInput je text poznámok, ktorý zadáva účtovná jednotka.
type NotesInput struct {
	Activity        string       `json:"activity"`        // čl. I bod 1: opis činnosti
	PrevApproved    invoice.Date `json:"prevApproved"`    // čl. I bod 2: schválenie závierky za minulé obdobie
	Employees       string       `json:"employees"`       // čl. I bod 5: priemerný prepočítaný počet zamestnancov
	Group           string       `json:"group"`           // čl. I bod 4: skupina a konsolidácia
	Bodies          string       `json:"bodies"`          // čl. II: pôžičky, záruky a plnenia členom orgánov
	NotGoingConcern bool         `json:"notGoingConcern"` // čl. III bod 1: nepokračuje nepretržite v činnosti
	Policies        string       `json:"policies"`        // čl. III bod 2 a 4: zásady a metódy (prázdne = návrh Fiškusu)
	Errors          string       `json:"errors"`          // čl. III bod 5: opravy chýb minulých období
	Liabilities5y   money.Dec    `json:"liabilities5y"`   // čl. IV bod 3 a): záväzky so splatnosťou nad päť rokov
	Secured         string       `json:"secured"`         // čl. IV bod 3 b): zabezpečené záväzky
	Unusual         string       `json:"unusual"`         // čl. IV bod 6: výnosy a náklady mimoriadnej veľkosti
	Contingent      string       `json:"contingent"`      // čl. V bod 1: podmienený majetok a záväzky
	OffBalance      string       `json:"offBalance"`      // čl. V bod 2 a 3: iné finančné povinnosti, podsúvaha
	Events          string       `json:"events"`          // čl. VI: udalosti po dni závierky
	Other           string       `json:"other"`           // čl. VII a ďalšie informácie
}

// NotesRow je riadok prehľadu súvahy alebo výkazu v poznámkach.
type NotesRow struct {
	Statement string    `json:"statement"` // aktíva | pasíva | výkaz
	Code      string    `json:"code"`
	Label     string    `json:"label"`
	Current   money.Dec `json:"current"`
	Previous  money.Dec `json:"previous"`
}

// LedgerNotes sú poznámky roka: zadaný text, návrh zásad a prehľad čísel.
type LedgerNotes struct {
	Year    int          `json:"year"`
	Form    ledger.Form  `json:"form"`
	Company NotesCompany `json:"company"`
	Input   NotesInput   `json:"input"`
	// DefaultPolicies je návrh textu zásad oceňovania podľa nastavení
	// programu (použije sa, kým Policies je prázdne).
	DefaultPolicies string     `json:"defaultPolicies"`
	Rows            []NotesRow `json:"rows"`
	PreviousEmpty   bool       `json:"previousEmpty"`
	// Basis je právny základ zostavenia (čl. I bod 3).
	Basis string `json:"basis"`
}

// NotesCompany sú údaje o účtovnej jednotke v hlavičke a čl. I.
type NotesCompany struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	ICO     string `json:"ico"`
	DIC     string `json:"dic"`
}

func notesKey(companyID int64, year int) string {
	return fmt.Sprintf("ledger.notes.%d.%d", companyID, year)
}

// prehľad položiek v poznámkach: hlavné riadky vzoru Úč POD
var notesRows = map[string][]string{
	"aktíva": {"02", "03", "11", "21", "33", "34", "41", "53", "66", "71", "74", "01"},
	"pasíva": {"80", "81", "97", "100", "101", "102", "118", "121", "122", "136", "139", "140", "141", "79"},
	"výkaz":  {"01", "02", "10", "27", "29", "45", "55", "56", "57", "61"},
}

func (a *App) loadNotes(ctx context.Context, companyID int64, year int) (NotesInput, error) {
	var in NotesInput
	raw, err := a.Store.GetSetting(ctx, notesKey(companyID, year))
	if errors.Is(err, store.ErrNotFound) || (err == nil && raw == "") {
		return in, nil
	}
	if err != nil {
		return in, err
	}
	return in, json.Unmarshal([]byte(raw), &in)
}

// LedgerNotesFor zostaví poznámky roka.
func (a *App) LedgerNotesFor(ctx context.Context, companyID int64, year int) (LedgerNotes, error) {
	out := LedgerNotes{Year: year, Rows: []NotesRow{}}
	st, err := a.LedgerStatementsFor(ctx, companyID, year)
	if err != nil {
		return out, err
	}
	out.Form, out.PreviousEmpty = st.Form, st.PreviousEmpty
	if out.Input, err = a.loadNotes(ctx, companyID, year); err != nil {
		return out, err
	}
	c, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return out, err
	}
	addr := strings.TrimSpace(strings.Join(nonEmpty(c.Street, strings.TrimSpace(c.Zip+" "+c.City)), ", "))
	out.Company = NotesCompany{Name: c.Name, Address: addr, ICO: c.ICO, DIC: c.DIC}
	out.Basis = fmt.Sprintf("Účtovná závierka je riadna, zostavená za obdobie od 1. 1. %d do 31. 12. %d podľa § 17 ods. 6 zákona č. 431/2002 Z. z. o účtovníctve.", year, year)
	if out.DefaultPolicies, err = a.defaultPolicies(ctx, companyID); err != nil {
		return out, err
	}
	if st.Form == ledger.FormSmall {
		for _, part := range []struct {
			name string
			rows []ledger.StatementRow
		}{{"aktíva", st.Assets}, {"pasíva", st.Liabilities}, {"výkaz", st.Income}} {
			by := map[string]ledger.StatementRow{}
			for _, r := range part.rows {
				by[r.Code] = r
			}
			for _, code := range notesRows[part.name] {
				r := by[code]
				out.Rows = append(out.Rows, NotesRow{Statement: part.name, Code: code, Label: r.Label, Current: r.Netto, Previous: r.Previous})
			}
		}
	}
	return out, nil
}

// defaultPolicies navrhne text zásad oceňovania (čl. III bod 2 a 4) podľa
// toho, ako Fiškus účtuje.
func (a *App) defaultPolicies(ctx context.Context, companyID int64) (string, error) {
	lines := []string{
		"Dlhodobý nehmotný a hmotný majetok sa oceňuje obstarávacou cenou, majetok vytvorený vlastnou činnosťou vlastnými nákladmi. Účtovné odpisy sa určujú podľa odpisového plánu rovnomerne počas predpokladanej doby používania majetku, od mesiaca nasledujúceho po zaradení do používania; pri majetku bez určenej doby účtovného odpisovania sa účtovné odpisy rovnajú daňovým.",
	}
	cfg, err := a.loadStock(ctx, companyID)
	if err != nil {
		return "", err
	}
	if len(cfg.Items) > 0 {
		method := "váženým aritmetickým priemerom"
		if cfg.Method == stock.FIFO {
			method = "metódou FIFO (prvá cena na ocenenie prírastku sa použije ako prvá na ocenenie úbytku)"
		}
		lines = append(lines, "Zásoby nakupované sa oceňujú obstarávacou cenou vrátane vedľajších obstarávacích nákladov; úbytok zásob sa oceňuje "+method+". Zásoby sa účtujú spôsobom A.")
	}
	lines = append(lines,
		"Pohľadávky a záväzky sa oceňujú menovitou hodnotou. Opravné položky k majetku sa tvoria, ak je ocenenie v účtovníctve vyššie ako jeho skutočná hodnota.",
		"Majetok a záväzky v cudzej mene sa prepočítavajú na eurá referenčným výmenným kurzom určeným a vyhláseným Európskou centrálnou bankou ku dňu uskutočnenia účtovného prípadu a ku dňu, ku ktorému sa zostavuje účtovná závierka; kurzové rozdiely sa účtujú do nákladov alebo výnosov.",
	)
	return strings.Join(lines, "\n\n"), nil
}

// SaveLedgerNotes uloží text poznámok roka. Poznámky sa dopĺňajú aj po
// uzavretí roka (účtovná závierka sa zostavuje po ňom), preto ich zámok
// roka neblokuje.
func (a *App) SaveLedgerNotes(ctx context.Context, companyID int64, year int, in NotesInput) (LedgerNotes, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return LedgerNotes{}, err
	}
	if year < 2001 || year > 2100 {
		return LedgerNotes{}, mfaFieldErr("year", "Neplatný rok")
	}
	if in.Liabilities5y.Sign() < 0 {
		return LedgerNotes{}, mfaFieldErr("liabilities5y", "Suma nesmie byť záporná")
	}
	if n := strings.TrimSpace(in.Employees); n != "" {
		if _, err := money.Parse(strings.ReplaceAll(n, ",", ".")); err != nil {
			return LedgerNotes{}, mfaFieldErr("employees", "Počet zamestnancov je číslo (napr. 2,5)")
		}
	}
	before, err := a.loadNotes(ctx, companyID, year)
	if err != nil {
		return LedgerNotes{}, err
	}
	b, err := json.Marshal(in)
	if err != nil {
		return LedgerNotes{}, err
	}
	if err := a.Store.SetSetting(ctx, notesKey(companyID, year), string(b)); err != nil {
		return LedgerNotes{}, err
	}
	a.record(ctx, companyID, audit.CatLedger, "update", "ledger", int64(year), fmt.Sprintf("Poznámky k účtovnej závierke %d", year), before, in)
	return a.LedgerNotesFor(ctx, companyID, year)
}
