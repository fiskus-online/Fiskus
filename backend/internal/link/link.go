// Package link: krížové prepojenia dokladov (faktúra, cenová ponuka,
// prijatá faktúra, tlačivo ako dodací list…). Prepojenie je neusporiadaná
// dvojica dokladov tej istej firmy; ukladá sa v ustálenom poradí, aby sa
// rovnaká dvojica nedala uložiť dvakrát.
package link

import (
	"errors"
	"time"
)

// Kind je druh dokladu.
type Kind string

const (
	Invoice  Kind = "invoice"  // vydaná faktúra (aj zálohová, dobropis)
	Quote    Kind = "quote"    // cenová ponuka
	Purchase Kind = "purchase" // prijatá faktúra
	Form     Kind = "form"     // tlačivo (dodací list, objednávka, preberací protokol…)
	Job      Kind = "job"      // zákazka (internal/job)
	POrder   Kind = "porder"   // objednávka dodávateľovi (internal/porder)
)

// Kinds sú druhy, ktoré sa dajú prepojiť.
var Kinds = []Kind{Invoice, Quote, Purchase, Form, Job, POrder}

// Known: je druh podporovaný?
func (k Kind) Known() bool {
	for _, x := range Kinds {
		if x == k {
			return true
		}
	}
	return false
}

// Ref ukazuje na doklad.
type Ref struct {
	Kind Kind  `json:"kind"`
	ID   int64 `json:"id"`
}

// Less: ustálené poradie (druh, potom id).
func (r Ref) Less(o Ref) bool {
	if r.Kind != o.Kind {
		return r.Kind < o.Kind
	}
	return r.ID < o.ID
}

// Link je prepojenie dvoch dokladov.
type Link struct {
	ID        int64     `json:"id"`
	CompanyID int64     `json:"companyId"`
	A         Ref       `json:"a"`
	B         Ref       `json:"b"`
	Created   time.Time `json:"created"`
}

// Other vráti druhý koniec prepojenia (z pohľadu dokladu r).
func (l Link) Other(r Ref) Ref {
	if l.A == r {
		return l.B
	}
	return l.A
}

var (
	ErrSelf    = errors.New("doklad nemožno prepojiť sám so sebou")
	ErrUnknown = errors.New("neznámy druh dokladu")
)

// New vytvorí prepojenie v ustálenom poradí.
func New(a, b Ref) (Link, error) {
	if !a.Kind.Known() || !b.Kind.Known() || a.ID <= 0 || b.ID <= 0 {
		return Link{}, ErrUnknown
	}
	if a == b {
		return Link{}, ErrSelf
	}
	if b.Less(a) {
		a, b = b, a
	}
	return Link{A: a, B: b}, nil
}
