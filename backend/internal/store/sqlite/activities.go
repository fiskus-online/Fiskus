//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/fiskus-online/fiskus/backend/internal/activity"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func (s *Store) ListActivities(ctx context.Context, companyID int64) ([]activity.Activity, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, data FROM activities WHERE company_id = ? ORDER BY day DESC, id DESC", companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []activity.Activity{}
	for rows.Next() {
		var j activity.Activity
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

func (s *Store) GetActivity(ctx context.Context, companyID, id int64) (activity.Activity, error) {
	var j activity.Activity
	var data string
	err := s.db.QueryRowContext(ctx, "SELECT data FROM activities WHERE id = ? AND company_id = ?", id, companyID).Scan(&data)
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

func (s *Store) SaveActivity(ctx context.Context, companyID int64, j *activity.Activity) error {
	j.CompanyID = companyID
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	if j.ID == 0 {
		res, err := s.db.ExecContext(ctx, "INSERT INTO activities (company_id, day, data) VALUES (?, ?, ?)", companyID, j.Date.String(), string(data))
		if isUnique(err) {
			return store.ErrDuplicate
		}
		if err != nil {
			return err
		}
		j.ID, err = res.LastInsertId()
		return err
	}
	res, err := s.db.ExecContext(ctx, "UPDATE activities SET day = ?, data = ? WHERE id = ? AND company_id = ?", j.Date.String(), string(data), j.ID, companyID)
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

func (s *Store) DeleteActivity(ctx context.Context, companyID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM activities WHERE id = ? AND company_id = ?", id, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}
