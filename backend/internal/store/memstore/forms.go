package memstore

import (
	"context"
	"sort"

	"github.com/fiskus-online/fiskus/backend/internal/forms"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func (s *Store) ListFormDocs(_ context.Context, companyID int64) ([]forms.Doc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []forms.Doc{}
	for _, d := range s.formDocs {
		if d.CompanyID == companyID {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) GetFormDoc(_ context.Context, companyID, id int64) (forms.Doc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.formDocs[id]
	if !ok || d.CompanyID != companyID {
		return forms.Doc{}, store.ErrNotFound
	}
	return d, nil
}

func (s *Store) SaveFormDoc(_ context.Context, companyID int64, d *forms.Doc) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.formDocs == nil {
		s.formDocs = map[int64]forms.Doc{}
	}
	if d.ID == 0 {
		d.ID = s.id()
	} else if old, ok := s.formDocs[d.ID]; !ok || old.CompanyID != companyID {
		return store.ErrNotFound
	}
	d.CompanyID = companyID
	s.formDocs[d.ID] = *d
	return nil
}

func (s *Store) DeleteFormDoc(_ context.Context, companyID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.formDocs[id]
	if !ok || d.CompanyID != companyID {
		return store.ErrNotFound
	}
	delete(s.formDocs, id)
	return nil
}
