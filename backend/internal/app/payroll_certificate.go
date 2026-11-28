package app

import (
	"github.com/fiskus-online/fiskus/backend/internal/dpfo"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/payroll"
)

// Potvrdenie o zdaniteľných príjmoch fyzickej osoby zo závislej činnosti
// (§ 39 ods. 5 zákona o dani z príjmov, vzor POT395v26 — informácia FR SR
// 42/DZPaU/2025/I). Vystaví ho zamestnávateľ zo mzdového listu, ak oň
// zamestnanec požiada: do 10. februára, ak ročné zúčtovanie urobí iný
// zamestnávateľ, inak do 10. marca, ak zamestnanec podáva priznanie.
// Obsahuje iba príjmy od tohto zamestnávateľa za uzavreté mesiace roka.

// CertificateChild je r. 14: dieťa, na ktoré zamestnávateľ priznal
// a vyplatil daňový bonus, s mesiacmi bonusu.
type CertificateChild struct {
	Name   string `json:"name"`
	RC     string `json:"rc"`
	Months []int  `json:"months"`
}

// Certificate sú údaje II. oddielu potvrdenia.
type Certificate struct {
	Rows     map[string]string  `json:"rows"`   // r. 01, 01a, 02, 02a, 02b, 03, 04, 11
	Months   []int              `json:"months"` // r. 13
	Children []CertificateChild `json:"children"`
}

// certAcc zbiera údaje potvrdenia po mesiacoch.
type certAcc struct {
	income, agree, social, health, tax, nczd money.Dec
	months                                   []int
	children                                 []CertificateChild
}

// add pripočíta mesiac mzdy; deti s bonusom určí z karty zamestnanca v tom
// mesiaci (dieťa v domácnosti a vo veku na bonus).
func (c *certAcc) add(month int, e payroll.Employee, r payroll.Result, p dpfo.Params) {
	c.months = append(c.months, month)
	c.income = c.income.Add(r.Income)
	if e.Contract.Agreement() {
		c.agree = c.agree.Add(r.Income)
	}
	c.social, c.health = c.social.Add(r.SocialEmployee), c.health.Add(r.HealthEmployee)
	c.tax, c.nczd = c.tax.Add(r.Tax), c.nczd.Add(r.NCZD)
	if r.TaxBonus.Sign() <= 0 {
		return
	}
	for _, ch := range e.Children {
		if b, err := dpfo.MonthlyBonus([]dpfo.Child{ch}, r.TaxBase, month, p); err != nil || b.Sign() <= 0 {
			continue
		}
		i := 0
		for i < len(c.children) && !(c.children[i].RC == ch.RC && c.children[i].Name == ch.Name) {
			i++
		}
		if i == len(c.children) {
			c.children = append(c.children, CertificateChild{Name: ch.Name, RC: ch.RC, Months: []int{}})
		}
		c.children[i].Months = append(c.children[i].Months, month)
	}
}

func (c *certAcc) certificate() Certificate {
	contrib := c.social.Add(c.health)
	r := map[string]money.Dec{"01": c.income, "01a": c.agree, "02": contrib, "02a": c.social, "02b": c.health,
		"03": c.income.Sub(contrib), "04": c.tax, "11": c.nczd}
	if r["03"].Sign() < 0 {
		r["03"] = money.Zero
	}
	out := Certificate{Rows: make(map[string]string, len(r)), Months: c.months, Children: c.children}
	for k, v := range r {
		out.Rows[k] = v.RoundCents().String()
	}
	if out.Months == nil {
		out.Months = []int{}
	}
	if out.Children == nil {
		out.Children = []CertificateChild{}
	}
	return out
}
