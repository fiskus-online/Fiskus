package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/chart"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/journal"
	"github.com/fiskus-online/fiskus/backend/internal/ledger"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/stock"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// Uzávierka roka v jednoduchom aj podvojnom účtovníctve: po kontrole sa rok uzamkne
// (pohyby peňazí, doklady a rozpis denníka s dátumom v roku sa už nedajú
// meniť), konečné stavy bankových účtov sa prenesú ako počiatočné stavy
// nasledujúceho roka a uloží sa súhrn výkazov. V podvojnom účtovníctve sa
// uzamknú aj zápisy hlavnej knihy roka a konečné stavy súvahových účtov sa
// prenesú ako počiatočné stavy ďalšieho roka (výsledok hospodárenia na 431). Rok sa dá znova otvoriť
// s uvedením dôvodu (zapíše sa do auditu).

// YearClose je uzávierka roka.
type YearClose struct {
	Year     int       `json:"year"`
	Closed   bool      `json:"closed"`
	ClosedAt time.Time `json:"closedAt"`
	ClosedBy string    `json:"closedBy"`
	Income   money.Dec `json:"income"`   // príjmy celkom (Úč FO 1-01 r. 4); podvojné: výnosy
	Expenses money.Dec `json:"expenses"` // výdavky celkom (r. 11); podvojné: náklady
	Result   money.Dec `json:"result"`   // rozdiel príjmov a výdavkov (r. 12); podvojné: výsledok hospodárenia
	Double   bool      `json:"double"`   // podvojné účtovníctvo
	// Opening: počiatočné stavy ďalšieho roka boli prenesené z uzávierky
	// (false = rok už mal zadané vlastné počiatočné stavy, ostali).
	Opening bool                 `json:"opening"`
	Bank    map[string]money.Dec `json:"bank"` // konečné stavy účtov (IBAN) → počiatočné stavy ďalšieho roka
	Cash    map[string]money.Dec `json:"cash"` // konečné stavy pokladníc (prenášajú sa samy z dokladov)
	History []YearCloseEvent     `json:"history"`
}

// YearCloseEvent je uzavretie alebo otvorenie roka.
type YearCloseEvent struct {
	At     time.Time `json:"at"`
	By     string    `json:"by"`
	Action string    `json:"action"` // close | reopen
	Reason string    `json:"reason,omitempty"`
}

// YearCheck je kontrola pred uzávierkou.
type YearCheck struct {
	ID       string `json:"id"`
	OK       bool   `json:"ok"`
	Blocking bool   `json:"blocking"` // bez splnenia sa rok nedá uzavrieť
	Text     string `json:"text"`
	Link     string `json:"link,omitempty"`
}

// YearCloseInfo je stav uzávierky roka s kontrolami.
type YearCloseInfo struct {
	Year     int         `json:"year"`
	Close    *YearClose  `json:"close"` // nil = rok nebol nikdy uzavretý
	Checks   []YearCheck `json:"checks"`
	CanClose bool        `json:"canClose"`
	Years    []int       `json:"years"` // uzavreté roky
}

func yearCloseKey(companyID int64, year int) string {
	return fmt.Sprintf("yearclose.%d.%d", companyID, year)
}

// ErrYearClosed — zmena v uzavretom roku.
func errYearClosed(year int) error {
	return &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "date",
		Message: fmt.Sprintf("Rok %d je uzavretý — zmenu urobíte až po jeho otvorení vo Výkazoch (Uzávierka roka)", year)}}}
}

