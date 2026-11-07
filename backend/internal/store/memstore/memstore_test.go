package memstore

import (
	"testing"

	"github.com/fiskus-online/fiskus/backend/internal/store"
	"github.com/fiskus-online/fiskus/backend/internal/store/storetest"
)

func TestContract(t *testing.T) {
	storetest.Run(t, func(*testing.T) store.Store { return New() })
}
