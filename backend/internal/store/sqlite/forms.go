//go:build sqlite

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/fiskus-online/fiskus/backend/internal/forms"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func (s *Store) ListFormDocs(ctx context.Context, companyID int64) ([]forms.Doc, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, data FROM form_docs WHERE company_id = ? ORDER BY id", companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []forms.Doc{}
	for rows.Next() {
		var d forms.Doc
		var id int64
		var data string
		if err := rows.Scan(&id, &data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &d); err != nil {
			return nil, err
		}
		d.ID, d.CompanyID = id, companyID
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) GetFormDoc(ctx context.Context, companyID, id int64) (forms.Doc, error) {
	var d forms.Doc
	var data string
	err := s.db.QueryRowContext(ctx, "SELECT data FROM form_docs WHERE id = ? AND company_id = ?", id, companyID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return d, store.ErrNotFound
	}
	if err != nil {
		return d, err
	}
	if err := json.Unmarshal([]byte(data), &d); err != nil {
		return d, err
	}
	d.ID, d.CompanyID = id, companyID
	return d, nil
}

func (s *Store) SaveFormDoc(ctx context.Context, companyID int64, d *forms.Doc) error {
	d.CompanyID = companyID
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	if d.ID == 0 {
		res, err := s.db.ExecContext(ctx, "INSERT INTO form_docs (company_id, data) VALUES (?, ?)", companyID, string(data))
		if err != nil {
			return err
		}
		d.ID, err = res.LastInsertId()
		return err
	}
	res, err := s.db.ExecContext(ctx, "UPDATE form_docs SET data = ? WHERE id = ? AND company_id = ?", string(data), d.ID, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteFormDoc(ctx context.Context, companyID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM form_docs WHERE id = ? AND company_id = ?", id, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}