func (a *App) yearClose(ctx context.Context, companyID int64, year int) (*YearClose, error) {
	v, err := a.Store.GetSetting(ctx, yearCloseKey(companyID, year))
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c YearClose
	if err := json.Unmarshal([]byte(v), &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// YearClosed: je rok firmy uzavretý?
func (a *App) YearClosed(ctx context.Context, companyID int64, year int) bool {
	c, err := a.yearClose(ctx, companyID, year)
	return err == nil && c != nil && c.Closed
}

// checkOpen vráti chybu, ak dátum patrí do uzavretého roka.
func (a *App) checkOpen(ctx context.Context, companyID int64, d invoice.Date) error {
	if d.IsZero() {
		return nil
	}
	if a.YearClosed(ctx, companyID, d.Year()) {
		return errYearClosed(d.Year())
	}
	return nil
}

// checkOpenYear vráti chybu, ak je rok uzavretý.
func (a *App) checkOpenYear(ctx context.Context, companyID int64, year int) error {
	if a.YearClosed(ctx, companyID, year) {
		return errYearClosed(year)
	}
	return nil
}

func (a *App) actorName(ctx context.Context) string {
	act := auth.ActorFrom(ctx)
	if act.Name != "" {
		return act.Name
	}
	return "Fiškus"
}

// closedYears vráti uzavreté roky firmy za posledných 15 rokov (zostupne).
func (a *App) closedYears(ctx context.Context, companyID int64) ([]int, error) {
	out := []int{}
	for y := a.Today().Year(); y > a.Today().Year()-15; y-- {
		c, err := a.yearClose(ctx, companyID, y)
		if err != nil {
			return nil, err
		}
		if c != nil && c.Closed {
			out = append(out, y)
		}
	}
	return out, nil
}

// YearCloseFor vráti stav uzávierky roka a kontroly pred ňou.
func (a *App) YearCloseFor(ctx context.Context, companyID int64, year int) (YearCloseInfo, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerRead); err != nil {
		return YearCloseInfo{}, err
	}
	co, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return YearCloseInfo{}, err
	}
	out := YearCloseInfo{Year: year, Checks: []YearCheck{}}
	if out.Close, err = a.yearClose(ctx, companyID, year); err != nil {
		return out, err
	}
	if out.Years, err = a.closedYears(ctx, companyID); err != nil {
		return out, err
	}
	add := func(c YearCheck) { out.Checks = append(out.Checks, c) }
	if co.Bookkeeping == store.BookkeepingHousehold {
		add(YearCheck{ID: "bookkeeping", Blocking: true, Text: "Uzávierka roka je pre jednoduché a podvojné účtovníctvo"})
		return out, nil
	}
	today := a.Today()
	add(YearCheck{ID: "ended", OK: today.Year() > year, Blocking: true,
		Text: fmt.Sprintf("Rok %d skončil (uzavrieť sa dá od 1. 1. %d)", year, year+1)})
	if co.Bookkeeping == store.BookkeepingDouble {
		if err := a.doubleChecks(ctx, companyID, year, add); err != nil {
			return out, err
		}
		return finishChecks(out), nil
	}
	st, err := a.StatementsFor(ctx, companyID, year)
	if err != nil {
		return out, err
	}
	add(YearCheck{ID: "journal", OK: st.Unclassified == 0, Blocking: true, Link: "/dennik",
		Text: map[bool]string{true: "Všetky pohyby v peňažnom denníku sú rozpísané",
			false: fmt.Sprintf("V peňažnom denníku je %d nerozpísaných pohybov", st.Unclassified)}[st.Unclassified == 0]})
	_, saved, err := a.statementInputs(ctx, companyID, year)
	if err != nil {
		return out, err
	}
	add(YearCheck{ID: "inputs", OK: saved, Link: "/vykazy-ju",
		Text: map[bool]string{true: "Zásoby, ceniny a ďalšie zadané riadky výkazu sú uložené",
			false: "Riadky výkazu o majetku, ktoré zadávate (zásoby po inventúre, ceniny, rezervy), ešte nie sú uložené"}[saved]})
	prevOK := true
	if prev, err := a.Journal(ctx, companyID, year-1); err == nil && len(prev.Entries) > 0 {
		prevOK = a.YearClosed(ctx, companyID, year-1)
	}
	add(YearCheck{ID: "previous", OK: prevOK,
		Text: map[bool]string{true: "Predchádzajúci rok je uzavretý alebo bez pohybov",
			false: fmt.Sprintf("Rok %d ešte nie je uzavretý — odporúčame uzavierať po poradí", year-1)}[prevOK]})
	book, err := a.Journal(ctx, companyID, year)
	if err != nil {
		return out, err
	}
	negative := []string{}
	for _, b := range book.Balances {
		if b.Means == journal.Cash && b.Closing.Sign() < 0 {
			negative = append(negative, b.Account)
		}
	}
	add(YearCheck{ID: "cash", OK: len(negative) == 0, Link: "/pokladna",
		Text: map[bool]string{true: "Pokladnice nemajú záporný zostatok",
			false: "Záporný zostatok pokladnice: " + strings.Join(negative, ", ") + " — chýba príjmový doklad"}[len(negative) == 0]})
	return finishChecks(out), nil
}

