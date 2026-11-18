package household

import "testing"

func TestBulkApply(t *testing.T) {
	cats := []Category{{Name: "Mzda", Income: true}, {Name: "Potraviny"}, {Name: "Drogéria"}}
	exp := Entry{AccountID: 1, Amount: m("-10"), Category: "Potraviny"}
	inc := Entry{AccountID: 1, Amount: m("100"), Category: "Mzda"}
	tr := Entry{AccountID: 1, ToAccountID: 2, Amount: m("50")}
	split := Entry{AccountID: 1, Amount: m("-10"), Category: "Potraviny", Split: []Split{{Category: "Potraviny", Amount: m("-6")}, {Category: "Drogéria", Amount: m("-4")}}}
	cases := []struct {
		name     string
		e        Entry
		ch       BulkChange
		reason   bool
		category string
		account  int64
	}{
		{"kategória výdavku", exp, BulkChange{Category: "Drogéria"}, false, "Drogéria", 1},
		{"kategória príjmov na výdavok", exp, BulkChange{Category: "Mzda"}, true, "", 0},
		{"kategória výdavkov na príjem", inc, BulkChange{Category: "Potraviny"}, true, "", 0},
		{"neznáma kategória", exp, BulkChange{Category: "Nič"}, true, "", 0},
		{"rozdelený pohyb", split, BulkChange{Category: "Drogéria"}, true, "", 0},
		{"rozdelený pohyb na iný účet", split, BulkChange{AccountID: 3}, false, "Potraviny", 3},
		{"prevod bez kategórie", tr, BulkChange{Category: "Drogéria"}, true, "", 0},
		{"prevod na iný účet, kategória sa ignoruje", tr, BulkChange{Category: "Drogéria", AccountID: 3}, false, "", 3},
		{"prevod na ten istý účet", tr, BulkChange{AccountID: 2}, true, "", 0},
		{"kategória aj účet", exp, BulkChange{Category: "Drogéria", AccountID: 3}, false, "Drogéria", 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, reason := BulkApply(c.e, c.ch, cats)
			if (reason != "") != c.reason {
				t.Fatalf("dôvod %q", reason)
			}
			if !c.reason && (got.Category != c.category || got.AccountID != c.account) {
				t.Fatalf("%+v", got)
			}
		})
	}
}
