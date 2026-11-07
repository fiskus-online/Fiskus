package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/cash"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
)

// Uzávierka e-kasy sa zapíše do pokladne dvoma dokladmi:
//   - PPD na celú tržbu s DPH podľa sadzieb (daň na výstupe, KV D.1;
//     peňažný denník d1/d2, v podvojnom účtovníctve 211/604 alebo 602, 343),
//   - VPD na platby kartou — peniaze na ceste (denník c, účet 261), kým ich
//     banka nepripíše; pripísanie z terminálu je potom tiež priebežná položka.

// CashClosings vráti uzávierky e-kasy roka.
func (a *App) CashClosings(ctx context.Context, companyID int64, year int) ([]cash.Closing, error) {
	if err := a.Require(ctx, companyID, auth.PermCashRead); err != nil {
		return nil, err
	}
	return a.Store.ListCashClosings(ctx, companyID, year)
}

// PreviewCashClosing dopočíta základ a DPH podľa sadzieb a časť v hotovosti.
func (a *App) PreviewCashClosing(ctx context.Context, companyID int64, c cash.Closing) (ClosingPreview, error) {
	if err := a.Require(ctx, companyID, auth.PermCashRead); err != nil {
		return ClosingPreview{}, err
	}
	company, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return ClosingPreview{}, err
	}
	c.Normalize(company.VATPayer)
	out := ClosingPreview{Closing: c, Total: c.Total(), CashPart: c.CashPart(), Errors: []invoice.FieldError{}}
	var ve *invoice.ValidationError
	if err := c.Validate(company.VATPayer); errors.As(err, &ve) {
		out.Errors = ve.Fields
	}
	return out, nil
}

// ClosingPreview je uzávierka po prepočte so súčtami a chybami.
type ClosingPreview struct {
	cash.Closing
	Total    money.Dec            `json:"total"`
	CashPart money.Dec            `json:"cashPart"`
	Errors   []invoice.FieldError `json:"errors"`
}

// CreateCashClosing zapíše uzávierku e-kasy do pokladne (PPD na tržbu, VPD na platby kartou).
func (a *App) CreateCashClosing(ctx context.Context, companyID int64, c cash.Closing) (cash.Closing, error) {
	if err := a.Require(ctx, companyID, auth.PermCashWrite); err != nil {
		return c, err
	}
	company, err := a.Store.GetCompany(ctx, companyID)
	if err != nil {
		return c, err
	}
	c.ID, c.InID, c.OutID, c.InNumber, c.OutNumber, c.Cancelled = 0, 0, 0, "", "", false
	c.Normalize(company.VATPayer)
	if err := c.Validate(company.VATPayer); err != nil {
		return c, err
	}
	if err := a.checkOpen(ctx, companyID, c.Date); err != nil {
		return c, err
	}
	reg, err := a.cashRegister(ctx, companyID, c.RegisterID)
	if err != nil {
		return c, err
	}
	if reg.Currency != "" && reg.Currency != "EUR" {
		return c, vErr("registerId", "Tržba z e-kasy sa zapisuje do pokladne v eurách")
	}
	existing, err := a.Store.ListCashClosings(ctx, companyID, 0)
	if err != nil {
		return c, err
	}
	for _, o := range existing {
		if !o.Cancelled && c.Number != "" && o.Number == c.Number && o.ORP == c.ORP {
			return c, vErr("number", fmt.Sprintf("Uzávierka %s je už zapísaná (%s, %s)", c.Number, o.Date.Display(), o.InNumber))
		}
	}
	if err := a.Store.SaveCashClosing(ctx, companyID, &c); err != nil {
		return c, err
	}
	undo := func(cause error) (cash.Closing, error) {
		if c.OutID != 0 {
			_, _ = a.cancelCash(ctx, companyID, c.OutID, true)
		}
		if c.InID != 0 {
			_, _ = a.cancelCash(ctx, companyID, c.InID, true)
		}
		_ = a.Store.DeleteCashClosing(ctx, companyID, c.ID)
		c.ID = 0
		return c, cause
	}
	label := c.Label()
	in := cash.Document{Kind: cash.In, Date: c.Date, Amount: c.Total(), RegisterID: c.RegisterID, Partner: "Tržby z e-kasy",
		Purpose: label, Note: c.Note, LinkKind: cash.LinkClosing, LinkID: c.ID}
	if company.VATPayer {
		in.Tax = &cash.Tax{Rows: c.Rows, Goods: c.Goods}
	}
	saved, err := a.CreateCash(ctx, companyID, in)
	if err != nil {
		return undo(err)
	}
	c.InID, c.InNumber = saved.ID, saved.Number
	if c.Card.Sign() > 0 {
		out := cash.Document{Kind: cash.Out, Date: c.Date, Amount: c.Card, RegisterID: c.RegisterID, Partner: "Platby kartou",
			Purpose: "Platby kartou na ceste na účet – " + label, LinkKind: cash.LinkClosing, LinkID: c.ID}
		saved, err := a.CreateCash(ctx, companyID, out)
		if err != nil {
			return undo(err)
		}
		c.OutID, c.OutNumber = saved.ID, saved.Number
	}
	if err := a.Store.SaveCashClosing(ctx, companyID, &c); err != nil {
		return undo(err)
	}
	a.record(ctx, companyID, audit.CatCash, "create", "cash_closing", c.ID,
		fmt.Sprintf("%s: tržba %s €, kartou %s € (%s %s)", label, c.Total().Display(), c.Card.Display(), c.InNumber, c.OutNumber), nil, c)
	return c, nil
}

// CancelCashClosing stornuje uzávierku aj jej pokladničné doklady.
func (a *App) CancelCashClosing(ctx context.Context, companyID, id int64) (cash.Closing, error) {
	if err := a.Require(ctx, companyID, auth.PermCashWrite); err != nil {
		return cash.Closing{}, err
	}
	c, err := a.Store.GetCashClosing(ctx, companyID, id)
	if err != nil {
		return c, err
	}
	if c.Cancelled {
		return c, cash.ErrCancelled
	}
	if err := a.checkOpen(ctx, companyID, c.Date); err != nil {
		return c, err
	}
	// najprv výdavok (platby kartou), inak by pokladňa mohla klesnúť pod nulu
	for _, docID := range []int64{c.OutID, c.InID} {
		if docID == 0 {
			continue
		}
		if _, err := a.cancelCash(ctx, companyID, docID, true); err != nil && !errors.Is(err, cash.ErrCancelled) {
			return c, err
		}
	}
	c.Cancelled = true
	if err := a.Store.SaveCashClosing(ctx, companyID, &c); err != nil {
		return c, err
	}
	a.record(ctx, companyID, audit.CatCash, "cancel", "cash_closing", c.ID, "Stornovaná "+c.Label(), nil, nil)
	return c, nil
}
