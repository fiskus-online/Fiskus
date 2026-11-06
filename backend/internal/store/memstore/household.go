package memstore

import (
	"context"
	"sort"

	"github.com/palo-krnac/fiscus/backend/internal/household"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func (s *Store) ListHHAccounts(_ context.Context, companyID int64) ([]household.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []household.Account{}
	for _, a := range s.hhAccounts {
		if a.CompanyID == companyID {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) SaveHHAccount(_ context.Context, companyID int64, a *household.Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hhAccounts == nil {
		s.hhAccounts = map[int64]household.Account{}
	}
	if a.ID == 0 {
		a.ID = s.id()
	} else if old, ok := s.hhAccounts[a.ID]; !ok || old.CompanyID != companyID {
		return store.ErrNotFound
	}
	a.CompanyID = companyID
	s.hhAccounts[a.ID] = *a
	return nil
}

func (s *Store) DeleteHHAccount(_ context.Context, companyID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.hhAccounts[id]
	if !ok || a.CompanyID != companyID {
		return store.ErrNotFound
	}
	delete(s.hhAccounts, id)
	return nil
}

func (s *Store) ListHHEntries(_ context.Context, companyID int64, year int) ([]household.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []household.Entry{}
	for _, e := range s.hhEntries {
		if e.CompanyID == companyID && (year == 0 || e.Date.Year() == year) {
			out = append(out, e)
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

func (s *Store) GetHHEntry(_ context.Context, companyID, id int64) (household.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.hhEntries[id]
	if !ok || e.CompanyID != companyID {
		return household.Entry{}, store.ErrNotFound
	}
	return e, nil
}

func (s *Store) SaveHHEntry(_ context.Context, companyID int64, e *household.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hhEntries == nil {
		s.hhEntries = map[int64]household.Entry{}
	}
	if e.ID != 0 {
		if old, ok := s.hhEntries[e.ID]; !ok || old.CompanyID != companyID {
			return store.ErrNotFound
		}
	}
	if e.Source != "" {
		for _, o := range s.hhEntries {
			if o.CompanyID == companyID && o.ID != e.ID && o.Source == e.Source {
				return store.ErrDuplicate
			}
		}
	}
	if e.ID == 0 {
		e.ID = s.id()
	}
	e.CompanyID = companyID
	s.hhEntries[e.ID] = *e
	return nil
}

func (s *Store) DeleteHHEntry(_ context.Context, companyID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.hhEntries[id]
	if !ok || e.CompanyID != companyID {
		return store.ErrNotFound
	}
	delete(s.hhEntries, id)
	delete(s.hhPhotos, id)
	return nil
}

func (s *Store) SaveHHPhoto(_ context.Context, p store.HHPhoto) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.hhEntries[p.EntryID]
	if !ok || e.CompanyID != p.CompanyID {
		return store.ErrNotFound
	}
	if s.hhPhotos == nil {
		s.hhPhotos = map[int64]store.HHPhoto{}
	}
	p.Data = append([]byte(nil), p.Data...)
	s.hhPhotos[p.EntryID] = p
	return nil
}

func (s *Store) GetHHPhoto(_ context.Context, companyID, entryID int64) (store.HHPhoto, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.hhPhotos[entryID]
	if !ok || p.CompanyID != companyID {
		return store.HHPhoto{}, store.ErrNotFound
	}
	return p, nil
}

func (s *Store) ListHHObligations(_ context.Context, companyID int64) ([]household.Obligation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []household.Obligation{}
	for _, o := range s.hhObligations {
		if o.CompanyID == companyID {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) SaveHHObligation(_ context.Context, companyID int64, o *household.Obligation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hhObligations == nil {
		s.hhObligations = map[int64]household.Obligation{}
	}
	if o.ID == 0 {
		o.ID = s.id()
	} else if old, ok := s.hhObligations[o.ID]; !ok || old.CompanyID != companyID {
		return store.ErrNotFound
	}
	o.CompanyID = companyID
	s.hhObligations[o.ID] = *o
	return nil
}

func (s *Store) DeleteHHObligation(_ context.Context, companyID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.hhObligations[id]
	if !ok || o.CompanyID != companyID {
		return store.ErrNotFound
	}
	delete(s.hhObligations, id)
	return nil
}
