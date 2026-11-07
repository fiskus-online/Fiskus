package household

import (
	"sort"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/money"
)

// Bloček v domácnosti: nákup rozdelený na položky, každá s kategóriou
// (potraviny, drogéria, …). Kategóriu položky navrhne história (ako ste
// rovnakú položku zaradili naposledy), potom známe tovary, inak kategória
// obchodu.

// ReceiptItem je položka bločku (cena za riadok s DPH, zľava záporná).
type ReceiptItem struct {
	Name     string    `json:"name"`
	Quantity string    `json:"quantity"`
	Amount   money.Dec `json:"amount"`
	Category string    `json:"category"`
}

// itemRules: známe tovary podľa začiatku slova (bez diakritiky). Poradie
// rozhoduje (motorový olej skôr ako olej).
var itemRules = []struct {
	words []string
	cat   string
}{
	{[]string{"nafta", "diesel", "benzin", "natural 95", "natural95", "nat 95", "lpg", "ostrekovac", "motorovy olej", "olej motor", "dialnicna", "parkovne", "umyvanie auta"}, "Auto a doprava"},
	{[]string{"ibalgin", "paralen", "nurofen", "aspirin", "vitamin", "magnezium", "magnesium", "kvapky", "sirup", "naplast", "obvaz", "tablet", "lieky", "homeopat", "probiotik", "rusko", "respirator"}, "Zdravie"},
	{[]string{"sampon", "mydlo", "zubna", "zubna pasta", "kefka", "toaletn", "toal pap", "wc ", "prac", "avivaz", "jar ", "cistic", "sprchov", "deodorant", "antiperspir", "plienk", "pampers", "vlhcen", "utierk", "servitk", "vrecia", "vrecka na", "alobal", "potravinova folia", "papierove", "kuchynske", "osviezovac", "krem ", "telove mlieko", "zilet", "vatove", "tampon", "vlozky", "baterie", "ziarovk", "sviecka", "drogeria", "dezinf", "persil", "ariel", "lenor", "domestos", "savo", "pur ", "fixinela"}, "Drogéria a domácnosť"},
	{[]string{"zosit", "pastelk", "pero", "ceruzk", "farbicky", "lepidlo", "nozni", "skolsk", "obal na", "plastelina", "hracka", "lego", "detsk"}, "Deti a škola"},
	{[]string{"tricko", "ponozk", "nohavic", "bunda", "mikina", "spodn", "pancuch", "obuv", "topanky", "tenisky", "silonk"}, "Oblečenie"},
	{[]string{"casopis", "noviny", "kniha", "listok", "vstupenk", "kino", "hra "}, "Voľný čas"},
	// potraviny posledné — najširšie
	{[]string{"rozok", "rozky", "chlieb", "chleba", "bageta", "pecivo", "kaiserk", "kajzerk", "croissant", "loupak", "buchta", "kolac", "kolac",
		"mlieko", "mliek", "syr", "eidam", "gouda", "mozzarel", "maslo", "jogurt", "smotan", "tvaroh", "bryndz", "kefir", "acidko", "zakys", "puding", "vajc",
		"salam", "sunk", "parky", "parok", "klobas", "slanin", "spekacik", "pastet", "kura", "kurac", "bravc", "hovad", "morc", "mlete", "maso", "rezne", "ryba", "losos", "tuniak",
		"ryza", "cestovin", "spaget", "penne", "muka", "cukor", "sol ", "korenie", "olej", "ocot", "kecup", "horcica", "majonez", "omacka", "polievk", "bujon",
		"zemiak", "cibul", "cesnak", "mrkva", "paradajk", "rajcin", "uhork", "paprik", "kapust", "salat", "zelenin", "jablk", "hrusk", "banan", "pomaranc", "mandarink", "citron", "hrozno", "jahod", "ovocie", "avokad", "kiwi",
		"kava", "caj", "kakao", "voda", "minerál", "mineral", "dzus", "dzus", "limonad", "sirup", "kofola", "coca", "pepsi", "fanta", "sprite", "pivo", "vino", "rum", "vodka", "borovick", "slivovic",
		"cokolad", "susienk", "keks", "oblatk", "chips", "cipsy", "tycink", "orieskov", "orech", "mandle", "zmrzlin", "cukrik", "zuvack", "muesli", "musli", "vlocky", "cornflakes",
		"konzerv", "fazula", "sosovic", "hrach", "kukuric", "pizza", "lasagn", "knedl", "halusk", "pirohy", "bryndz", "potravin"}, "Potraviny"},
}

// SuggestItem navrhne kategóriu položky bločku: história položiek (rovnaký
// názov), potom známe tovary; "" = nevie (použije sa kategória obchodu).
func SuggestItem(name string, history []Entry, cats []Category) string {
	have := map[string]bool{}
	for _, c := range cats {
		if !c.Income {
			have[c.Name] = true
		}
	}
	key := fold(name)
	if key == "" {
		return ""
	}
	hist := append([]Entry(nil), history...)
	sort.SliceStable(hist, func(i, j int) bool { return hist[j].Date.Before(hist[i].Date) })
	for _, e := range hist {
		for _, x := range e.Split {
			if have[x.Category] && fold(x.Note) == key {
				return x.Category
			}
		}
	}
	padded := " " + key + " "
	for _, r := range itemRules {
		if !have[r.cat] {
			continue
		}
		for _, w := range r.words {
			if strings.Contains(padded, " "+w) {
				return r.cat
			}
		}
	}
	return ""
}

// ReceiptSplit rozdelí nákup na položky so zápornými sumami (výdavok).
// Položky bez kategórie dostanú kategóriu obchodu; rozdiel medzi súčtom
// položiek a celkovou sumou (zaokrúhlenie, nečitateľná položka) je
// samostatná položka. Vráti položky a zápornú sumu pohybu.
func ReceiptSplit(items []ReceiptItem, total money.Dec, shopCategory string) ([]Split, money.Dec) {
	out := make([]Split, 0, len(items)+1)
	sum := money.Zero
	for _, it := range items {
		amt := it.Amount.RoundCents()
		if amt.IsZero() {
			continue
		}
		cat := it.Category
		if cat == "" {
			cat = shopCategory
		}
		out = append(out, Split{Category: cat, Amount: amt.Neg(), Note: strings.TrimSpace(it.Name), Quantity: strings.TrimSpace(it.Quantity)})
		sum = sum.Add(amt)
	}
	total = total.RoundCents()
	if total.IsZero() {
		total = sum
	}
	if diff := total.Sub(sum); !diff.IsZero() {
		note := "Rozdiel k celkovej sume"
		if diff.Cmp(money.MustParse("0.05")) <= 0 && diff.Cmp(money.MustParse("-0.05")) >= 0 {
			note = "Zaokrúhlenie"
		}
		out = append(out, Split{Category: shopCategory, Amount: diff.Neg(), Note: note})
	}
	return out, total.Neg()
}
