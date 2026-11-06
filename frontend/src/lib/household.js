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
  return { ...e, type: entryType(e), amount: unsigned(e.amount).replace('.', ','), toAccountId: e.toAccountId || null, split: e.split?.map((x) => ({ ...x })) }
}

/** Formulár → pohyb pre API (výdavok so záporným znamienkom). */
export function fromForm(f) {
  const amount = String(f.amount ?? '').trim().replace(/\s/g, '').replace(',', '.')
  const e = { id: f.id || 0, date: f.date, accountId: f.accountId, amount, category: f.category, payee: f.payee, note: f.note, obligationId: f.obligationId || 0 }
  if (f.split?.length && f.type !== 'transfer') {
    e.split = f.split.map((x) => ({ category: x.category, amount: x.amount, note: x.note, quantity: x.quantity }))
    e.source = f.source || ''
  }
  if (f.type === 'transfer') {
    e.toAccountId = f.toAccountId
    e.category = ''
    e.obligationId = 0
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
  if (e.toAccountId) return `Prevod → ${accountName(accounts, e.toAccountId)}`
  const others = new Set((e.split ?? []).map((x) => x.category))
  others.delete(e.category)
  return others.size ? `${e.category} + ${others.size} ďalš${others.size === 1 ? 'ia' : others.size < 5 ? 'ie' : 'ích'}` : e.category
}

/** Filter pohybov: účet (aj prevody naň), kategória, druh a hľadanie. */
export function filterEntries(rows, f, accounts = []) {
  const terms = searchTerms(f.search)
  return rows.filter((e) => {
    if (f.accountId && e.accountId !== f.accountId && e.toAccountId !== f.accountId) return false
    if (f.category && e.category !== f.category && !(e.split ?? []).some((x) => x.category === f.category)) return false
    if (f.type && entryType(e) !== f.type) return false
    if (f.month && Number(String(e.date).slice(5, 7)) !== f.month) return false
    return matches(terms, [formatDate(e.date), e.date, entryCategory(e, accounts), e.payee, e.note, accountName(accounts, e.accountId), e.amount,
      ...(e.split ?? []).flatMap((x) => [x.note, x.category])])
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

// ---- záväzky ----

export const obligationKinds = [
  { value: 'loan', label: 'Pôžička, úver', icon: 'pi-building-columns', hint: 'Banka, nebanková spoločnosť, hypotéka, leasing, nákup na splátky' },
  { value: 'debt', label: 'Dlh', icon: 'pi-user-minus', hint: 'Dlžíte niekomu — známemu, firme, štátu' },
  { value: 'execution', label: 'Exekúcia', icon: 'pi-exclamation-triangle', hint: 'Splátky exekútorovi podľa dohody alebo zrážky' },
  { value: 'regular', label: 'Pravidelná platba', icon: 'pi-replay', hint: 'Nájom, zálohy na energie, poistka, predplatné — bez dlžnej sumy' },
  { value: 'lent', label: 'Požičal som', icon: 'pi-user-plus', hint: 'Niekto vám vracia peniaze — splátky sú príjem' },
]
export const obligationKindLabel = (k) => obligationKinds.find((x) => x.value === k)?.label ?? k
export const obligationIcon = (k) => obligationKinds.find((x) => x.value === k)?.icon ?? 'pi-file'

export const frequencies = [
  { value: 'monthly', label: 'Mesačne' },
  { value: 'quarterly', label: 'Štvrťročne' },
  { value: 'halfyear', label: 'Polročne' },
  { value: 'yearly', label: 'Ročne' },
  { value: 'once', label: 'Jednorazovo' },
]
export const frequencyLabel = (f) => frequencies.find((x) => x.value === f)?.label ?? f

export const dueStatuses = {
  paid: { label: 'zaplatená', tone: 'paid' },
  partial: { label: 'čiastočne', tone: 'draft' },
  overdue: { label: 'po splatnosti', tone: 'overdue' },
  upcoming: { label: 'čaká', tone: 'issued' },
}
export const dueStatus = (s) => dueStatuses[s] ?? { label: s, tone: 'cancelled' }

/** Nový záväzok pre formulár. */
export function newObligation(kind = 'loan', accounts = [], today = new Date()) {
  const acc = accounts.find((a) => !a.closed && a.kind === 'bank') ?? accounts.find((a) => !a.closed)
  return {
    id: 0, kind, name: '', creditor: '', principal: '', paidBefore: '', installment: '', frequency: kind === 'regular' ? 'monthly' : 'monthly',
    firstDue: toISODate(today), count: 0, category: '', accountId: acc?.id ?? 0, reference: '', vs: '', iban: '', closed: false, note: '',
  }
}

/** Splatené v percentách (na pruh); pri pravidelnej platbe null. */
export function paidPercent(o) {
  const total = Number(o.principal)
  if (!(total > 0)) return null
  return Math.max(0, Math.min(100, Math.round((Number(o.state?.paid) / total) * 100)))
}

/** Stav záväzku na zobrazenie. */
export function obligationTone(o) {
  if (o.closed) return { label: 'ukončený', tone: 'cancelled' }
  if (o.state?.paidOff) return { label: 'splatený', tone: 'paid' }
  if (o.state?.overdue > 0) return { label: o.state.overdue === 1 ? '1 splátka po splatnosti' : `${o.state.overdue} splátky po splatnosti`, tone: 'overdue' }
  return { label: 'splácam', tone: 'issued' }
}

/** Filter záväzkov: otvorené / všetky a hľadanie. */
export function filterObligations(list, { all = false, search = '' } = {}) {
  const terms = searchTerms(search)
  return list.filter((o) => (all || (!o.closed && !o.state?.paidOff)) && matches(terms, [o.name, o.creditor, obligationKindLabel(o.kind), o.reference, o.vs, o.note]))
}

/** Suma zo vstupu (1 234,50 → 1234.50) pre API. */
export const toAmount = (v) => String(v ?? '').replace(/\s/g, '').replace(',', '.')
/** Suma z API do vstupu (1234.50 → 1234,50; nula → prázdne). */
export const fromAmount = (v) => (Number(v) ? String(v).replace('.', ',') : '')

// ---- bločky ----

/** Súčty položiek bločku po kategóriách (už vypočítané sumy), najväčšie prvé. */
export function splitByCategory(split = []) {
  const by = new Map()
  for (const x of split) by.set(x.category, (by.get(x.category) ?? 0) + Math.round(Number(x.amount) * 100))
  return [...by.entries()].map(([category, c]) => ({ category, amount: (c / 100).toFixed(2) })).sort((a, b) => Math.abs(b.amount) - Math.abs(a.amount))
}

/** Zaradí všetky položky do jednej kategórie. */
export const setAllCategory = (split, category) => split.map((x) => ({ ...x, category }))
