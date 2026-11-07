package memstore

import (
	"context"
	"sort"

	"github.com/fiskus-online/fiskus/backend/internal/cash"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

func (s *Store) ListCashClosings(_ context.Context, companyID int64, year int) ([]cash.Closing, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []cash.Closing{}
	for _, c := range s.cashClosings {
		if c.CompanyID == companyID && (year == 0 || c.Date.Year() == year) {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date.Before(out[j].Date)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *Store) GetCashClosing(_ context.Context, companyID, id int64) (cash.Closing, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.cashClosings[id]
	if !ok || c.CompanyID != companyID {
		return cash.Closing{}, store.ErrNotFound
	}
	return c, nil
}

func (s *Store) SaveCashClosing(_ context.Context, companyID int64, c *cash.Closing) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cashClosings == nil {
		s.cashClosings = map[int64]cash.Closing{}
	}
	if c.ID == 0 {
		c.ID = s.id()
	} else if old, ok := s.cashClosings[c.ID]; !ok || old.CompanyID != companyID {
		return store.ErrNotFound
	}
	c.CompanyID = companyID
	cp := *c
	cp.Rows = append([]cash.VATRow(nil), c.Rows...)
	s.cashClosings[c.ID] = cp
	return nil
}

func (s *Store) DeleteCashClosing(_ context.Context, companyID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.cashClosings[id]
	if !ok || c.CompanyID != companyID {
		return store.ErrNotFound
	}
	delete(s.cashClosings, id)
	return nil
}
