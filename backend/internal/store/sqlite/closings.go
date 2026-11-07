//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/fiskus-online/fiskus/backend/internal/cash"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func (s *Store) ListCashClosings(ctx context.Context, companyID int64, year int) ([]cash.Closing, error) {
	q := "SELECT id, data FROM cash_closings WHERE company_id = ?"
	args := []any{companyID}
	if year != 0 {
		q += " AND date >= ? AND date <= ?"
		args = append(args, fmt.Sprintf("%04d-01-01", year), fmt.Sprintf("%04d-12-31", year))
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY date, id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []cash.Closing{}
	for rows.Next() {
		var c cash.Closing
		var id int64
		var data string
		if err := rows.Scan(&id, &data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &c); err != nil {
			return nil, err
		}
		c.ID, c.CompanyID = id, companyID
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetCashClosing(ctx context.Context, companyID, id int64) (cash.Closing, error) {
	var c cash.Closing
	var data string
	err := s.db.QueryRowContext(ctx, "SELECT data FROM cash_closings WHERE id = ? AND company_id = ?", id, companyID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return c, store.ErrNotFound
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal([]byte(data), &c); err != nil {
		return c, err
	}
	c.ID, c.CompanyID = id, companyID
	return c, nil
}

func (s *Store) SaveCashClosing(ctx context.Context, companyID int64, c *cash.Closing) error {
	c.CompanyID = companyID
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if c.ID == 0 {
		res, err := s.db.ExecContext(ctx, "INSERT INTO cash_closings (company_id, date, data) VALUES (?, ?, ?)", companyID, c.Date.String(), string(data))
		if err != nil {
			return err
		}
		c.ID, err = res.LastInsertId()
		return err
	}
	res, err := s.db.ExecContext(ctx, "UPDATE cash_closings SET date = ?, data = ? WHERE id = ? AND company_id = ?", c.Date.String(), string(data), c.ID, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteCashClosing(ctx context.Context, companyID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM cash_closings WHERE id = ? AND company_id = ?", id, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}
