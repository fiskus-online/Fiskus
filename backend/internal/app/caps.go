package app

import (
	"strconv"
	"time"

	"github.com/fiskus-online/fiskus/backend/internal/edition"
)

// Capabilities vráti schopnosti režimu behu (bez licencií).
func (a *App) Capabilities() edition.Capabilities { return a.Caps }

func (a *App) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

func itoa(n int) string { return strconv.Itoa(n) }
