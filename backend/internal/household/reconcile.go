package household

import (
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
)

// Reconciled je odsúhlasený zostatok účtu: k dátumu sa zostatok vo Fiškuse
// zhodoval so zostatkom na výpise.
type Reconciled struct {
	Date    invoice.Date `json:"date"`
	Balance money.Dec    `json:"balance"`
}

// BalanceAt vráti zostatok účtu na konci dňa date (počiatočný stav a
// pohyby do toho dňa vrátane).
func BalanceAt(acc Account, entries []Entry, date invoice.Date) money.Dec {
	bal := acc.Opening
	for _, e := range entries {
		if e.Date.After(date) {
			continue
		}
		switch {
		case e.Transfer() && e.AccountID == acc.ID:
			bal = bal.Sub(e.Amount)
		case e.Transfer() && e.ToAccountID == acc.ID:
			bal = bal.Add(e.Amount)
		case e.AccountID == acc.ID:
			bal = bal.Add(e.Amount)
		}
	}
	return bal
}

// Reconciliation je porovnanie zostatku vo Fiškuse so zostatkom na výpise.
type Reconciliation struct {
	Date       invoice.Date `json:"date"`
	Statement  money.Dec    `json:"statement"`  // zostatok na výpise
	Book       money.Dec    `json:"book"`       // zostatok vo Fiškuse k dátumu
	Difference money.Dec    `json:"difference"` // výpis − Fiškus (o toľko treba opraviť)
	Later      int          `json:"later"`      // pohyby účtu po dátume (do porovnania nevstupujú)
}

// Reconcile porovná zostatok účtu k dátumu so zostatkom z výpisu.
func Reconcile(acc Account, entries []Entry, date invoice.Date, statement money.Dec) Reconciliation {
	r := Reconciliation{Date: date, Statement: statement, Book: BalanceAt(acc, entries, date)}
	r.Difference = statement.Sub(r.Book)
	for _, e := range entries {
		if e.Date.After(date) && (e.AccountID == acc.ID || e.ToAccountID == acc.ID) {
			r.Later++
		}
	}
	return r
}

// Stale: zmenili sa od odsúhlasenia pohyby do jeho dátumu? Vráti rozdiel
// medzi dnešným zostatkom k dátumu odsúhlasenia a odsúhlaseným zostatkom
// (nula = odsúhlasenie platí, alebo účet ešte odsúhlasený nebol).
func Stale(acc Account, entries []Entry) money.Dec {
	if acc.Reconciled == nil {
		return money.Zero
	}
	return BalanceAt(acc, entries, acc.Reconciled.Date).Sub(acc.Reconciled.Balance)
}

// AdjustCategory vyberie kategóriu pre vyrovnanie rozdielu: „Iné príjmy“
// alebo „Iné výdavky“, ak sú v zozname, inak poslednú kategóriu daného druhu.
func AdjustCategory(cats []Category, income bool) string {
	want := "Iné výdavky"
	if income {
		want = "Iné príjmy"
	}
	last := ""
	for _, c := range cats {
		if c.Income != income {
			continue
		}
		if c.Name == want {
			return c.Name
		}
		last = c.Name
	}
	return last
}
