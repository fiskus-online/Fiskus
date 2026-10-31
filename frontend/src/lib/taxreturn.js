// Daňové priznanie FO typ B: riadky na zobrazenie (výpočet robí server).

/** Riadky priznania v poradí, ako idú vo formulári; sum = medzisúčet. */
export const taxRows = [
  { code: '39', label: 'Príjmy zo živnosti (tab. 1, r. 2 a 10, stĺ. 1)' },
  { code: '40', label: 'Výdavky (tab. 1, r. 2 a 10, stĺ. 2)' },
  { code: '41', label: 'Základ dane z podnikania', sum: true },
  { code: '42', label: 'Daňová strata' },
  { code: '38', label: 'Základ dane zo zamestnania (r. 36 − r. 37)', optional: true },
  { code: '72', label: 'Základ dane na výpočet nezdaniteľnej časti' },
  { code: '73', label: 'Nezdaniteľná časť na daňovníka' },
  { code: '75', label: 'Príspevky na doplnkové dôchodkové sporenie', optional: true },
  { code: '77', label: 'Nezdaniteľné časti spolu' },
  { code: '81', label: 'Daň zo zamestnania', optional: true },
  { code: '92', label: 'Základ dane z podnikania po nezdaniteľných častiach', sum: true },
  { code: '96', label: 'Daň z podnikania' },
  { code: '116', label: 'Daň celkom', sum: true },
  { code: '131', label: 'Preddavky zrazené zamestnávateľom', optional: true },
  { code: '133', label: 'Zaplatené preddavky na daň' },
  { code: '135', label: 'Daň na úhradu', total: true },
  { code: '136', label: 'Daňový preplatok', total: true },
  { code: '152', label: 'Podiel dane pre prijímateľa (2 % / 3 %)', optional: true },
]

/** Riadky na zobrazenie: voliteľné sa skryjú, ak sú nulové. */
export function visibleRows(rows = {}) {
  const val = (c) => Number(rows[c] ?? 0)
  return taxRows.filter((r) => {
    if (r.code === '42') return val('42') !== 0
    if (r.code === '41') return val('42') === 0
    if (r.code === '135') return val('136') === 0
    if (r.code === '136') return val('136') !== 0
    return !r.optional || val(r.code) !== 0
  })
}

/** Sadzba dane z podnikania podľa príjmov (text pri r. 96). */
export const businessRate = (rows = {}) => (Number(rows['95'] ?? 0) <= 100000 ? '15 % (príjmy do 100 000 €)' : 'progresívne 19 – 35 % (príjmy nad 100 000 €)')
