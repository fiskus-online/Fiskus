//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/fiskus-online/fiskus/backend/internal/sorder"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func (s *Store) ListSalesOrders(ctx context.Context, companyID int64) ([]sorder.Order, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, data FROM sales_orders WHERE company_id = ? ORDER BY id DESC", companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []sorder.Order{}
	for rows.Next() {
		var j sorder.Order
		var id int64
		var data string
		if err := rows.Scan(&id, &data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &j); err != nil {
			return nil, err
		}
		j.ID, j.CompanyID = id, companyID
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *Store) GetSalesOrder(ctx context.Context, companyID, id int64) (sorder.Order, error) {
	var j sorder.Order
	var data string
	err := s.db.QueryRowContext(ctx, "SELECT data FROM sales_orders WHERE id = ? AND company_id = ?", id, companyID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return j, store.ErrNotFound
	}
	if err != nil {
		return j, err
	}
	if err := json.Unmarshal([]byte(data), &j); err != nil {
		return j, err
	}
	j.ID, j.CompanyID = id, companyID
	return j, nil
}

func (s *Store) SaveSalesOrder(ctx context.Context, companyID int64, j *sorder.Order) error {
	j.CompanyID = companyID
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	if j.ID == 0 {
		res, err := s.db.ExecContext(ctx, "INSERT INTO sales_orders (company_id, number, data) VALUES (?, ?, ?)", companyID, j.Number, string(data))
		if isUnique(err) {
			return store.ErrDuplicate
		}
		if err != nil {
			return err
		}
		j.ID, err = res.LastInsertId()
		return err
	}
	res, err := s.db.ExecContext(ctx, "UPDATE sales_orders SET number = ?, data = ? WHERE id = ? AND company_id = ?", j.Number, string(data), j.ID, companyID)
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

func (s *Store) DeleteSalesOrder(ctx context.Context, companyID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM sales_orders WHERE id = ? AND company_id = ?", id, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}
