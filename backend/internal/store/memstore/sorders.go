package memstore

import (
	"context"
	"sort"

	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/sorder"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// cloneSalesOrder skopíruje objednávku vrátane riadkov a dodaní.
func cloneSalesOrder(j sorder.Order) sorder.Order {
	j.Lines = append([]sorder.Line(nil), j.Lines...)
	fs := make([]sorder.Fulfil, len(j.Fulfils))
	for i, f := range j.Fulfils {
		q := make(map[int]money.Dec, len(f.Qty))
		for k, v := range f.Qty {
			q[k] = v
		}
		f.Qty = q
		fs[i] = f
	}
	if j.Fulfils != nil {
		j.Fulfils = fs
	}
	return j
}

// ListSalesOrders vráti prijaté objednávky, najnovšie prvé.
func (s *Store) ListSalesOrders(_ context.Context, companyID int64) ([]sorder.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []sorder.Order{}
	for _, j := range s.sorders {
		if j.CompanyID == companyID {
			out = append(out, cloneSalesOrder(j))
		}
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID > out[k].ID })
	return out, nil
}

func (s *Store) GetSalesOrder(_ context.Context, companyID, id int64) (sorder.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.sorders[id]
	if !ok || j.CompanyID != companyID {
		return sorder.Order{}, store.ErrNotFound
	}
	return cloneSalesOrder(j), nil
}

func (s *Store) SaveSalesOrder(_ context.Context, companyID int64, j *sorder.Order) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sorders == nil {
		s.sorders = map[int64]sorder.Order{}
	}
	if j.ID != 0 {
		if old, ok := s.sorders[j.ID]; !ok || old.CompanyID != companyID {
			return store.ErrNotFound
		}
	}
	for _, x := range s.sorders {
		if x.CompanyID == companyID && x.ID != j.ID && j.Number != "" && x.Number == j.Number {
			return store.ErrDuplicate
		}
	}
	if j.ID == 0 {
		j.ID = s.id()
	}
	j.CompanyID = companyID
	s.sorders[j.ID] = cloneSalesOrder(*j)
	return nil
}

func (s *Store) DeleteSalesOrder(_ context.Context, companyID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.sorders[id]
	if !ok || j.CompanyID != companyID {
		return store.ErrNotFound
	}
	delete(s.sorders, id)
	return nil
}
