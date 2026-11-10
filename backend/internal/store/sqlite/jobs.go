//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/fiskus-online/fiskus/backend/internal/job"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func (s *Store) ListJobs(ctx context.Context, companyID int64) ([]job.Job, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, data FROM jobs WHERE company_id = ? ORDER BY id DESC", companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []job.Job{}
	for rows.Next() {
		var j job.Job
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

func (s *Store) GetJob(ctx context.Context, companyID, id int64) (job.Job, error) {
	var j job.Job
	var data string
	err := s.db.QueryRowContext(ctx, "SELECT data FROM jobs WHERE id = ? AND company_id = ?", id, companyID).Scan(&data)
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

func (s *Store) SaveJob(ctx context.Context, companyID int64, j *job.Job) error {
	j.CompanyID = companyID
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	if j.ID == 0 {
		res, err := s.db.ExecContext(ctx, "INSERT INTO jobs (company_id, number, data) VALUES (?, ?, ?)", companyID, j.Number, string(data))
		if isUnique(err) {
			return store.ErrDuplicate
		}
		if err != nil {
			return err
		}
		j.ID, err = res.LastInsertId()
		return err
	}
	res, err := s.db.ExecContext(ctx, "UPDATE jobs SET number = ?, data = ? WHERE id = ? AND company_id = ?", j.Number, string(data), j.ID, companyID)
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

func (s *Store) DeleteJob(ctx context.Context, companyID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM jobs WHERE id = ? AND company_id = ?", id, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}
