package cash

import (
	"fmt"
	"strings"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
)

// LinkClosing je väzba pokladničného dokladu na uzávierku e-kasy.
const LinkClosing = "closing"

// Closing je uzávierka e-kasy (denná alebo intervalová): obrat podľa sadzieb
// DPH a časť zaplatená bezhotovostne. Zapíše sa do pokladne ako príjmový
// doklad na celú tržbu (daň na výstupe, KV D.1) a výdavkový doklad na platby
// kartou — tie sú peniaze na ceste, kým ich banka nepripíše na účet.
type Closing struct {
	ID         int64        `json:"id"`
	CompanyID  int64        `json:"companyId"`
	RegisterID int64        `json:"registerId"`
	Date       invoice.Date `json:"date"`
	ORP        string       `json:"orp"`    // kód pokladnice e-kasa klient
	Number     string       `json:"number"` // číslo uzávierky
	Receipts   int          `json:"receipts"`
	// Rows je obrat s DPH podľa sadzieb (po odpočítaní vrátených a stornovaných);
	// neplatiteľ DPH má jeden riadok so sadzbou 0.
	Rows []VATRow  `json:"rows"`
	Card money.Dec `json:"card"` // zaplatené kartou a inak bezhotovostne
	// Goods: tržba za predaj tovaru (peňažný denník d1, účet 604), inak výrobky a služby.
	Goods     bool   `json:"goods"`
	Note      string `json:"note"`
	InID      int64  `json:"inId"`
	InNumber  string `json:"inNumber"`
	OutID     int64  `json:"outId"`
	OutNumber string `json:"outNumber"`
	Cancelled bool   `json:"cancelled"`
}

// Total je celá tržba s DPH.
func (c Closing) Total() money.Dec {
	t := money.Zero
	for _, r := range c.Rows {
		t = t.Add(r.Gross)
	}
	return t
}

// CashPart je tržba v hotovosti.
func (c Closing) CashPart() money.Dec { return c.Total().Sub(c.Card) }

// Label je text na pokladničné doklady: „Tržba e-kasa, uzávierka 12 (ORP 88812345678900001)“.
func (c Closing) Label() string {
	s := "Tržba e-kasa"
	if c.Number != "" {
		s += ", uzávierka " + c.Number
	} else {
		s += " " + c.Date.Display()
	}
	if c.ORP != "" {
		s += " (ORP " + c.ORP + ")"
	}
	return s
}

// Normalize zlúči sadzby, dopočíta základ a daň; neplatiteľovi zlúči obrat
// do jedného riadku bez DPH.
func (c *Closing) Normalize(vatPayer bool) {
	c.ORP = strings.Join(strings.Fields(c.ORP), "")
	c.Number = strings.TrimSpace(c.Number)
	c.Note = strings.TrimSpace(c.Note)
	if !vatPayer {
		t := c.Total()
		c.Rows = []VATRow{{Rate: 0, Gross: t, Base: t}}
		return
	}
	d := Document{Kind: In, Tax: &Tax{Rows: c.Rows}}
	d.NormalizeTax()
	c.Rows = []VATRow{}
	if d.Tax != nil {
		c.Rows = d.Tax.Rows
	}
}

// Validate overí uzávierku (po Normalize).
func (c Closing) Validate(vatPayer bool) error {
	var fe []invoice.FieldError
	add := func(f, m string) { fe = append(fe, invoice.FieldError{Field: f, Message: m}) }
	if c.Date.IsZero() {
		add("date", "Zadajte dátum uzávierky")
	}
	total := c.Total()
	for i, r := range c.Rows {
		switch {
		case !r.Gross.Equal(r.Gross.RoundCents()):
			add(fmt.Sprintf("rows.%d.gross", i), "Suma môže mať najviac dve desatinné miesta")
		case vatPayer && (!r.Rate.Known() || (!c.Date.IsZero() && !r.Rate.ValidOn(c.Date.String()))):
			add(fmt.Sprintf("rows.%d.vatRate", i), fmt.Sprintf("Sadzba %d %% v tento deň neplatí", r.Rate))
		}
	}
	if total.Sign() <= 0 {
		add("rows", "Zadajte obrat z uzávierky")
	}
	switch {
	case c.Card.Sign() < 0:
		add("card", "Platby kartou nemôžu byť záporné")
	case !c.Card.Equal(c.Card.RoundCents()):
		add("card", "Suma môže mať najviac dve desatinné miesta")
	case total.Sign() > 0 && c.Card.Cmp(total) > 0:
		add("card", fmt.Sprintf("Platby kartou sú vyššie ako celá tržba (%s €)", total.Display()))
	}
	if c.Receipts < 0 {
		add("receipts", "Počet dokladov nemôže byť záporný")
	}
	if len(fe) > 0 {
		return &invoice.ValidationError{Fields: fe}
	}
	return nil
}
