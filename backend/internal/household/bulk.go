package household

// BulkChange je hromadná zmena označených pohybov: nová kategória a/alebo
// účet (prázdne = nemení sa).
type BulkChange struct {
	Category  string `json:"category"`
	AccountID int64  `json:"accountId"`
}

// BulkApply použije hromadnú zmenu na pohyb. Vráti upravený pohyb, alebo
// dôvod, prečo sa pohyb nezmení (prázdny dôvod = zmena sa použije).
// Prevod kategóriu nemá a rozdelený pohyb (bloček s položkami) si
// kategórie položiek ponechá; kategória musí byť toho istého druhu
// (príjem/výdavok) ako pohyb.
func BulkApply(e Entry, ch BulkChange, cats []Category) (Entry, string) {
	if ch.Category != "" && !e.Transfer() {
		if len(e.Split) > 0 {
			return e, "rozdelený na položky — kategórie zmeňte v pohybe"
		}
		found := false
		for _, c := range cats {
			if c.Name == ch.Category {
				found = true
				if c.Income != (e.Amount.Sign() > 0) {
					if c.Income {
						return e, "výdavok nemôže mať kategóriu príjmov"
					}
					return e, "príjem nemôže mať kategóriu výdavkov"
				}
			}
		}
		if !found {
			return e, "kategória „" + ch.Category + "“ nie je v zozname"
		}
		e.Category = ch.Category
	} else if ch.Category != "" && ch.AccountID == 0 {
		return e, "prevod nemá kategóriu"
	}
	if ch.AccountID != 0 {
		if e.Transfer() && e.ToAccountID == ch.AccountID {
			return e, "prevod by bol na ten istý účet"
		}
		e.AccountID = ch.AccountID
	}
	return e, ""
}
