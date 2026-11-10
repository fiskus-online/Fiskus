package job

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
)

func TestValidate(t *testing.T) {
	d := invoice.MustDate
	for _, tc := range []struct {
		name string
		j    Job
		want string // pole s chybou
	}{
		{"v poriadku", Job{Name: "Rekonštrukcia RS", Start: d("2026-10-01"), Due: d("2026-10-31")}, ""},
		{"bez názvu", Job{Name: "  "}, "name"},
		{"zlý stav", Job{Name: "x", Status: "x"}, "status"},
		{"termín pred začiatkom", Job{Name: "x", Start: d("2026-10-10"), Due: d("2026-10-01")}, "due"},
		{"záporná cena", Job{Name: "x", Budget: money.MustParse("-1")}, "budget"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			j := tc.j
			j.Normalize()
			errs := j.Validate()
			got := ""
			if len(errs) > 0 {
				got = errs[0].Field
			}
			if got != tc.want || j.Status == "" {
				t.Fatalf("%+v %+v", errs, j)
			}
		})
	}
}

func TestStatus(t *testing.T) {
	today := invoice.MustDate("2026-11-10")
	j := Job{Name: "x", Status: Active, Due: invoice.MustDate("2026-11-01")}
	if !j.Late(today) {
		t.Fatal("po termíne")
	}
	j.SetStatus(Done, today)
	if j.Closed != today || j.Late(today) || j.Status.Open() {
		t.Fatalf("dokončená: %+v", j)
	}
	j.SetStatus(Done, invoice.MustDate("2026-12-01"))
	if j.Closed != today {
		t.Fatal("dátum dokončenia sa nemení")
	}
	j.SetStatus(Active, today)
	if !j.Closed.IsZero() {
		t.Fatal("znovu otvorená")
	}
}

func TestValidateEntries(t *testing.T) {
	d := invoice.MustDate
	m := money.MustParse
	ok := WorkEntry{Date: d("2026-11-02"), Description: "Montáž", Hours: m("3.5"), Rate: m("35")}
	mat := MaterialEntry{Date: d("2026-11-02"), Name: "Kábel CYKY 3x2,5", Quantity: m("25"), UnitPrice: m("1.20")}
	for _, tc := range []struct {
		name string
		mod  func(j *Job)
		want string
	}{
		{"v poriadku", func(j *Job) {}, ""},
		{"bez dátumu", func(j *Job) { j.Work[0].Date = invoice.Date{} }, "work[0].date"},
		{"bez popisu", func(j *Job) { j.Work[0].Description = " " }, "work[0].description"},
		{"nula hodín", func(j *Job) { j.Work[0].Hours = money.Zero }, "work[0].hours"},
		{"viac ako 24 h", func(j *Job) { j.Work[0].Hours = m("25") }, "work[0].hours"},
		{"záporná sadzba", func(j *Job) { j.Work[0].CostRate = m("-1") }, "work[0].rate"},
		{"materiál bez názvu", func(j *Job) { j.Material[0].Name = "" }, "material[0].name"},
		{"materiál z cenníka bez názvu", func(j *Job) { j.Material[0].Name, j.Material[0].ItemID = "", 5 }, ""},
		{"materiál bez množstva", func(j *Job) { j.Material[0].Quantity = money.Zero }, "material[0].quantity"},
		{"záporná sadzba zákazky", func(j *Job) { j.Rate = m("-5") }, "rate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			j := Job{Name: "x", Work: []WorkEntry{ok}, Material: []MaterialEntry{mat}}
			tc.mod(&j)
			j.Normalize()
			errs := j.Validate()
			got := ""
			if len(errs) > 0 {
				got = errs[0].Field
			}
			if got != tc.want {
				t.Fatalf("chyba %q, chcel som %q (%v)", got, tc.want, errs)
			}
		})
	}
	j := Job{Name: "x", Material: []MaterialEntry{{Name: "a", Date: d("2026-11-02"), Quantity: m("1")}}}
	j.Normalize()
	if j.Material[0].Unit != "ks" {
		t.Errorf("predvolená jednotka %q", j.Material[0].Unit)
	}
	if w := (WorkEntry{Hours: m("2.5"), Rate: m("33.33"), CostRate: m("10")}); w.Amount().String() != "83.33" || w.Cost().String() != "25.00" {
		t.Errorf("suma práce %s / %s", w.Amount(), w.Cost())
	}
}

