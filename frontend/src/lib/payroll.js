// Mzdy: číselníky, formulár zamestnanca a hľadanie. Všetky sumy (odvody,
// daň, čistá mzda, súčty) počíta server — tu sa iba zobrazujú.
import { matches, searchTerms } from './search.js'

export const contracts = [
  { value: 'tpp', label: 'Pracovný pomer', short: 'PP' },
  { value: 'dopc', label: 'Dohoda o pracovnej činnosti', short: 'DoPČ' },
  { value: 'dovp', label: 'Dohoda o vykonaní práce', short: 'DoVP' },
  { value: 'dobps', label: 'Dohoda o brigádnickej práci študentov', short: 'DoBPŠ' },
]
export const contractLabel = (v) => contracts.find((c) => c.value === v)?.label ?? v
export const contractShort = (v) => contracts.find((c) => c.value === v)?.short ?? v

export const insurers = [
  { value: '25', label: 'VšZP (25)' },
  { value: '24', label: 'Dôvera (24)' },
  { value: '27', label: 'Union (27)' },
]
export const insurerLabel = (v) => insurers.find((i) => i.value === v)?.label ?? v

export const pensions = [
  { value: '', label: 'Bez dôchodku' },
  { value: 'old', label: 'Starobný (aj predčasný, výsluhový po dôchodkovom veku)' },
  { value: 'disabled', label: 'Invalidný nad 70 %' },
]

export const monthNames = ['január', 'február', 'marec', 'apríl', 'máj', 'jún', 'júl', 'august', 'september', 'október', 'november', 'december']

/** „Nová Jana, Ing.“ → zobrazované meno. */
export function employeeName(e) {
  return [e?.title, e?.firstName, e?.lastName].filter(Boolean).join(' ')
}

/** Dohoda: odvodová výnimka sa dá uplatniť (študent DoBPŠ, dôchodca na dohode). */
export function oopAllowed(e) {
  return e.contract === 'dobps' || (['dopc', 'dovp'].includes(e.contract) && !!e.pension)
}

/** Pracovný vzťah trvá v mesiaci RRRR-MM. */
export function activeIn(e, ym) {
  if (e.start && e.start.slice(0, 7) > ym) return false
  return !e.end || e.end.slice(0, 7) >= ym
}

/** Stav zamestnanca k mesiacu: aktívny, ukončený, nastúpi. */
export function employeeState(e, ym) {
  if (e.start && e.start.slice(0, 7) > ym) return { tone: 'draft', label: 'nastúpi' }
  if (e.end && e.end.slice(0, 7) < ym) return { tone: 'cancelled', label: 'ukončený' }
  return { tone: 'paid', label: 'pracuje' }
}

/** Hľadanie v zamestnancoch (bez diakritiky, viac slov). */
export function filterEmployees(list, { search = '', all = true, ym = '' } = {}) {
  const terms = searchTerms(search)
  return list.filter(
    (e) =>
      (all || activeIn(e, ym)) &&
      matches(terms, [employeeName(e), e.position, contractLabel(e.contract), contractShort(e.contract), insurerLabel(e.insurer), e.city, e.email]),
  )
}

const amt = (v) => (Number(v) ? String(v).replace('.', ',') : '')
const num = (v) => String(v ?? '').replace(/\s/g, '').replace(',', '.') || '0'

export function newEmployee(today = new Date()) {
  const y = today.getFullYear()
  const m = String(today.getMonth() + 1).padStart(2, '0')
  return {
    id: 0, firstName: '', lastName: '', title: '', rc: '', street: '', city: '', zip: '', iban: '', email: '', insurer: '25',
    contract: 'tpp', regular: true, start: `${y}-${m}-01`, end: '', position: '', hourly: false, wage: '', hours: '40',
    nczd: true, bonus: false, children: [], pension: '', ztp: false, oop: false, note: '',
  }
}

/** Zamestnanec z API → formulár (sumy s desatinnou čiarkou). */
export function employeeToForm(e) {
  return { ...e, wage: amt(e.wage), hours: amt(e.hours), children: (e.children ?? []).map((c) => ({ ...c, months: [...(c.months ?? Array(12).fill(false))] })) }
}

