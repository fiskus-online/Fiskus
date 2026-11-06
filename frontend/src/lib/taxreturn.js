// Daňové priznanie FO typ B: riadky na zobrazenie (výpočet robí server).

/** Riadky priznania v poradí, ako idú vo formulári; sum = medzisúčet. */
export const taxRows = [
  { code: '39', label: 'Príjmy zo živnosti (tab. 1, r. 2 a 10, stĺ. 1)' },
  { code: '40', label: 'Výdavky (tab. 1, r. 2 a 10, stĺ. 2)' },
  { code: '41', label: 'Základ dane z podnikania', sum: true },
  { code: '42', label: 'Daňová strata' },
  { code: '51', label: 'Odpočet straty', back: 5, optional: true },
  { code: '52', label: 'Odpočet straty', back: 4, optional: true },
  { code: '53', label: 'Odpočet straty', back: 3, optional: true },
  { code: '54', label: 'Odpočet straty', back: 2, optional: true },
  { code: '54a', label: 'Odpočet straty', back: 1, optional: true },
  { code: '55', label: 'Základ dane po odpočte strát', sum: true, optional: true, onlyWithLoss: true },
  { code: '38', label: 'Základ dane zo zamestnania (r. 36 − r. 37)', optional: true },
  { code: '72', label: 'Základ dane na výpočet nezdaniteľnej časti' },
  { code: '73', label: 'Nezdaniteľná časť na daňovníka' },
  { code: '74', label: 'Nezdaniteľná časť na manžela/ku', optional: true },
  { code: '75', label: 'Príspevky na doplnkové dôchodkové sporenie', optional: true },
  { code: '77', label: 'Nezdaniteľné časti spolu' },
  { code: '81', label: 'Daň zo zamestnania', optional: true },
  { code: '92', label: 'Základ dane z podnikania po nezdaniteľných častiach', sum: true },
  { code: '96', label: 'Daň z podnikania' },
  { code: '116', label: 'Daň celkom', sum: true },
  { code: '117', label: 'Daňový bonus na deti', optional: true },
  { code: '119', label: 'Bonus vyplatený zamestnávateľom', optional: true },
  { code: '118', label: 'Daň po daňovom bonuse', sum: true, optional: true, onlyWithBonus: true },
  { code: '131', label: 'Preddavky zrazené zamestnávateľom', optional: true },
  { code: '133', label: 'Zaplatené preddavky na daň' },
  { code: '135', label: 'Daň na úhradu', total: true },
  { code: '136', label: 'Daňový preplatok', total: true },
  { code: '121', label: 'Daňový bonus, ktorý vám vyplatí finančná správa', total: true, optional: true },
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
    if (r.onlyWithLoss) return ['51', '52', '53', '54', '54a'].some((c) => val(c) !== 0)
    if (r.onlyWithBonus) return val('117') !== 0
    return !r.optional || val(r.code) !== 0
  })
}

/** Sadzba dane z podnikania podľa príjmov (text pri r. 96). */
export const businessRate = (rows = {}) => (Number(rows['95'] ?? 0) <= 100000 ? '15 % (príjmy do 100 000 €)' : 'progresívne 19 – 35 % (príjmy nad 100 000 €)')

/** Mesiace na výber pri dieťati. */
export const monthLabels = ['I', 'II', 'III', 'IV', 'V', 'VI', 'VII', 'VIII', 'IX', 'X', 'XI', 'XII']

/** Celý rok = žiadny mesiac nevybraný. */
export const allYear = (months = []) => !months.some(Boolean)

/** Nové dieťa do formulára. */
export const newChild = () => ({ name: '', rc: '', months: Array(12).fill(false) })

/** Straty za 5 rokov dozadu (najstarší prvý) zo zadaných strát. */
export function lossRows(year, losses = []) {
  return [5, 4, 3, 2, 1].map((back) => {
    const y = year - back
    const l = losses.find((x) => x.year === y)
    return { year: y, amount: l ? Number(l.amount) || null : null, used: l ? Number(l.used) || null : null }
  })
}

/** Straty z formulára pre API (iba vyplnené). */
export const lossesOut = (rows) =>
  rows.filter((r) => r.amount || r.used).map((r) => ({ year: r.year, amount: String(r.amount ?? 0), used: String(r.used ?? 0) }))

/** Rodné číslo: kontrola tvaru a dátumu (server overí aj kontrolný súčet). */
export function rcHint(rc) {
  const d = String(rc ?? '').replace(/[\s/]/g, '')
  if (!d) return ''
  if (!/^\d{9,10}$/.test(d)) return 'Rodné číslo má 9 alebo 10 číslic'
  let mm = Number(d.slice(2, 4))
  if (mm > 70) mm -= 70
  else if (mm > 50) mm -= 50
  else if (mm > 20) mm -= 20
  if (mm < 1 || mm > 12) return 'Rodné číslo nemá platný mesiac'
  if (d.length === 10 && BigInt(d) % 11n !== 0n && !(BigInt(d.slice(0, 9)) % 11n === 10n && d[9] === '0')) return 'Rodné číslo nemá správny kontrolný súčet'
  return ''
}
