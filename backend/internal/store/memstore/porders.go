package memstore

import (
	"context"
	"sort"

	"github.com/fiskus-online/fiskus/backend/internal/porder"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// cloneOrder skopíruje zákazku vrátane výkazu práce a materiálu.
func cloneOrder(j porder.Order) porder.Order {
	j.Lines = append([]porder.Line(nil), j.Lines...)
	return j
}

// ListPurchaseOrders vráti zákazky, najnovšie (podľa čísla) prvé.
func (s *Store) ListPurchaseOrders(_ context.Context, companyID int64) ([]porder.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []porder.Order{}
	for _, j := range s.porders {
		if j.CompanyID == companyID {
			out = append(out, cloneOrder(j))
		}
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID > out[k].ID })
	return out, nil
}

func (s *Store) GetPurchaseOrder(_ context.Context, companyID, id int64) (porder.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.porders[id]
	if !ok || j.CompanyID != companyID {
		return porder.Order{}, store.ErrNotFound
	}
	return cloneOrder(j), nil
}

func (s *Store) SavePurchaseOrder(_ context.Context, companyID int64, j *porder.Order) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.porders == nil {
		s.porders = map[int64]porder.Order{}
	}
	if j.ID != 0 {
		if old, ok := s.porders[j.ID]; !ok || old.CompanyID != companyID {
			return store.ErrNotFound
		}
	}
	for _, x := range s.porders {
		if x.CompanyID == companyID && x.ID != j.ID && j.Number != "" && x.Number == j.Number {
			return store.ErrDuplicate
		}
	}
	if j.ID == 0 {
		j.ID = s.id()
	}
	j.CompanyID = companyID
	s.porders[j.ID] = cloneOrder(*j)
	return nil
}

func (s *Store) DeletePurchaseOrder(_ context.Context, companyID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.porders[id]
	if !ok || j.CompanyID != companyID {
		return store.ErrNotFound
	}
	delete(s.porders, id)
	return nil
}