/** Formulár → API. */
export function employeeFromForm(f) {
  return {
    ...f,
    wage: num(f.wage),
    hours: num(f.hours),
    regular: f.contract === 'tpp' ? true : !!f.regular,
    oop: oopAllowed(f) ? !!f.oop : false,
    children: f.bonus ? f.children : [],
  }
}

/** Vstupy mesiaca z výplatnej listiny → formulár. */
export function inputToForm(i) {
  return { employeeId: i.employeeId, hours: amt(i.hours), base: amt(i.base), extra: amt(i.extra), leave: amt(i.leave), sickPay: amt(i.sickPay), deduction: amt(i.deduction), note: i.note ?? '' }
}

export function inputFromForm(f) {
  return { employeeId: f.employeeId, hours: num(f.hours), base: num(f.base), extra: num(f.extra), leave: num(f.leave), sickPay: num(f.sickPay), deduction: num(f.deduction), note: f.note }
}

/** Názov mesiaca: „marec 2026“. */
export const periodLabel = (year, month) => `${monthNames[month - 1]} ${year}`

/** Predchádzajúci / nasledujúci mesiac. */
export function shiftMonth(year, month, d) {
  const t = year * 12 + (month - 1) + d
  return { year: Math.floor(t / 12), month: (t % 12) + 1 }
}

const pairLabels = {
  '521/331': 'Hrubé mzdy',
  '527/331': 'Náhrada príjmu pri PN',
  '524/336': 'Poistné zamestnávateľa',
  '331/336': 'Poistné zamestnancov',
  '331/342': 'Preddavok na daň',
  '342/331': 'Daňový bonus vyplatený zamestnávateľom',
  '331/379': 'Zrážky zo mzdy',
}

/** Riadky dokladu miezd zo servera (MD, D, MD, D …) → dvojice na zobrazenie. */
export function postingPairs(lines = []) {
  const out = []
  for (let i = 0; i + 1 < lines.length; i += 2) {
    const md = lines[i].account
    const d = lines[i + 1].account
    out.push({ md, d, amount: lines[i].debit, label: pairLabels[`${md}/${d}`] ?? '' })
  }
  return out
}

/** Súčet vybraných úhrad v eurách (sumy sú hotové zo servera; sčíta sa v centoch). */
export function payTotal(payments = [], keys = []) {
  const set = new Set(keys)
  const cents = payments.filter((p) => set.has(p.key)).reduce((a, p) => a + Math.round(Number(p.amount) * 100), 0)
  return (cents / 100).toFixed(2)
}

/** Predvolene vybrané úhrady: všetky bez problému. */
export const payDefault = (payments = []) => payments.filter((p) => !p.problem).map((p) => p.key)

/** Platobné údaje → formulár so všetkými poisťovňami. */
export function paySettingsForm(s = {}, codes = ['24', '25', '27']) {
  const health = {}
  for (const c of codes) health[c] = { iban: s.health?.[c]?.iban ?? '', vs: s.health?.[c]?.vs ?? '' }
  return { socialIban: s.socialIban ?? '', socialVs: s.socialVs ?? '', taxOud: s.taxOud ?? '', payDay: s.payDay || null, contactFirst: s.contactFirst ?? '', contactLast: s.contactLast ?? '', contactPhone: s.contactPhone ?? '', contactEmail: s.contactEmail ?? '', health }
}

/** Ročné zúčtovanie: hľadanie a filter „iba žiadosti“. */
export function filterSettlements(list = [], { search = '', requested = false } = {}) {
  const terms = searchTerms(search)
  return list.filter((r) => (!requested || r.request?.requested) && matches(terms, [employeeName(r.employee), r.employee.position]))
}

/** Text výsledku ročného zúčtovania (suma zo servera: + zrazí, − vráti). */
export function settlementOutcome(result) {
  const v = Number(result)
  if (v > 0) return 'nedoplatok — zrazí zo mzdy'
  if (v < 0) return 'preplatok — vráti vo mzde'
  return 'bez rozdielu'
}