func finishChecks(out YearCloseInfo) YearCloseInfo {
	out.CanClose = true
	for _, c := range out.Checks {
		if c.Blocking && !c.OK {
			out.CanClose = false
		}
	}
	if out.Close != nil && out.Close.Closed {
		out.CanClose = false
	}
	return out
}

// doubleChecks sú kontroly pred uzávierkou v podvojnom účtovníctve.
func (a *App) doubleChecks(ctx context.Context, companyID int64, year int, add func(YearCheck)) error {
	book, err := a.Ledger(ctx, companyID, year)
	if err != nil {
		return err
	}
	add(YearCheck{ID: "unposted", OK: book.Unposted == 0, Blocking: true, Link: "/hlavna-kniha",
		Text: map[bool]string{true: "Všetky doklady sú zaúčtované",
			false: fmt.Sprintf("V účtovnom denníku je %d dokladov bez zápisu alebo s chybou", book.Unposted)}[book.Unposted == 0]})
	add(YearCheck{ID: "balanced", OK: book.Totals.Balanced, Blocking: true, Link: "/hlavna-kniha",
		Text: map[bool]string{true: "Hlavná kniha je vyrovnaná (MD = D, počiatočné stavy sedia)",
			false: "Hlavná kniha nie je vyrovnaná — skontrolujte počiatočné stavy a ručné zápisy"}[book.Totals.Balanced]})
	st, err := a.LedgerStatementsFor(ctx, companyID, year)
	if err != nil {
		return err
	}
	add(YearCheck{ID: "unmapped", OK: len(st.Unmapped) == 0, Link: "/hlavna-kniha",
		Text: map[bool]string{true: "Všetky účty so zostatkom patria do súvahy alebo výkazu ziskov a strát",
			false: fmt.Sprintf("%d účtov so zostatkom nepatrí do žiadneho riadku závierky", len(st.Unmapped))}[len(st.Unmapped) == 0]})
	if err := a.stockAcquisitionCheck(ctx, companyID, year, book.Accounts, add); err != nil {
		return err
	}
	if err := a.integrityChecks(ctx, companyID, year, add); err != nil {
		return err
	}
	prevOK := true
	if prev, err := a.Ledger(ctx, companyID, year-1); err == nil && len(prev.Postings) > 0 {
		prevOK = a.YearClosed(ctx, companyID, year-1)
	}
	add(YearCheck{ID: "previous", OK: prevOK,
		Text: map[bool]string{true: "Predchádzajúci rok je uzavretý alebo bez zápisov",
			false: fmt.Sprintf("Rok %d ešte nie je uzavretý — odporúčame uzavierať po poradí", year-1)}[prevOK]})
	return nil
}

// closeDouble uzavrie rok v podvojnom účtovníctve: výnosy, náklady,
// výsledok hospodárenia a prenos počiatočných stavov ďalšieho roka (ak
// ešte nie sú zadané).
func (a *App) closeDouble(ctx context.Context, companyID int64, year int, c *YearClose) error {
	book, err := a.Ledger(ctx, companyID, year)
	if err != nil {
		return err
	}
	for _, t := range book.Accounts {
		switch t.Type {
		case chart.Revenue:
			c.Income = c.Income.Sub(t.Closing) // výnosy majú zostatok D (záporný)
		case chart.Expense:
			c.Expenses = c.Expenses.Add(t.Closing)
		}
	}
	c.Result = c.Income.Sub(c.Expenses)
	next, err := a.loadLedger(ctx, companyID, year+1)
	if err != nil {
		return err
	}
	if len(next.Opening) > 0 && !next.OpeningAuto {
		return nil // ručne zadané počiatočné stavy ďalšieho roka ostanú (kontrola nadväznosti ich porovná)
	}
	// prvé uzavretie alebo opakované po otvorení roka: stavy sa prenesú znova
	lines, err := a.OpeningProposal(ctx, companyID, year+1)
	if err != nil {
		return err
	}
	if len(lines) == 0 && len(next.Opening) == 0 {
		return nil
	}
	if err := a.setLedgerOpening(ctx, companyID, year+1, lines, true); err != nil {
		return err
	}
	c.Opening = len(lines) > 0
	return nil
}

