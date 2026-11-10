package memstore

import (
	"context"
	"sort"

	"github.com/fiskus-online/fiskus/backend/internal/job"
	"github.com/fiskus-online/fiskus/backend/internal/store"
)

// cloneJob skopíruje zákazku vrátane výkazu práce a materiálu.
func cloneJob(j job.Job) job.Job {
	j.Work = append([]job.WorkEntry(nil), j.Work...)
	j.Material = append([]job.MaterialEntry(nil), j.Material...)
	if j.Vehicle != nil {
		v := *j.Vehicle
		j.Vehicle = &v
	}
	if j.Site != nil {
		s := *j.Site
		j.Site = &s
	}
	if j.Device != nil {
		d := *j.Device
		j.Device = &d
	}
	return j
}

// ListJobs vráti zákazky, najnovšie (podľa čísla) prvé.
func (s *Store) ListJobs(_ context.Context, companyID int64) ([]job.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []job.Job{}
	for _, j := range s.jobs {
		if j.CompanyID == companyID {
			out = append(out, cloneJob(j))
		}
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID > out[k].ID })
	return out, nil
}

func (s *Store) GetJob(_ context.Context, companyID, id int64) (job.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok || j.CompanyID != companyID {
		return job.Job{}, store.ErrNotFound
	}
	return cloneJob(j), nil
}

func (s *Store) SaveJob(_ context.Context, companyID int64, j *job.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jobs == nil {
		s.jobs = map[int64]job.Job{}
	}
	if j.ID != 0 {
		if old, ok := s.jobs[j.ID]; !ok || old.CompanyID != companyID {
			return store.ErrNotFound
		}
	}
	for _, x := range s.jobs {
		if x.CompanyID == companyID && x.ID != j.ID && j.Number != "" && x.Number == j.Number {
			return store.ErrDuplicate
		}
	}
	if j.ID == 0 {
		j.ID = s.id()
	}
	j.CompanyID = companyID
	s.jobs[j.ID] = cloneJob(*j)
	return nil
}

func (s *Store) DeleteJob(_ context.Context, companyID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok || j.CompanyID != companyID {
		return store.ErrNotFound
	}
	delete(s.jobs, id)
	return nil
}
