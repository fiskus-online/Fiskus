// Prijaté objednávky od zákazníkov: stavy, filtre, formulár a výber
// množstiev na dodanie. Sumy, zvyšky a voľné množstvá počíta server.
import { partyOf } from './deliveries.js'

export { partyOf }

/** Stavy (tone = farba štítku ako pri dokladoch). */
export const salesOrderStatuses = [
  { value: 'open', label: 'Prijatá', tone: 'issued' },
  { value: 'partial', label: 'Čiastočne dodaná', tone: 'issued' },
  { value: 'done', label: 'Vybavená', tone: 'paid' },
  { value: 'cancelled', label: 'Zrušená', tone: 'cancelled' },
]

/** Filtre zoznamu. */
export const salesOrderViews = [
  { label: 'Na dodanie', value: 'active' },
  { label: 'Všetky', value: 'all' },
  { label: 'Vybavené', value: 'done' },
  { label: 'Zrušené', value: 'cancelled' },
]

/** Stav na zobrazenie; aktívna po termíne dodania je omeškaná. */
export function salesOrderStatusOf(o, today = '') {
  const s = salesOrderStatuses.find((x) => x.value === o?.status) ?? salesOrderStatuses[0]
  if (isLate(o, today)) return { label: `${s.label}, po termíne`, tone: 'overdue' }
  return { label: s.label, tone: s.tone }
}

/** Čaká objednávka na dodanie? */
export const isActive = (o) => o?.status === 'open' || o?.status === 'partial'

/** Aktívna objednávka po požadovanom termíne dodania. */
export const isLate = (o, today) => !!(today && isActive(o) && o.due && o.due < today)

/** Patrí objednávka do zobrazenia? */
export function matchesSalesOrderView(o, view) {
  if (view === 'all') return true
  if (view === 'active') return isActive(o)
  return o.status === view
}

/** Nová objednávka. */
export function blankSalesOrder(contact = null, today = '') {
  return {
    id: 0, contactId: contact?.id ?? 0, customer: contact ? partyOf(contact) : { name: '', country: 'SK' }, date: today, due: '', ref: '', place: '', note: '',
    priceMode: 'net', lines: [],
  }
}

/** Objednávka zo servera na úpravu. */
export function salesOrderForm(o) {
  return {
    id: o.id, contactId: o.contactId || 0, customer: { ...o.customer }, date: o.date || '', due: o.due || '', ref: o.ref ?? '', place: o.place ?? '',
    note: o.note ?? '', priceMode: o.priceMode || 'net', lines: (o.lines ?? []).map((l) => ({ ...l })),
  }
}

/** Riadok z položky cenníka (cena podľa režimu, neplatiteľ bez DPH). */
export function salesOrderLine(it = null, { priceMode = 'net', nonVat = false, rate = 23 } = {}) {
  if (!it) return { key: 0, itemId: 0, description: '', quantity: '1', unit: 'ks', unitPrice: '0', vatRate: nonVat ? 0 : rate }
  return { key: 0, itemId: it.id, description: it.name, quantity: '1', unit: it.unit || 'ks', unitPrice: !nonVat && priceMode === 'gross' ? it.priceGross : it.priceNet,
    vatRate: nonVat ? 0 : it.vatRate }
}

const num = (v) => {
  const t = String(v ?? '').replace(/\s/g, '').replace(',', '.')
  return t || '0'
}

/** Telo na uloženie (kľúč riadku ostáva — dodania sa naň odvolávajú). */
export function salesOrderPayload(f) {
  return {
    contactId: f.contactId || 0,
    customer: { ...f.customer, name: (f.customer?.name ?? '').trim() },
    date: f.date || '',
    due: f.due || '',
    ref: (f.ref ?? '').trim(),
    place: (f.place ?? '').trim(),
    note: (f.note ?? '').trim(),
    priceMode: f.priceMode || 'net',
    lines: (f.lines ?? []).map((l) => ({ key: l.key || 0, itemId: l.itemId || 0, description: (l.description ?? '').trim(), quantity: num(l.quantity),
      unit: l.unit || 'ks', unitPrice: num(l.unitPrice), vatRate: Number(l.vatRate ?? 0) })),
  }
}

/** Návrh množstiev na dodanie: celý zvyšok každého riadku. */
export function pickDraft(o) {
  const out = {}
  for (const l of o?.lines ?? []) if (Number(l.remaining) > 0) out[l.key] = String(l.remaining).replace(/\.?0+$/, '')
  return out
}

/** Telo na dodanie: iba kladné množstvá. */
export function pickPayload(to, qty) {
  const out = {}
  for (const [k, v] of Object.entries(qty ?? {})) {
    const n = num(v)
    if (Number(n) > 0) out[k] = n
  }
  return { to, qty: out }
}

/** Chýba na sklade? Vráti chýbajúce množstvo zo servera ako číslo alebo 0. */
export const shortage = (l) => Number(l?.short ?? 0)
