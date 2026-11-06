package extract

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// ReceiptItem je položka bločku od modelu.
type ReceiptItem struct {
	Name     string `json:"name"`
	Quantity string `json:"quantity"`
	Amount   string `json:"amount"`   // cena za riadok s DPH (zľava záporná)
	Category string `json:"category"` // kategória domácnosti zo zoznamu
}

// ReceiptResult je bloček prečítaný z fotky (domácnosť).
type ReceiptResult struct {
	Store string        `json:"store"`
	Date  string        `json:"date"`
	Total string        `json:"total"`
	Items []ReceiptItem `json:"items"`
}

func receiptSchema(categories []string) map[string]any {
	cat := map[string]any{"type": "string", "description": "kategória výdavku domácnosti pre položku"}
	if len(categories) > 0 {
		cat["enum"] = categories
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"store": map[string]any{"type": "string", "description": "obchod (predajca), napr. Lidl, Kaufland, Dr.Max"},
			"date":  map[string]any{"type": "string", "description": "dátum nákupu YYYY-MM-DD"},
			"total": map[string]any{"type": "string", "description": "celková suma na úhradu"},
			"items": map[string]any{
				"type":        "array",
				"description": "všetky položky nákupu v poradí na bločku; zľavy ako samostatné položky so zápornou sumou",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":     map[string]any{"type": "string", "description": "názov položky tak, ako je na bločku"},
						"quantity": map[string]any{"type": "string", "description": "množstvo (ks, kg)"},
						"amount":   map[string]any{"type": "string", "description": "suma za riadok s DPH"},
						"category": cat,
					},
					"required": []string{"name", "amount", "category"},
				},
			},
		},
		"required": []string{"store", "total", "items"},
	}
}

// ExtractReceipt prečíta z fotky bločku obchod, dátum, položky a zaradí
// každú položku do jednej z kategórií domácnosti.
func (c Client) ExtractReceipt(ctx context.Context, data []byte, categories []string) (ReceiptResult, error) {
	text := `Toto je bloček (pokladničný doklad) z nákupu domácnosti na Slovensku. Prečítaj obchod, dátum, celkovú sumu ` +
		`a všetky položky a zapíš ich nástrojom zapis_blocek. Sumy píš ako čísla s desatinnou bodkou bez meny, dátum ako YYYY-MM-DD. ` +
		`Každú položku zaraď do najvhodnejšej kategórie zo zoznamu: ` + strings.Join(categories, ", ") + `. ` +
		`Skratky z bločku chápej ako bežné tovary (napr. ROZOK → pečivo → potraviny, SAMPON → drogéria). Čo na bločku nie je, nechaj prázdne.`
	raw, err := c.call(ctx, data, "zapis_blocek", "Zapíše nákup z bločku do domáceho účtovníctva.", receiptSchema(categories), text, 4096)
	if err != nil {
		return ReceiptResult{}, err
	}
	var r ReceiptResult
	if err := json.Unmarshal(raw, &r); err != nil {
		return ReceiptResult{}, fmt.Errorf("Claude API vrátilo nečitateľné údaje: %w", err)
	}
	return r, nil
}

// Num prečíta sumu od modelu („1234.50“, „1 234,50“).
func Num(s string) (money.Dec, bool) { return num(s) }
