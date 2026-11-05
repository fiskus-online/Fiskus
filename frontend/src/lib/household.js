// Domácnosť (domáce účtovníctvo): číselníky, filtre a formulár pohybu.
// Zostatky, súčty po mesiacoch a čerpanie rozpočtu počíta server
// (internal/household), tu sa iba zobrazujú.
import { matches, searchTerms } from './search.js'
import { formatDate, toISODate } from './format.js'

export const accountKinds = [
  { value: 'bank', label: 'Bežný účet', icon: 'pi-building-columns' },
  { value: 'cash', label: 'Hotovosť', icon: 'pi-wallet' },
  { value: 'card', label: 'Kreditná karta', icon: 'pi-credit-card' },
  { value: 'savings', label: 'Sporenie', icon: 'pi-database' },
]

export const accountKindLabel = (k) => accountKinds.find((x) => x.value === k)?.label ?? k
export const accountIcon = (k) => accountKinds.find((x) => x.value === k)?.icon ?? 'pi-wallet'

export const monthNames = ['január', 'február', 'marec', 'apríl', 'máj', 'jún', 'júl', 'august', 'september', 'október', 'november', 'december']
export const monthShort = ['I', 'II', 'III', 'IV', 'V', 'VI', 'VII', 'VIII', 'IX', 'X', 'XI', 'XII']

/** Druh pohybu: prevod, príjem alebo výdavok. */
export function entryType(e) {
  if (e.toAccountId) return 'transfer'
  return String(e.amount ?? '').trim().startsWith('-') ? 'expense' : 'income'
}

export const entryTypes = [
  { value: 'expense', label: 'Výdavok' },
  { value: 'income', label: 'Príjem' },
  { value: 'transfer', label: 'Prevod' },
]

export const typeLabel = (t) => entryTypes.find((x) => x.value === t)?.label ?? ''

/** Suma bez znamienka (do formulára). */
export const unsigned = (a) => String(a ?? '').trim().replace(/^[-+]/, '')

/** Nový pohyb pre formulár. */
export function newEntry(type = 'expense', accounts = [], today = new Date()) {
  const open = accounts.filter((a) => !a.closed)
  const first = (open.find((a) => a.kind === (type === 'income' ? 'bank' : 'cash')) ?? open[0])?.id ?? null
  return { id: 0, type, date: toISODate(today), accountId: first, toAccountId: null, amount: '', category: '', payee: '', note: '' }
}

/** Pohyb z API → formulár (suma bez znamienka a druh zvlášť). */
export function toForm(e) {
  return { ...e, type: entryType(e), amount: unsigned(e.amount), toAccountId: e.toAccountId || null }
}

/** Formulár → pohyb pre API (výdavok so záporným znamienkom). */
export function fromForm(f) {
  const amount = String(f.amount ?? '').trim().replace(/\s/g, '').replace(',', '.')
  const e = { id: f.id || 0, date: f.date, accountId: f.accountId, amount, category: f.category, payee: f.payee, note: f.note }
  if (f.type === 'transfer') {
    e.toAccountId = f.toAccountId
    e.category = ''
  } else if (f.type === 'expense' && amount && !amount.startsWith('-')) {
    e.amount = '-' + amount
  }
  return e
}

/** Kategórie pre druh pohybu (príjem/výdavok). */
export const categoriesFor = (cats, type) => cats.filter((c) => (type === 'income' ? c.income : !c.income)).map((c) => c.name)

export const accountName = (accounts, id) => accounts.find((a) => a.id === id)?.name ?? ''

/** Popis kategórie v zozname (prevod: „Prevod → Peňaženka“). */
export function entryCategory(e, accounts) {
  return e.toAccountId ? `Prevod → ${accountName(accounts, e.toAccountId)}` : e.category
}

/** Filter pohybov: účet (aj prevody naň), kategória, druh a hľadanie. */
export function filterEntries(rows, f, accounts = []) {
  const terms = searchTerms(f.search)
  return rows.filter((e) => {
    if (f.accountId && e.accountId !== f.accountId && e.toAccountId !== f.accountId) return false
    if (f.category && e.category !== f.category) return false
    if (f.type && entryType(e) !== f.type) return false
    if (f.month && Number(String(e.date).slice(5, 7)) !== f.month) return false
    return matches(terms, [formatDate(e.date), e.date, entryCategory(e, accounts), e.payee, e.note, accountName(accounts, e.accountId), e.amount])
  })
}

/** Výška stĺpcov grafu príjmov a výdavkov v percentách najvyššieho mesiaca. */
export function chartBars(income = [], expense = []) {
  const vals = [...income, ...expense].map(Number).filter((n) => Number.isFinite(n))
  const max = Math.max(0, ...vals)
  const pct = (v) => (max > 0 ? Math.max(0, (Number(v) / max) * 100) : 0)
  return monthShort.map((m, i) => ({ month: m, income: pct(income[i] ?? 0), expense: pct(expense[i] ?? 0) }))
}

/** Tón riadku rozpočtu: prečerpaný, blízko (90 %+), v poriadku. */
export function budgetTone(b) {
  if (b.over) return 'over'
  if (Number(b.budget) > 0 && b.percent >= 90) return 'near'
  return 'ok'
}

/** Šírka pruhu čerpania (najviac 100 %). */
export const budgetWidth = (b) => (Number(b.budget) > 0 ? Math.min(100, b.percent) : 100)

/** Kontrola kategórií pred uložením (server overí znova). */
export function categoryErrors(cats) {
  const e = {}
  const seen = new Set()
  cats.forEach((c, i) => {
    const n = String(c.name ?? '').trim().toLowerCase()
    if (!n) e[i] = 'Zadajte názov'
    else if (seen.has(n)) e[i] = 'Kategória je dvakrát'
    seen.add(n)
  })
  return e
}

/** Je firma domácnosť? */
export const isHousehold = (company) => company?.bookkeeping === 'household'

const cents = (a) => Math.round(Number(a) * 100) || 0
const fromCents = (c) => (c / 100).toFixed(2)

/** Súčty zoznamu (už vypočítané sumy pohybov): príjmy a výdavky bez prevodov. */
export function listTotals(rows) {
  let inc = 0
  let out = 0
  for (const e of rows) {
    if (e.toAccountId) continue
    const c = cents(e.amount)
    if (c > 0) inc += c
    else out -= c
  }
  return { income: fromCents(inc), expense: fromCents(out), count: rows.length }
}