// CloseYear uzavrie rok: overí kontroly, uloží súhrn výkazov a konečné
// stavy, prenesie stavy bankových účtov do počiatočných stavov ďalšieho
// roka a rok uzamkne.
func (a *App) CloseYear(ctx context.Context, companyID int64, year int) (YearCloseInfo, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return YearCloseInfo{}, err
	}
	info, err := a.YearCloseFor(ctx, companyID, year)
	if err != nil {
		return info, err
	}
	if info.Close != nil && info.Close.Closed {
		return info, vErr("year", fmt.Sprintf("Rok %d je už uzavretý", year))
	}
	for _, c := range info.Checks {
		if c.Blocking && !c.OK {
			return info, vErr("year", c.Text)
		}
	}
	co, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return info, err
	}
	c := YearClose{Year: year, Closed: true, ClosedAt: a.now(), ClosedBy: a.actorName(ctx), Bank: map[string]money.Dec{}, Cash: map[string]money.Dec{}}
	if info.Close != nil {
		c.History = info.Close.History
	}
	if co.Bookkeeping == store.BookkeepingDouble {
		c.Double = true
		if err := a.closeDouble(ctx, companyID, year, &c); err != nil {
			return info, err
		}
		c.History = append(c.History, YearCloseEvent{At: c.ClosedAt, By: c.ClosedBy, Action: "close"})
		b, _ := json.Marshal(c)
		if err := a.Store.SetSetting(ctx, yearCloseKey(companyID, year), string(b)); err != nil {
			return info, err
		}
		carry := "počiatočné stavy " + strconv.Itoa(year+1) + " ostali zadané"
		if c.Opening {
			carry = "počiatočné stavy prenesené do " + strconv.Itoa(year+1)
		}
		a.record(ctx, companyID, audit.CatLedger, "close_year", "year", int64(year),
			fmt.Sprintf("Uzávierka roka %d: výnosy %s €, náklady %s €, výsledok hospodárenia %s €, %s", year, c.Income.Display(), c.Expenses.Display(), c.Result.Display(), carry), nil, c)
		return a.YearCloseFor(ctx, companyID, year)
	}
	book, err := a.Journal(ctx, companyID, year)
	if err != nil {
		return info, err
	}
	st, err := a.StatementsFor(ctx, companyID, year)
	if err != nil {
		return info, err
	}
	for _, r := range st.Income {
		switch r.Code {
		case "04":
			c.Income = r.Current
		case "11":
			c.Expenses = r.Current
		case "12":
			c.Result = r.Current
		}
	}
	for _, b := range book.Balances {
		// účet bez stavu a pohybov v roku sa neprenáša (neprepíše zadaný stav ďalšieho roka)
		if b.Opening.IsZero() && b.In.IsZero() && b.Out.IsZero() {
			continue
		}
		if b.Means == journal.Cash {
			c.Cash[b.Account] = b.Closing
		} else {
			c.Bank[b.Account] = b.Closing
		}
	}
	// prenos stavov bankových účtov do ďalšieho roka (zadané stavy iných účtov ostanú)
	next, err := a.journalOpening(ctx, companyID, year+1)
	if err != nil {
		return info, err
	}
	if next == nil {
		next = map[string]money.Dec{}
	}
	for k, v := range c.Bank {
		next[k] = v
	}
	if len(next) > 0 {
		b, _ := json.Marshal(next)
		if err := a.Store.SetSetting(ctx, journalOpeningKey(companyID, year+1), string(b)); err != nil {
			return info, err
		}
	}
	c.History = append(c.History, YearCloseEvent{At: c.ClosedAt, By: c.ClosedBy, Action: "close"})
	b, _ := json.Marshal(c)
	if err := a.Store.SetSetting(ctx, yearCloseKey(companyID, year), string(b)); err != nil {
		return info, err
	}
	a.record(ctx, companyID, audit.CatLedger, "close_year", "year", int64(year),
		fmt.Sprintf("Uzávierka roka %d: príjmy %s €, výdavky %s €, stavy účtov prenesené do %d", year, c.Income.Display(), c.Expenses.Display(), year+1), nil, c)
	return a.YearCloseFor(ctx, companyID, year)
}

