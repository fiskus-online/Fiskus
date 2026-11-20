package logbook

import (
	"fmt"

	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/money"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
)

// Úprava odpočítanej dane pri osobnom vozidle kúpenom s 50 % odpočtom
// (§ 85n ods. 10 zákona o DPH): § 54d sa nepoužije, iba ak platiteľ
// v období na úpravu (5 kalendárnych rokov od prvotného použitia, § 52a
// ods. 2 písm. a)
//   - vozidlo dodá (predá) — vozidlo sa považuje za používané iba na
//     podnikanie do konca obdobia (§ 54d ods. 4), úprava v roku predaja
//     za tento a zostávajúce roky;
//   - začne ho od nasledujúceho roka používať výlučne na podnikanie
//     (oznámenie do 31. 12.) — úprava za každý taký rok zvlášť (§ 54d ods. 3).
// Výpočet podľa prílohy č. 1: DD = DV × (A − B) × R / 5, kde A = 0,5
// (odpočítaná časť pri prvotnom použití), B = 1 (výlučne podnikanie)
// a R = počet zostávajúcich rokov vrátane roka predaja, inak 1. Záporné DD
// je dodatočne odpočítateľná daň; uvádza sa v priznaní za posledné
// zdaňovacie obdobie roka v riadku odpočítania dane (r. 18, 18a, 19).

// AdjustPeriod je obdobie na úpravu odpočítanej dane pri hnuteľnom
// investičnom majetku v rokoch.
const AdjustPeriod = 5

// Acquisition sú údaje o nadobudnutí vozidla pre úpravu odpočtu.
type Acquisition struct {
	FirstUse invoice.Date // prvotné použitie (zaradenie do užívania)
	VAT      money.Dec    // DV: daň z obstarania vozidla
	Rate     vat.Rate     // sadzba dane z obstarania
	Sold     invoice.Date // predaj (dodanie) vozidla; prázdne = nepredané
}

// VATAdjustment je úprava odpočítanej dane za rok (Tax kladná = dodatočne
// odpočítateľná daň).
type VATAdjustment struct {
	VehicleID int64     `json:"vehicleId"`
	Vehicle   string    `json:"vehicle"`
	Year      int       `json:"year"`
	Rate      vat.Rate  `json:"rate"`
	Tax       money.Dec `json:"tax"`
	Years     int       `json:"years"` // R
	Reason    string    `json:"reason"`
}

// VATAdjustments vráti úpravy odpočítanej dane vozidla po rokoch.
func (v Vehicle) VATAdjustments(a Acquisition) []VATAdjustment {
	if a.FirstUse.IsZero() || a.VAT.Sign() <= 0 || v.VATPercent(a.FirstUse) != 50 {
		return nil // pri nadobudnutí nebol 50 % odpočet
	}
	start := a.FirstUse.Year()
	end := start + AdjustPeriod - 1
	sold := 0
	if !a.Sold.IsZero() && a.Sold.Year() <= end {
		sold = a.Sold.Year()
	}
	// DD = DV × (A − B) × R / 5 s A = 0,5 a B = 1 → dodatočný odpočet DV × R / 10
	part := func(r int) money.Dec {
		return a.VAT.MulRatio(money.FromInt(int64(r)), money.FromInt(2*AdjustPeriod)).RoundCents()
	}
	label := v.Name
	if v.Plate != "" {
		label += " (" + v.Plate + ")"
	}
	var out []VATAdjustment
	if v.FullVAT && v.FullVATFrom > start {
		for y := v.FullVATFrom; y <= end && (sold == 0 || y < sold); y++ {
			out = append(out, VATAdjustment{VehicleID: v.ID, Vehicle: label, Year: y, Rate: a.Rate, Tax: part(1), Years: 1,
				Reason: fmt.Sprintf("výlučne na podnikanie od %d (§ 85n ods. 10 písm. b, § 54d)", v.FullVATFrom)})
		}
	}
	if sold > 0 {
		r := end - sold + 1
		out = append(out, VATAdjustment{VehicleID: v.ID, Vehicle: label, Year: sold, Rate: a.Rate, Tax: part(r), Years: r,
			Reason: fmt.Sprintf("predaj %s v období na úpravu (§ 85n ods. 10 písm. a, § 54d ods. 4)", a.Sold.Display())})
	}
	return out
}
