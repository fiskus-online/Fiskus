package memstore

import (
	"context"
	"sort"

	"github.com/fiskus-online/fiskus/backend/internal/activity"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// ListActivities vráti aktivity firmy, najnovšie prvé.
func (s *Store) ListActivities(_ context.Context, companyID int64) ([]activity.Activity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []activity.Activity{}
	for _, a := range s.activities {
		if a.CompanyID == companyID {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, k int) bool {
		if out[i].Date != out[k].Date {
			return out[k].Date.Before(out[i].Date)
		}
		return out[i].ID > out[k].ID
	})
	return out, nil
}

func (s *Store) GetActivity(_ context.Context, companyID, id int64) (activity.Activity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.activities[id]
	if !ok || a.CompanyID != companyID {
		return activity.Activity{}, store.ErrNotFound
	}
	return a, nil
}

func (s *Store) SaveActivity(_ context.Context, companyID int64, a *activity.Activity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activities == nil {
		s.activities = map[int64]activity.Activity{}
	}
	if a.ID != 0 {
		if old, ok := s.activities[a.ID]; !ok || old.CompanyID != companyID {
			return store.ErrNotFound
		}
	} else {
		a.ID = s.id()
	}
	a.CompanyID = companyID
	s.activities[a.ID] = *a
	return nil
}

func (s *Store) DeleteActivity(_ context.Context, companyID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a, ok := s.activities[id]; !ok || a.CompanyID != companyID {
		return store.ErrNotFound
	}
	delete(s.activities, id)
	return nil
}