// ReopenYear znova otvorí uzavretý rok (oprava); dôvod sa zapíše.
// Nasledujúci uzavretý rok treba otvoriť skôr.
func (a *App) ReopenYear(ctx context.Context, companyID int64, year int, reason string) (YearCloseInfo, error) {
	if err := a.Require(ctx, companyID, auth.PermLedgerWrite); err != nil {
		return YearCloseInfo{}, err
	}
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) < 5 {
		return YearCloseInfo{}, vErr("reason", "Napíšte, prečo rok otvárate (zapíše sa do auditu)")
	}
	c, err := a.yearClose(ctx, companyID, year)
	if err != nil {
		return YearCloseInfo{}, err
	}
	if c == nil || !c.Closed {
		return YearCloseInfo{}, vErr("year", fmt.Sprintf("Rok %d nie je uzavretý", year))
	}
	if a.YearClosed(ctx, companyID, year+1) {
		return YearCloseInfo{}, vErr("year", fmt.Sprintf("Najprv otvorte rok %d — uzávierky nadväzujú", year+1))
	}
	c.Closed = false
	c.History = append(c.History, YearCloseEvent{At: a.now(), By: a.actorName(ctx), Action: "reopen", Reason: reason})
	b, _ := json.Marshal(c)
	if err := a.Store.SetSetting(ctx, yearCloseKey(companyID, year), string(b)); err != nil {
		return YearCloseInfo{}, err
	}
	a.record(ctx, companyID, audit.CatLedger, "reopen_year", "year", int64(year), fmt.Sprintf("Otvorený rok %d: %s", year, reason), nil, nil)
	return a.YearCloseFor(ctx, companyID, year)
}

// stockAcquisitionCheck upozorní na zostatok obstarávacích účtov 111 a 131
// (zásoby spôsobom A). Príjemka účtuje 112/111 a 132/131, prijatá faktúra
// so zásobami 111 alebo 131 len vtedy, keď je príjemka k nej pripojená —
// inak ide faktúra na 501 a obstarávací účet ostane so zostatkom (zásoby
// v súvahe vychádzajú nesprávne, aj záporné).
func (a *App) stockAcquisitionCheck(ctx context.Context, companyID int64, year int, accounts []ledger.Turnover, add func(YearCheck)) error {
	bal := map[string]money.Dec{"111": money.Zero, "131": money.Zero}
	for _, t := range accounts {
		for acc := range bal {
			if strings.HasPrefix(t.Account, acc) {
				bal[acc] = bal[acc].Add(t.Closing)
			}
		}
	}
	if bal["111"].IsZero() && bal["131"].IsZero() {
		add(YearCheck{ID: "stock", OK: true, Text: "Obstarávacie účty zásob 111 a 131 sú bez zostatku"})
		return nil
	}
	docs, err := a.Store.ListStockDocs(ctx, companyID)
	if err != nil {
		return err
	}
	unlinked := 0
	for _, d := range docs {
		// príjemka z objednávky prijatú faktúru pripojenú nemá (faktúra ešte neprišla alebo ide mimo objednávky)
		if d.Kind == stock.Receipt && !d.Return && d.Date.Year() == year && (d.LinkKind == "" || d.LinkKind == "porder") {
			unlinked++
		}
	}
	var parts []string
	for _, acc := range []string{"111", "131"} {
		if v := bal[acc]; !v.IsZero() {
			parts = append(parts, fmt.Sprintf("%s: %s €", acc, v.Display()))
		}
	}
	text := "Obstarávacie účty zásob majú zostatok (" + strings.Join(parts, ", ") + ")"
	if unlinked > 0 {
		word := "príjemok"
		switch {
		case unlinked == 1:
			word = "príjemka"
		case unlinked < 5:
			word = "príjemky"
		}
		text += fmt.Sprintf(" — %d %s bez pripojenej prijatej faktúry. Pripojte faktúru k príjemke", unlinked, word)
	} else {
		text += " — skontrolujte predkontácie nákupu zásob"
	}
	text += "; tovar bez faktúry patrí na 326, faktúra bez príjemky na 139 alebo 119"
	add(YearCheck{ID: "stock", OK: false, Link: "/sklad", Text: text})
	return nil
}
