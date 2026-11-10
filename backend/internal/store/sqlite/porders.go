//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/fiskus-online/fiskus/backend/internal/porder"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func (s *Store) ListPurchaseOrders(ctx context.Context, companyID int64) ([]porder.Order, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, data FROM purchase_orders WHERE company_id = ? ORDER BY id DESC", companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []porder.Order{}
	for rows.Next() {
		var j porder.Order
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

func (s *Store) GetPurchaseOrder(ctx context.Context, companyID, id int64) (porder.Order, error) {
	var j porder.Order
	var data string
	err := s.db.QueryRowContext(ctx, "SELECT data FROM purchase_orders WHERE id = ? AND company_id = ?", id, companyID).Scan(&data)
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

func (s *Store) SavePurchaseOrder(ctx context.Context, companyID int64, j *porder.Order) error {
	j.CompanyID = companyID
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	if j.ID == 0 {
		res, err := s.db.ExecContext(ctx, "INSERT INTO purchase_orders (company_id, number, data) VALUES (?, ?, ?)", companyID, j.Number, string(data))
		if isUnique(err) {
			return store.ErrDuplicate
		}
		if err != nil {
			return err
		}
		j.ID, err = res.LastInsertId()
		return err
	}
	res, err := s.db.ExecContext(ctx, "UPDATE purchase_orders SET number = ?, data = ? WHERE id = ? AND company_id = ?", j.Number, string(data), j.ID, companyID)
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

func (s *Store) DeletePurchaseOrder(ctx context.Context, companyID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM purchase_orders WHERE id = ? AND company_id = ?", id, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}
