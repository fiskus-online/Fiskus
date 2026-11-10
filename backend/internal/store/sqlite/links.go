//go:build sqlite

package sqlite

import (
	"context"
	"time"

	"github.com/fiskus-online/fiskus/backend/internal/link"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func (s *Store) ListDocLinks(ctx context.Context, companyID int64, r link.Ref) ([]link.Link, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, a_kind, a_id, b_kind, b_id, created FROM doc_links
		WHERE company_id = ? AND ((a_kind = ? AND a_id = ?) OR (b_kind = ? AND b_id = ?)) ORDER BY id`,
		companyID, string(r.Kind), r.ID, string(r.Kind), r.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []link.Link{}
	for rows.Next() {
		var l link.Link
		var ak, bk, created string
		if err := rows.Scan(&l.ID, &ak, &l.A.ID, &bk, &l.B.ID, &created); err != nil {
			return nil, err
		}
		l.CompanyID, l.A.Kind, l.B.Kind = companyID, link.Kind(ak), link.Kind(bk)
		l.Created, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) SaveDocLink(ctx context.Context, companyID int64, l *link.Link) error {
	if l.Created.IsZero() {
		l.Created = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO doc_links (company_id, a_kind, a_id, b_kind, b_id, created) VALUES (?, ?, ?, ?, ?, ?)`,
		companyID, string(l.A.Kind), l.A.ID, string(l.B.Kind), l.B.ID, l.Created.UTC().Format(time.RFC3339Nano))
	if isUnique(err) {
		return store.ErrDuplicate
	}
	if err != nil {
		return err
	}
	l.CompanyID = companyID
	l.ID, err = res.LastInsertId()
	return err
}

func (s *Store) DeleteDocLink(ctx context.Context, companyID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM doc_links WHERE id = ? AND company_id = ?", id, companyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}
