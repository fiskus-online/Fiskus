package api

import (
	"github.com/fiskus-online/fiskus/backend/internal/fx"
	"github.com/fiskus-online/fiskus/backend/internal/journal"
	"github.com/fiskus-online/fiskus/backend/internal/purchase"
	"net/http"

	"github.com/fiskus-online/fiskus/backend/internal/app"
	"github.com/fiskus-online/fiskus/backend/internal/audit"
	"github.com/fiskus-online/fiskus/backend/internal/auth"
	"github.com/fiskus-online/fiskus/backend/internal/invoice"
	"github.com/fiskus-online/fiskus/backend/internal/store"
	"github.com/fiskus-online/fiskus/backend/internal/vat"
	"github.com/fiskus-online/fiskus/backend/internal/vatreturn"
)

// codes vráti systémové číselníky (rovnaké pre všetky firmy).
func (s *Server) codes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"vatPeriods":      vat.Periods(),
		"paymentMethods":  invoice.PaymentMethods,
		"defaultUnits":    store.DefaultUnits,
		"defaultFormats":  defaultFormats(),
		"roles":           auth.Roles,
		"auditCategories": audit.Categories,
		"vatRows":         vatreturn.Rows,
		"currencies":      fx.Currencies,
		"domesticRCs":     invoice.DomesticRCs,
		"expenseKinds":    purchase.ExpenseKinds,
		"journalColumns":  journal.Columns,
	})
}

func defaultFormats() map[string]string {
	out := map[string]string{}
	for k, v := range app.ExtraFormats {
		out[k] = v
	}
	for k, v := range invoice.DefaultFormats {
		out[string(k)] = v
	}
	return out
}
