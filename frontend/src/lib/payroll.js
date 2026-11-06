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
