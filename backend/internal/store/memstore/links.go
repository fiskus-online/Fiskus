package memstore

import (
	"context"
	"sort"
	"time"

	"github.com/fiskus-online/fiskus/backend/internal/link"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func (s *Store) ListDocLinks(_ context.Context, companyID int64, r link.Ref) ([]link.Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []link.Link{}
	for _, l := range s.docLinks {
		if l.CompanyID == companyID && (l.A == r || l.B == r) {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) SaveDocLink(_ context.Context, companyID int64, l *link.Link) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.docLinks == nil {
		s.docLinks = map[int64]link.Link{}
	}
	for _, x := range s.docLinks {
		if x.CompanyID == companyID && x.A == l.A && x.B == l.B {
			return store.ErrDuplicate
		}
	}
	l.ID, l.CompanyID = s.id(), companyID
	if l.Created.IsZero() {
		l.Created = time.Now().UTC()
	}
	s.docLinks[l.ID] = *l
	return nil
}

func (s *Store) DeleteDocLink(_ context.Context, companyID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.docLinks[id]
	if !ok || l.CompanyID != companyID {
		return store.ErrNotFound
	}
	delete(s.docLinks, id)
	return nil
}