/** Žiadosť o ročné zúčtovanie → formulár a späť (sumy ako text). */
export function settlementToForm(r = {}) {
  return {
    requested: !!r.requested,
    dds: r.dds ?? '',
    spouse: !!r.spouse,
    spouseMonths: r.spouse?.months ?? 12,
    spouseIncome: r.spouse?.ownIncome ?? '',
    spouseName: r.spouse?.name ?? '',
    otherIncome: r.otherIncome ?? '',
    otherSocial: r.otherSocial ?? '',
    otherHealth: r.otherHealth ?? '',
    otherPrepaid: r.otherPrepaid ?? '',
    otherBonus: r.otherBonus ?? '',
    date: r.date ?? '',
    settleMonth: r.settleMonth ?? '',
  }
}

export function settlementFromForm(f) {
  const n = (v) => (v === '' || v == null ? '0' : String(v).replace(',', '.').replace(/\s/g, ''))
  return {
    requested: f.requested,
    dds: n(f.dds),
    spouse: f.spouse ? { name: f.spouseName, months: Number(f.spouseMonths) || 0, ownIncome: n(f.spouseIncome) } : null,
    otherIncome: n(f.otherIncome),
    otherSocial: n(f.otherSocial),
    otherHealth: n(f.otherHealth),
    otherPrepaid: n(f.otherPrepaid),
    otherBonus: n(f.otherBonus),
    date: f.date || '',
    settleMonth: f.settleMonth || '',
  }
}

/** Riadky tlačiva ročného zúčtovania (kód, text) v poradí. */
export const settlementLines = [
  ['01', 'Úhrn zdaniteľných príjmov zo závislej činnosti'],
  ['01a', 'z toho príjmy z dohôd'],
  ['02', 'Úhrn poistného zamestnanca'],
  ['02a', 'z toho sociálne poistenie'],
  ['02b', 'z toho zdravotné poistenie'],
  ['03', 'Základ dane (r. 01 − r. 02)'],
  ['04a', 'Nezdaniteľná časť na daňovníka'],
  ['04b', 'Nezdaniteľná časť na manžela/manželku'],
  ['04c', 'Príspevky na doplnkové dôchodkové sporenie'],
  ['04', 'Nezdaniteľné časti spolu'],
  ['05', 'Zdaniteľná mzda (r. 03 − r. 04)'],
  ['06', 'Daň podľa § 15'],
  ['09', 'Nárok na daňový bonus'],
  ['10', 'Daňový bonus vyplatený zamestnávateľom'],
  ['11', 'Daňový bonus na vyplatenie (r. 09 − r. 10 > 0)'],
  ['13', 'Daňový bonus na vybratie (r. 09 − r. 10 < 0)'],
  ['14', 'Daň znížená o daňový bonus'],
  ['18', 'Úhrn zrazených preddavkov na daň'],
  ['19', 'Nedoplatok dane (r. 06 − r. 18 > 0)'],
  ['20', 'Preplatok dane (r. 06 − r. 18 < 0)'],
  ['22', 'Zamestnávateľ zrazí (+) alebo vráti (−)'],
]

/** Riadky ročného zúčtovania na výplatnej páske (znamienko podľa vplyvu na čistú mzdu; sumy zo servera). */
export function settleLines(result = {}) {
  const out = []
  const abs = (v) => String(v).replace(/^-/, '')
  const neg = (v) => String(v).startsWith('-')
  const tax = result.settleTax
  if (tax && Number(tax)) {
    out.push(neg(tax) ? { label: 'Ročné zúčtovanie — preplatok dane', sign: '+', amount: abs(tax) } : { label: 'Ročné zúčtovanie — nedoplatok dane', sign: '−', amount: tax })
  }
  const bonus = result.settleBonus
  if (bonus && Number(bonus)) {
    out.push(neg(bonus) ? { label: 'Ročné zúčtovanie — vrátenie bonusu', sign: '−', amount: abs(bonus) } : { label: 'Ročné zúčtovanie — doplatok bonusu', sign: '+', amount: bonus })
  }
  return out
}

/** Mesiace, v ktorých možno vyrovnať ročné zúčtovanie za rok (január až apríl nasledujúceho roka). */
export function settleMonthOptions(year) {
  const y = year + 1
  return [{ label: 'ručne (mimo miezd)', value: '' }, ...[1, 2, 3, 4].map((m) => ({ label: `mzda za ${monthNames[m - 1]} ${y}`, value: `${y}-0${m}` }))]
}
