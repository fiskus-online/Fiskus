package memstore

import (
	"context"
	"sort"

	"github.com/fiskus-online/fiskus/backend/internal/delivery"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// cloneDelivery skopíruje zákazku vrátane výkazu práce a materiálu.
func cloneDelivery(j delivery.Note) delivery.Note {
	j.Lines = append([]invoice.Line(nil), j.Lines...)
	return j
}

// ListDeliveries vráti zákazky, najnovšie (podľa čísla) prvé.
func (s *Store) ListDeliveries(_ context.Context, companyID int64) ([]delivery.Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []delivery.Note{}
	for _, j := range s.deliveries {
		if j.CompanyID == companyID {
			out = append(out, cloneDelivery(j))
		}
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID > out[k].ID })
	return out, nil
}

func (s *Store) GetDelivery(_ context.Context, companyID, id int64) (delivery.Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.deliveries[id]
	if !ok || j.CompanyID != companyID {
		return delivery.Note{}, store.ErrNotFound
	}
	return cloneDelivery(j), nil
}

func (s *Store) SaveDelivery(_ context.Context, companyID int64, j *delivery.Note) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deliveries == nil {
		s.deliveries = map[int64]delivery.Note{}
	}
	if j.ID != 0 {
		if old, ok := s.deliveries[j.ID]; !ok || old.CompanyID != companyID {
			return store.ErrNotFound
		}
	}
	for _, x := range s.deliveries {
		if x.CompanyID == companyID && x.ID != j.ID && j.Number != "" && x.Number == j.Number {
			return store.ErrDuplicate
		}
	}
	if j.ID == 0 {
		j.ID = s.id()
	}
	j.CompanyID = companyID
	s.deliveries[j.ID] = cloneDelivery(*j)
	return nil
}

func (s *Store) DeleteDelivery(_ context.Context, companyID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.deliveries[id]
	if !ok || j.CompanyID != companyID {
		return store.ErrNotFound
	}
	delete(s.deliveries, id)
	return nil
}
