package ledger

import (
	"sort"

	"github.com/fiskus-online/fiskus/backend/internal/money"
)

// Uzávierkové a otváracie účtovné zápisy (postupy účtovania pre
// podnikateľov, účtová trieda 7): k poslednému dňu roka sa nákladové
// a výnosové účty uzavrú na 710 – Účet ziskov a strát, výsledok
// hospodárenia sa prevedie na 702 – Konečný účet súvahový a súvahové účty
// sa uzavrú na 702; k prvému dňu ďalšieho roka sa súvahové účty otvoria
// cez 701 – Začiatočný účet súvahový a výsledok minulého roka ide na 431.
//
// Zápisy sú doklad o uzavretí účtovných kníh — do obratov roka sa
// nezapočítavajú, výkazy sa zostavujú zo zostatkov pred uzavretím.

// Entry je jeden uzávierkový alebo otvárací zápis (skupina riadkov).
type Entry struct {
	Text  string `json:"text"`
	Lines []Line `json:"lines"`
}

const (
	accOpen   = "701"
	accClose  = "702"
	accProfit = "710"
	accResult = "431"
)

// signed vráti riadok so zostatkom v (kladný MD, záporný D).
func signed(acc string, v money.Dec) Line {
	if v.Sign() >= 0 {
		return Line{Account: acc, Debit: v}
	}
	return Line{Account: acc, Credit: v.Neg()}
}

// ClosingEntries vráti uzávierkové zápisy roka zo zostatkov účtov:
// uzavretie nákladov a výnosov na 710, prevod výsledku na 702 a uzavretie
// súvahových účtov na 702. Po nich majú všetky účty (aj 702 a 710) nulový
// zostatok. Vráti aj výsledok hospodárenia (kladný = zisk).
func ClosingEntries(accounts []Turnover) ([]Entry, money.Dec) {
	sorted := append([]Turnover(nil), accounts...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Account < sorted[j].Account })
	var costs, revenues, balance Entry
	costs.Text = "Uzavretie nákladových účtov na účet ziskov a strát"
	revenues.Text = "Uzavretie výnosových účtov na účet ziskov a strát"
	balance.Text = "Uzavretie súvahových účtov na konečný účet súvahový"
	pl, bs := money.Zero, money.Zero // zostatok 710 a 702 (MD kladný)
	for _, t := range sorted {
		if t.Account == "" || t.Closing.IsZero() {
			continue
		}
		switch c := t.Account[0]; {
		case c == '5':
			// zostatok MD sa uzavrie na D účtu, protistrana 710 MD
			costs.Lines = append(costs.Lines, signed(t.Account, t.Closing.Neg()))
			pl = pl.Add(t.Closing)
		case c == '6':
			revenues.Lines = append(revenues.Lines, signed(t.Account, t.Closing.Neg()))
			pl = pl.Add(t.Closing)
		case c <= '4':
			balance.Lines = append(balance.Lines, signed(t.Account, t.Closing.Neg()))
			bs = bs.Add(t.Closing)
		}
	}
	result := pl.Neg() // výnosy (D) prevyšujú náklady (MD) = zisk
	var out []Entry
	if len(costs.Lines) > 0 {
		sum := money.Zero
		for _, l := range costs.Lines {
			sum = sum.Add(l.Credit).Sub(l.Debit)
		}
		costs.Lines = append([]Line{signed(accProfit, sum)}, costs.Lines...)
		out = append(out, costs)
	}
	if len(revenues.Lines) > 0 {
		sum := money.Zero
		for _, l := range revenues.Lines {
			sum = sum.Add(l.Debit).Sub(l.Credit)
		}
		revenues.Lines = append(revenues.Lines, signed(accProfit, sum.Neg()))
		out = append(out, revenues)
	}
	if !result.IsZero() {
		text := "Prevod zisku na konečný účet súvahový"
		if result.Sign() < 0 {
			text = "Prevod straty na konečný účet súvahový"
		}
		// zisk: 710 MD / 702 D; strata: 702 MD / 710 D
		lines := []Line{signed(accProfit, result), signed(accClose, result.Neg())}
		if result.Sign() < 0 {
			lines[0], lines[1] = lines[1], lines[0] // MD prvé
		}
		out = append(out, Entry{Text: text, Lines: lines})
	}
	if len(balance.Lines) > 0 {
		// aktíva (MD) idú na 702 MD, pasíva na 702 D; súvahové zostatky
		// s výsledkom dávajú nulu, preto 702 vyrovnáva iba súvahové účty
		lines := append([]Line(nil), balance.Lines...)
		lines = append(lines, signed(accClose, bs))
		out = append(out, Entry{Text: balance.Text, Lines: lines})
	}
	return out, result
}

// OpeningEntries vráti otváracie zápisy ďalšieho roka cez 701 z konečných
// zostatkov: súvahové účty a výsledok hospodárenia na 431 (ako
// OpeningFromClosing).
func OpeningEntries(accounts []Turnover) []Entry {
	lines := OpeningFromClosing(accounts)
	if len(lines) == 0 {
		return nil
	}
	debit, credit := money.Zero, money.Zero
	for _, l := range lines {
		debit, credit = debit.Add(l.Debit), credit.Add(l.Credit)
	}
	// protistranou každého počiatočného stavu je 701: aktíva 701 D,
	// pasíva a výsledok 701 MD (spolu sa 701 vyrovná)
	out := append([]Line(nil), lines...)
	if !debit.IsZero() {
		out = append(out, Line{Account: accOpen, Credit: debit})
	}
	if !credit.IsZero() {
		out = append(out, Line{Account: accOpen, Debit: credit})
	}
	return []Entry{{Text: "Otvorenie súvahových účtov cez začiatočný účet súvahový", Lines: out}}
}
