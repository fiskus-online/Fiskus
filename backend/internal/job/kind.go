package job

import (
	"strings"
	"unicode"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
)

// Kind je druh zákazky podľa odboru. Druh pridá zákazke vlastné údaje
// (vozidlo, miesto montáže, zariadenie); ekonomika, výkaz práce, materiál
// a fakturácia sú pre všetky druhy rovnaké.
type Kind string

const (
	General Kind = ""        // všeobecná
	Auto    Kind = "auto"    // autoservis: vozidlo
	Install Kind = "install" // montáž / inštalácia: miesto a montážnici
	Service Kind = "service" // servis zariadení: zariadenie a záruka
)

// Kinds sú známe druhy.
var Kinds = []Kind{General, Auto, Install, Service}

// Known: je druh platný?
func (k Kind) Known() bool {
	for _, x := range Kinds {
		if x == k {
			return true
		}
	}
	return false
}

// Vehicle je vozidlo pri zákazke autoservisu.
type Vehicle struct {
	Plate   string `json:"plate"`   // evidenčné číslo (EČV), veľkými písmenami bez medzier
	VIN     string `json:"vin"`     // identifikačné číslo vozidla
	Make    string `json:"make"`    // značka
	Model   string `json:"model"`   // model
	Year    int    `json:"year"`    // rok výroby (0 = neuvedený)
	Mileage int    `json:"mileage"` // stav tachometra pri príjme (km)
	Fuel    string `json:"fuel"`    // stav paliva pri príjme (napr. 1/2)
	Fault   string `json:"fault"`   // popis závady / požiadavka zákazníka
	Damage  string `json:"damage"`  // poškodenia zistené pri príjme
	// Inspection: STK/EK platí do; NextService: ďalší servis (oba do kalendára).
	Inspection  invoice.Date `json:"inspection"`
	NextService invoice.Date `json:"nextService"`
}

// Site je miesto a tím montáže.
type Site struct {
	Address string `json:"address"` // miesto montáže
	Workers string `json:"workers"` // montážnici
	Contact string `json:"contact"` // kontakt na mieste (meno, telefón)
	Scope   string `json:"scope"`   // rozsah prác
}

// Device je servisované zariadenie.
type Device struct {
	Type        string       `json:"type"`        // druh zariadenia (napr. práčka, frekvenčný menič)
	Make        string       `json:"make"`        // výrobca
	Model       string       `json:"model"`       // typ / model
	Serial      string       `json:"serial"`      // výrobné číslo
	Warranty    bool         `json:"warranty"`    // záručná oprava
	Purchased   invoice.Date `json:"purchased"`   // dátum kúpy (pri záruke)
	Fault       string       `json:"fault"`       // popis závady
	Accessories string       `json:"accessories"` // prevzaté príslušenstvo
}

// NormalizePlate: EČV veľkými písmenami bez medzier a pomlčiek (BB-123AB → BB123AB).
func NormalizePlate(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// validVIN: 17 znakov, písmená a číslice bez I, O, Q.
func validVIN(s string) bool {
	if len(s) != 17 {
		return false
	}
	for _, r := range s {
		switch {
		case r == 'I' || r == 'O' || r == 'Q':
			return false
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

func trim(ss ...*string) {
	for _, s := range ss {
		*s = strings.TrimSpace(*s)
	}
}

// normalizeKind oreže údaje druhu a zahodí údaje iných druhov.
func (j *Job) normalizeKind() {
	if j.Kind != Auto {
		j.Vehicle = nil
	} else if j.Vehicle == nil {
		j.Vehicle = &Vehicle{}
	}
	if j.Kind != Install {
		j.Site = nil
	} else if j.Site == nil {
		j.Site = &Site{}
	}
	if j.Kind != Service {
		j.Device = nil
	} else if j.Device == nil {
		j.Device = &Device{}
	}
	if v := j.Vehicle; v != nil {
		trim(&v.VIN, &v.Make, &v.Model, &v.Fuel, &v.Fault, &v.Damage)
		v.Plate = NormalizePlate(v.Plate)
		v.VIN = strings.ToUpper(strings.ReplaceAll(v.VIN, " ", ""))
	}
	if s := j.Site; s != nil {
		trim(&s.Address, &s.Workers, &s.Contact, &s.Scope)
	}
	if d := j.Device; d != nil {
		trim(&d.Type, &d.Make, &d.Model, &d.Serial, &d.Fault, &d.Accessories)
		if !d.Warranty {
			d.Purchased = invoice.Date{}
		}
	}
}

// validateKind overí údaje druhu.
func (j *Job) validateKind() []FieldError {
	var out []FieldError
	if !j.Kind.Known() {
		return []FieldError{{"kind", "Neznámy druh zákazky"}}
	}
	if v := j.Vehicle; v != nil {
		if v.Plate == "" && v.VIN == "" {
			out = append(out, FieldError{"vehicle.plate", "Zadajte EČV alebo VIN"})
		}
		if len(v.Plate) > 10 {
			out = append(out, FieldError{"vehicle.plate", "EČV má najviac 10 znakov"})
		}
		if v.VIN != "" && !validVIN(v.VIN) {
			out = append(out, FieldError{"vehicle.vin", "VIN má 17 znakov (písmená bez I, O, Q a číslice)"})
		}
		if v.Year != 0 && (v.Year < 1900 || v.Year > 2100) {
			out = append(out, FieldError{"vehicle.year", "Neplatný rok výroby"})
		}
		if v.Mileage < 0 {
			out = append(out, FieldError{"vehicle.mileage", "Stav tachometra nemôže byť záporný"})
		}
	}
	if d := j.Device; d != nil {
		if d.Type == "" && d.Model == "" {
			out = append(out, FieldError{"device.type", "Zadajte zariadenie alebo jeho typ"})
		}
	}
	return out
}

// Subject je krátky opis predmetu zákazky podľa druhu (do zoznamu a
// hľadania): EČV a vozidlo, zariadenie s výrobným číslom, miesto montáže.
func (j *Job) Subject() string {
	join := func(ss ...string) string {
		var out []string
		for _, s := range ss {
			if s != "" {
				out = append(out, s)
			}
		}
		return strings.Join(out, " ")
	}
	switch {
	case j.Vehicle != nil:
		return join(j.Vehicle.Plate, join(j.Vehicle.Make, j.Vehicle.Model))
	case j.Device != nil:
		s := join(j.Device.Type, j.Device.Make, j.Device.Model)
		if j.Device.Serial != "" {
			s = join(s, "v. č. "+j.Device.Serial)
		}
		return s
	case j.Site != nil:
		return j.Site.Address
	}
	return ""
}
