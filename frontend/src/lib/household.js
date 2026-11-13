// Domácnosť (domáce účtovníctvo): číselníky, filtre a formulár pohybu.
// Zostatky, súčty po mesiacoch a čerpanie rozpočtu počíta server
// (internal/household), tu sa iba zobrazujú.
import { fold, matches, searchTerms } from './search.js'
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
  const e = { id: f.id || 0, date: f.date, accountId: f.accountId, amount, category: f.category, payee: f.payee, contactId: f.contactId || 0, note: f.note, obligationId: f.obligationId || 0 }
  if (f.split?.length && f.type !== 'transfer') {
    e.split = f.split.map((x) => ({ category: x.category, amount: x.amount, note: x.note, quantity: x.quantity }))
    e.source = f.source || ''
    if (f.receipt) e.receipt = f.receipt // údaje bločku z eKasy/fotky (server ich pri úprave ponechá)
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

// druhy záväzku v poradí výberu; pravidelná platba je najčastejšia, preto prvá
export const obligationKinds = [
  { value: 'regular', label: 'Pravidelná platba', icon: 'pi-replay', hint: 'Nájom, zálohy na energie, poistka, predplatné — bez dlžnej sumy' },
  { value: 'loan', label: 'Pôžička, úver', icon: 'pi-building-columns', hint: 'Banka, nebanková spoločnosť, hypotéka, leasing, nákup na splátky' },
  { value: 'debt', label: 'Dlh', icon: 'pi-user-minus', hint: 'Dlžíte niekomu — známemu, firme, štátu' },
  { value: 'execution', label: 'Exekúcia', icon: 'pi-exclamation-triangle', hint: 'Splátky exekútorovi podľa dohody alebo zrážky' },
  { value: 'lent', label: 'Požičal som', icon: 'pi-user-plus', hint: 'Niekto vám vracia peniaze — splátky sú príjem' },
  { value: 'fine', label: 'Pokuta, poplatok', icon: 'pi-ticket', hint: 'Pokuta, poplatok, platba do školy, krúžok, výlet — jednorazovo alebo na splátky' },
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
export function newObligation(kind = obligationKinds[0].value, accounts = [], today = new Date()) {
  const acc = accounts.find((a) => !a.closed && a.kind === 'bank') ?? accounts.find((a) => !a.closed)
  return {
    id: 0, kind, name: '', creditor: '', principal: '', paidBefore: '', installment: '', frequency: kind === 'regular' ? 'monthly' : 'monthly',
    firstDue: toISODate(today), count: 0, category: '', accountId: acc?.id ?? 0, reference: '', vs: '', iban: '', closed: false, note: '',
    address: '', months: [], remindDays: 0,
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
  return list.filter((o) => (all || (!o.closed && !o.state?.paidOff)) && matches(terms, [o.name, o.creditor, obligationKindLabel(o.kind), o.reference, o.vs, o.note, o.address, o.iban]))
}

// ---- mesiace platby a plán ----

/** Voľba mesiacov vo formulári (skratky). */
export const monthOptions = monthNames.map((n, i) => ({ value: i + 1, label: n.slice(0, 3) }))

/** Mesiace platby slovne: [9..12, 1..6] → „september – jún“; prázdne = ''. */
export function monthsLabel(months = []) {
  const set = new Set(months ?? [])
  if (!set.size || set.size === 12) return ''
  // úseky po sebe idúcich mesiacov (aj cez prelom roka)
  const start = [...set].filter((m) => !set.has(m === 1 ? 12 : m - 1)).sort((a, b) => a - b)
  return start
    .map((s) => {
      let e = s
      while (set.has(e === 12 ? 1 : e + 1) && (e === 12 ? 1 : e + 1) !== s) e = e === 12 ? 1 : e + 1
      return e === s ? monthNames[s - 1] : `${monthNames[s - 1]} – ${monthNames[e - 1]}`
    })
    .join(', ')
}

/** IBAN po štvoriciach: SK3112000000198742637541 → SK31 1200 0000 1987 4263 7541. */
export const ibanGroups = (v) => String(v ?? '').replace(/\s/g, '').replace(/(.{4})(?=.)/g, '$1 ')

/** Názov mesiaca plánu: 2026-11 → „november 2026“. */
export function planMonthLabel(ym) {
  const [y, m] = String(ym).split('-').map(Number)
  return `${monthNames[m - 1] ?? ''} ${y}`
}

/** Položky plánu (po splatnosti a mesiace) vyfiltrované hľadaním so súčtami zoznamu. */
export function filterPlan(plan, search = '') {
  const terms = searchTerms(search)
  const ok = (it) => matches(terms, [it.name, it.creditor, obligationKindLabel(it.kind), it.vs, it.iban, it.address])
  const sum = (items) => {
    let pay = 0
    let receive = 0
    for (const it of items) {
      const c = Math.round(Number(it.rest || 0) * 100)
      if (it.income) receive += c
      else pay += c
    }
    return { pay: fromCents(pay), receive: fromCents(receive) }
  }
  const overdue = (plan?.overdue ?? []).filter(ok)
  const months = (plan?.months ?? []).map((m) => {
    const items = m.items.filter(ok)
    return { month: m.month, items, ...sum(items) }
  })
  const all = months.flatMap((m) => m.items)
  return { overdue, overdueSum: sum(overdue), months, total: sum(all), rows: [...overdue.map((it) => ({ ...it, month: 'po splatnosti' })), ...months.flatMap((m) => m.items.map((it) => ({ ...it, month: planMonthLabel(m.month) })))] }
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

/** Druhy položiek eKasa (K = bežná položka). */
const itemTypes = { Z: 'zľava', V: 'vrátenie', VO: 'vratný obal', O: 'odpočet zálohy', OZ: 'odpočet zálohy', VP: 'výmena poukazu', PP: 'platba poukazom' }
const receiptTypes = { PD: 'pokladničný doklad', UF: 'úhrada faktúry', 'ÚF': 'úhrada faktúry', VK: 'vklad', VY: 'výber', ND: 'neplatný doklad' }

/**
 * Riadky bločku na zobrazenie: položka pohybu (kategória) spojená s položkou
 * bločku (množstvo, cena za kus, DPH) — spájajú sa podľa poradia, ak počet sedí.
 */
export function receiptRows(split = [], receipt = null) {
  const items = receipt?.items ?? []
  const paired = items.length === split.length
  return split.map((x, i) => {
    const it = paired ? items[i] : null
    return {
      index: i,
      name: x.note || it?.name || '—',
      quantity: it ? it.quantity : x.quantity || '',
      unitPrice: it?.unitPrice ?? '',
      vatRate: it && it.vatRate != null && receipt?.vat?.length ? `${it.vatRate} %` : '',
      kind: it?.type ? itemTypes[it.type] ?? '' : '',
      amount: it?.price ?? x.amount, // suma ako na bločku (kladná), inak suma pohybu
    }
  })
}

/** Údaje bločku ako zoznam „popis — hodnota“ (prázdne sa vynechajú). */
export function receiptFacts(r) {
  if (!r) return []
  const s = r.seller ?? {}
  const seat = [s.street, [s.zip, s.city].filter(Boolean).join(' ')].filter(Boolean).join(', ')
  const issued = r.issued && !r.issued.startsWith('0001') ? new Date(r.issued) : null
  const time = issued && (issued.getHours() || issued.getMinutes()) ? issued.toLocaleString('sk-SK', { dateStyle: 'long', timeStyle: 'short' }) : issued?.toLocaleDateString('sk-SK', { dateStyle: 'long' })
  return [
    ['Predajca', s.name],
    ['Sídlo', seat],
    ['Predajňa', r.unit],
    ['IČO', s.ico],
    ['DIČ', s.dic],
    ['IČ DPH', s.icDph],
    ['Vystavený', time],
    ['Číslo dokladu', r.number],
    ['Druh dokladu', receiptTypes[r.type] ?? r.type],
    ['Kód pokladnice', r.cashRegister],
    ['ID dokladu eKasa', r.id],
    ['Kupujúci', r.customer],
    ['Paragón', r.paragon ? 'áno — dodatočne zaevidovaný' : ''],
  ].filter(([, v]) => v)
}

// polia záväzku, ktoré prijíma server (household.Obligation); stav, splátkový
// kalendár a platby z detailu sa neposielajú — server neznáme polia odmietne
const obligationFields = ['id', 'kind', 'name', 'creditor', 'contactId', 'principal', 'paidBefore', 'installment', 'frequency', 'firstDue', 'count',
  'category', 'accountId', 'reference', 'vs', 'iban', 'address', 'months', 'remindDays', 'closed', 'note']

/** Telo na uloženie záväzku z formulára (sumy s čiarkou, dátum ako Date). */
export function obligationBody(f) {
  const body = {}
  for (const k of obligationFields) if (f[k] !== undefined) body[k] = f[k]
  return {
    ...body, principal: toAmount(f.principal) || '0', paidBefore: toAmount(f.paidBefore) || '0', installment: toAmount(f.installment) || '0',
    firstDue: toISODate(f.firstDue), count: f.count || 0, accountId: f.accountId || 0, remindDays: f.remindDays || 0,
    months: f.frequency === 'monthly' ? [...(f.months ?? [])] : [],
  }
}

/** Telo na uloženie účtu z formulára (bez zostatku a počtu pohybov zo zoznamu). */
export function accountBody(f) {
  return {
    id: f.id || 0, name: f.name ?? '', kind: f.kind, iban: f.iban ?? '', closed: !!f.closed,
    opening: String(f.opening ?? '').replace(/\s/g, '').replace(',', '.') || '0',
  }
}

// ---- partneri z adresára ----

/** Kontakt z adresára s presne týmto menom (bez diakritiky); 0 = žiadny alebo viac rovnakých. */
export function payeeContact(text, contacts) {
  const f = fold(String(text ?? '').trim())
  if (!f) return 0
  const hits = contacts.filter((c) => fold(c.name) === f)
  return hits.length === 1 ? hits[0].id : 0
}

/**
 * Návrhy príjemcu: najprv kontakty z adresára, potom obchody z histórie
 * pohybov, ktoré nie sú v adresári. Položka { label, contactId }.
 */
export function payeeOptions(q, contacts, payees = [], limit = 12) {
  const f = fold(String(q ?? '').trim())
  const has = (s) => !f || fold(s).includes(f)
  const out = contacts.filter((c) => c.name && has(c.name)).map((c) => ({ label: c.name, contactId: c.id, city: c.city || '' }))
  const known = new Set(contacts.map((c) => fold(c.name)))
  for (const p of payees) {
    if (out.length >= limit) break
    if (p && has(p) && !known.has(fold(p))) {
      known.add(fold(p))
      out.push({ label: p, contactId: 0, city: '' })
    }
  }
  return out.slice(0, limit)
}

/**
 * Zmena príjemcu vo formulári: vybraný návrh (objekt) nastaví meno aj
 * kontakt, napísaný text sa spáruje s kontaktom podľa mena. Ak sa text
 * nezmenil, väzba ostane (napr. pohyb z banky spárovaný podľa IBAN).
 */
export function setPayee(form, value, contacts, field = 'payee') {
  if (value && typeof value === 'object') {
    form[field] = value.label
    form.contactId = value.contactId || 0
    return form
  }
  const text = value ?? ''
  if (text === form[field]) return form
  form[field] = text
  form.contactId = payeeContact(text, contacts)
  return form
}

/** Účty partnera na výber pri záväzku: hlavný IBAN a ďalšie účty s účelom. */
export function partnerAccounts(contact) {
  if (!contact) return []
  const out = []
  const seen = new Set()
  for (const a of [{ iban: contact.iban, label: 'hlavný účet' }, ...(contact.accounts ?? [])]) {
    const key = String(a.iban ?? '').replace(/\s/g, '').toUpperCase()
    if (!key || seen.has(key)) continue
    seen.add(key)
    out.push({ iban: a.iban, key, label: a.label || 'účet' })
  }
  return out
}
