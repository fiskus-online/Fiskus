package link

import (
	"errors"
	"testing"
)

func TestNew(t *testing.T) {
	inv, q := Ref{Invoice, 7}, Ref{Quote, 3}
	for _, tc := range []struct {
		name string
		a, b Ref
		want Link
		err  error
	}{
		{"poradie", q, inv, Link{A: inv, B: q}, nil},
		{"opačne rovnaké", inv, q, Link{A: inv, B: q}, nil},
		{"rovnaký druh podľa id", Ref{Invoice, 9}, Ref{Invoice, 2}, Link{A: Ref{Invoice, 2}, B: Ref{Invoice, 9}}, nil},
		{"sám so sebou", inv, inv, Link{}, ErrSelf},
		{"neznámy druh", Ref{"bank", 1}, inv, Link{}, ErrUnknown},
		{"bez id", Ref{Quote, 0}, inv, Link{}, ErrUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := New(tc.a, tc.b)
			if !errors.Is(err, tc.err) || got != tc.want {
				t.Fatalf("%+v %v", got, err)
			}
			if err == nil && (got.Other(got.A) != got.B || got.Other(got.B) != got.A) {
				t.Fatal("Other")
			}
		})
	}
}
