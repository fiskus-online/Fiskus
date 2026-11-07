//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/fiskus-online/fiskus/backend/internal/household"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func (s *Store) ListHHAccounts(ctx context.Context, companyID int64) ([]household.Account, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, data FROM hh_accounts WHERE company_id = ? ORDER BY id", companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []household.Account{}
	for rows.Next() {
		var a household.Account
		var id int64
		var data string
		if err := rows.Scan(&id, &data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &a); err != nil {
			return nil, err
		}
		a.ID, a.CompanyID = id, companyID
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) SaveHHAccount(ctx context.Context, companyID int64, a *household.Account) error {
	a.CompanyID = companyID
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	if a.ID == 0 {
		res, err := s.db.ExecContext(ctx, "INSERT INTO hh_accounts (company_id, data) VALUES (?, ?)", companyID, string(data))
		if err != nil {
			return err
		}
		a.ID, err = res.LastInsertId()
		return err
	}
	res, err := s.db.ExecContext(ctx, "UPDATE hh_accounts SET data = ? WHERE id = ? AND company_id = ?", string(data), a.ID, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteHHAccount(ctx context.Context, companyID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM hh_accounts WHERE id = ? AND company_id = ?", id, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) ListHHEntries(ctx context.Context, companyID int64, year int) ([]household.Entry, error) {
	q := "SELECT id, data FROM hh_entries WHERE company_id = ?"
	args := []any{companyID}
	if year != 0 {
		q += " AND substr(date, 1, 4) = ?"
		args = append(args, fmtYear(year))
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY date, id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []household.Entry{}
	for rows.Next() {
		var e household.Entry
		var id int64
		var data string
		if err := rows.Scan(&id, &data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &e); err != nil {
			return nil, err
		}
		e.ID, e.CompanyID = id, companyID
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) GetHHEntry(ctx context.Context, companyID, id int64) (household.Entry, error) {
	var e household.Entry
	var data string
	err := s.db.QueryRowContext(ctx, "SELECT data FROM hh_entries WHERE id = ? AND company_id = ?", id, companyID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return e, store.ErrNotFound
	}
	if err != nil {
		return e, err
	}
	if err := json.Unmarshal([]byte(data), &e); err != nil {
		return e, err
	}
	e.ID, e.CompanyID = id, companyID
	return e, nil
}

func (s *Store) SaveHHEntry(ctx context.Context, companyID int64, e *household.Entry) error {
	e.CompanyID = companyID
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if e.ID == 0 {
		res, err := s.db.ExecContext(ctx, "INSERT INTO hh_entries (company_id, date, source, data) VALUES (?, ?, ?, ?)",
			companyID, e.Date.String(), e.Source, string(data))
		if isUnique(err) {
			return store.ErrDuplicate
		}
		if err != nil {
			return err
		}
		e.ID, err = res.LastInsertId()
		return err
	}
	res, err := s.db.ExecContext(ctx, "UPDATE hh_entries SET date = ?, source = ?, data = ? WHERE id = ? AND company_id = ?",
		e.Date.String(), e.Source, string(data), e.ID, companyID)
	if isUnique(err) {
		return store.ErrDuplicate
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteHHEntry(ctx context.Context, companyID, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, "DELETE FROM hh_entries WHERE id = ? AND company_id = ?", id, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM hh_photos WHERE entry_id = ? AND company_id = ?", id, companyID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SaveHHPhoto(ctx context.Context, p store.HHPhoto) error {
	var n int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM hh_entries WHERE id = ? AND company_id = ?", p.EntryID, p.CompanyID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return store.ErrNotFound
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO hh_photos (entry_id, company_id, content_type, data) VALUES (?, ?, ?, ?)
		ON CONFLICT (entry_id) DO UPDATE SET content_type = excluded.content_type, data = excluded.data`, p.EntryID, p.CompanyID, p.ContentType, p.Data)
	return err
}

func (s *Store) GetHHPhoto(ctx context.Context, companyID, entryID int64) (store.HHPhoto, error) {
	p := store.HHPhoto{CompanyID: companyID, EntryID: entryID}
	err := s.db.QueryRowContext(ctx, "SELECT content_type, data FROM hh_photos WHERE entry_id = ? AND company_id = ?", entryID, companyID).Scan(&p.ContentType, &p.Data)
	if errors.Is(err, sql.ErrNoRows) {
		return p, store.ErrNotFound
	}
	return p, err
}

func (s *Store) ListHHObligations(ctx context.Context, companyID int64) ([]household.Obligation, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, data FROM hh_obligations WHERE company_id = ? ORDER BY id", companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []household.Obligation{}
	for rows.Next() {
		var o household.Obligation
		var id int64
		var data string
		if err := rows.Scan(&id, &data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &o); err != nil {
			return nil, err
		}
		o.ID, o.CompanyID = id, companyID
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) SaveHHObligation(ctx context.Context, companyID int64, o *household.Obligation) error {
	o.CompanyID = companyID
	data, err := json.Marshal(o)
	if err != nil {
		return err
	}
	if o.ID == 0 {
		res, err := s.db.ExecContext(ctx, "INSERT INTO hh_obligations (company_id, data) VALUES (?, ?)", companyID, string(data))
		if err != nil {
			return err
		}
		o.ID, err = res.LastInsertId()
		return err
	}
	res, err := s.db.ExecContext(ctx, "UPDATE hh_obligations SET data = ? WHERE id = ? AND company_id = ?", string(data), o.ID, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteHHObligation(ctx context.Context, companyID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM hh_obligations WHERE id = ? AND company_id = ?", id, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}
