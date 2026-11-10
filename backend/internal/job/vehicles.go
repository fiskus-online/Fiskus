package job

import (
	"sort"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
)

// VehicleCard je súhrn vozidla zo zákaziek autoservisu (rovnaké EČV, bez
// EČV rovnaké VIN): posledné údaje, návštevy a termíny.
type VehicleCard struct {
	Key         string       `json:"key"` // EČV, bez neho VIN
	Plate       string       `json:"plate"`
	VIN         string       `json:"vin"`
	Make        string       `json:"make"`
	Model       string       `json:"model"`
	Year        int          `json:"year"`
	Mileage     int          `json:"mileage"` // najvyšší zapísaný stav tachometra
	ContactID   int64        `json:"contactId"`
	Customer    string       `json:"customer"`
	Last        invoice.Date `json:"last"` // posledná návšteva (začiatok alebo založenie zákazky)
	LastJobID   int64        `json:"lastJobId"`
	Visits      int          `json:"visits"`
	JobIDs      []int64      `json:"jobIds"` // najnovšie prvé
	Inspection  invoice.Date `json:"inspection"`
	NextService invoice.Date `json:"nextService"`
}

// VehicleKey je kľúč vozidla: EČV, bez neho VIN.
func VehicleKey(v *Vehicle) string {
	if v == nil {
		return ""
	}
	if v.Plate != "" {
		return v.Plate
	}
	return v.VIN
}

// visit je dátum návštevy zákazky.
func (j *Job) visit() invoice.Date {
	if !j.Start.IsZero() {
		return j.Start
	}
	return j.Created
}

// Vehicles zostaví karty vozidiel zo zákaziek (najnovšia návšteva prvá).
// Údaje vozidla a termíny berie z najnovšej zákazky, kde sú vyplnené;
// zákazky stratené sa nerátajú ako návšteva, ale údaje z nich áno.
func Vehicles(jobs []Job) []VehicleCard {
	list := append([]Job(nil), jobs...)
	sort.SliceStable(list, func(i, k int) bool {
		a, b := list[i].visit(), list[k].visit()
		if a != b {
			return b.Before(a)
		}
		return list[i].ID > list[k].ID
	})
	idx := map[string]int{}
	var out []VehicleCard
	for _, j := range list {
		key := VehicleKey(j.Vehicle)
		if key == "" {
			continue
		}
		v := j.Vehicle
		i, ok := idx[key]
		if !ok {
			idx[key] = len(out)
			i = len(out)
			out = append(out, VehicleCard{Key: key, Plate: v.Plate, LastJobID: j.ID, Last: j.visit(), ContactID: j.ContactID, Customer: j.Customer})
		}
		c := &out[i]
		fill := func(dst *string, src string) {
			if *dst == "" {
				*dst = src
			}
		}
		fill(&c.VIN, v.VIN)
		fill(&c.Make, v.Make)
		fill(&c.Model, v.Model)
		fill(&c.Customer, j.Customer)
		if c.ContactID == 0 {
			c.ContactID = j.ContactID
		}
		if c.Year == 0 {
			c.Year = v.Year
		}
		if v.Mileage > c.Mileage {
			c.Mileage = v.Mileage
		}
		if c.Inspection.IsZero() {
			c.Inspection = v.Inspection
		}
		if c.NextService.IsZero() {
			c.NextService = v.NextService
		}
		c.JobIDs = append(c.JobIDs, j.ID)
		if j.Status != Lost {
			c.Visits++
		}
	}
	if out == nil {
		out = []VehicleCard{}
	}
	return out
}