func TestKeepBilled(t *testing.T) {
	d := invoice.MustDate
	m := money.MustParse
	billed := Billing{InvoiceID: 7, InvoiceNumber: "FA20260045"}
	old := Job{
		Work: []WorkEntry{
			{ID: 1, Date: d("2026-11-02"), Description: "Montáž", Hours: m("4"), Rate: m("35"), Billing: billed},
			{ID: 2, Date: d("2026-11-03"), Description: "Revízia", Hours: m("2"), Rate: m("40")},
		},
		Material: []MaterialEntry{{ID: 3, Date: d("2026-11-02"), Name: "Istič", Unit: "ks", Quantity: m("2"), UnitPrice: m("8"), Billing: billed}},
	}
	tests := []struct {
		name  string
		mod   func(j *Job)
		field string
		check func(t *testing.T, j Job)
	}{
		{"nové záznamy dostanú ID a nie sú vyfakturované", func(j *Job) {
			j.Work = append(j.Work, WorkEntry{Date: d("2026-11-04"), Description: "Oživenie", Hours: m("1"), Billing: Billing{InvoiceID: 99}})
			j.Material = append(j.Material, MaterialEntry{ID: 1, Date: d("2026-11-04"), Name: "Svorky", Quantity: m("10")})
		}, "", func(t *testing.T, j Job) {
			if j.Work[2].ID != 4 || j.Work[2].Billed() || j.Material[1].ID != 5 {
				t.Errorf("ID %+v %+v", j.Work[2], j.Material[1])
			}
		}},
		{"nevyfakturovanú prácu možno meniť", func(j *Job) { j.Work[1].Hours = m("3") }, "", func(t *testing.T, j Job) {
			if j.Work[1].Hours.String() != "3.00" {
				t.Errorf("hodiny %s", j.Work[1].Hours)
			}
		}},
		{"vyfakturovanú prácu nemožno meniť", func(j *Job) { j.Work[0].Hours = m("5") }, "work[0].description", nil},
		{"vyfakturovanú prácu nemožno zmazať", func(j *Job) { j.Work = j.Work[1:] }, "work", nil},
		{"vyfakturovaný materiál nemožno meniť", func(j *Job) { j.Material[0].Quantity = m("3") }, "material[0].name", nil},
		{"vyfakturovaný materiál nemožno zmazať", func(j *Job) { j.Material = nil }, "material", nil},
		{"klient nezruší vyfakturovanie", func(j *Job) { j.Work[0].Billing = Billing{} }, "", func(t *testing.T, j Job) {
			if j.Work[0].InvoiceNumber != "FA20260045" {
				t.Errorf("vyfakturovanie %+v", j.Work[0])
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			j := Job{Work: append([]WorkEntry(nil), old.Work...), Material: append([]MaterialEntry(nil), old.Material...)}
			tc.mod(&j)
			errs := j.KeepBilled(old)
			got := ""
			if len(errs) > 0 {
				got = errs[0].Field
			}
			if got != tc.field {
				t.Fatalf("chyba %q, chcel som %q (%v)", got, tc.field, errs)
			}
			if tc.check != nil {
				tc.check(t, j)
			}
		})
	}
	j := old
	j.Work = append([]WorkEntry(nil), old.Work...)
	j.Material = append([]MaterialEntry(nil), old.Material...)
	if !j.Release(func(id int64) bool { return id != 7 }) || j.Work[0].Billed() || j.Material[0].Billed() {
		t.Errorf("uvoľnenie %+v", j)
	}
	if j.Release(func(int64) bool { return false }) {
		t.Error("nič na uvoľnenie")
	}
}

func TestKinds(t *testing.T) {
	base := func() Job { return Job{Name: "Oprava", Status: Lead} }
	tests := []struct {
		name    string
		edit    func(*Job)
		field   string
		subject string
	}{
		{"všeobecná bez údajov", func(j *Job) { j.Vehicle = &Vehicle{Plate: "X"} }, "", ""},
		{"neznámy druh", func(j *Job) { j.Kind = "lod" }, "kind", ""},
		{"auto bez EČV aj VIN", func(j *Job) { j.Kind = Auto }, "vehicle.plate", ""},
		{"auto s EČV", func(j *Job) {
			j.Kind, j.Vehicle = Auto, &Vehicle{Plate: " bb-123 ab ", Make: "Škoda", Model: "Octavia"}
		}, "", "BB123AB Škoda Octavia"},
		{"zlé VIN", func(j *Job) { j.Kind, j.Vehicle = Auto, &Vehicle{VIN: "WVWZZZ1KZAW00000I"} }, "vehicle.vin", ""},
		{"dobré VIN s medzerou", func(j *Job) { j.Kind, j.Vehicle = Auto, &Vehicle{VIN: "wvwzzz1kzaw 000001"} }, "", ""},
		{"zlý rok", func(j *Job) { j.Kind, j.Vehicle = Auto, &Vehicle{Plate: "BB1", Year: 19} }, "vehicle.year", ""},
		{"servis bez zariadenia", func(j *Job) { j.Kind = Service }, "device.type", ""},
		{"servis", func(j *Job) {
			j.Kind, j.Device = Service, &Device{Type: "Menič", Make: "Siemens", Model: "G120", Serial: "XAJ123", Purchased: invoice.MustDate("2026-01-01")}
		}, "", "Menič Siemens G120 v. č. XAJ123"},
		{"montáž", func(j *Job) { j.Kind, j.Site = Install, &Site{Address: " Hala 2, Zvolen "} }, "", "Hala 2, Zvolen"},
		{"montáž bez miesta", func(j *Job) { j.Kind = Install }, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := base()
			tt.edit(&j)
			j.Normalize()
			errs := j.Validate()
			if tt.field == "" {
				if len(errs) != 0 {
					t.Fatalf("chyby: %+v", errs)
				}
			} else if len(errs) == 0 || errs[0].Field != tt.field {
				t.Fatalf("čakal som %s, mám %+v", tt.field, errs)
			}
			if tt.subject != "" && j.Subject() != tt.subject {
				t.Fatalf("predmet %q", j.Subject())
			}
		})
	}
	// údaje iného druhu sa zahodia, záruka bez dátumu kúpy
	j := base()
	j.Kind, j.Vehicle, j.Device = Service, &Vehicle{Plate: "BB1"}, &Device{Type: "Práčka", Purchased: invoice.MustDate("2026-01-01")}
	j.Normalize()
	if j.Vehicle != nil || j.Site != nil || !j.Device.Purchased.IsZero() {
		t.Fatalf("normalizácia %+v %+v", j.Vehicle, j.Device)
	}
}
