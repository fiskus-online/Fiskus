// Objednávky dodávateľom: stavy, filtre a formulár. Sumy počíta server.

/** Stavy (tone = farba štítku ako pri dokladoch). */
export const porderStatuses = [
  { value: 'draft', label: 'Rozpracovaná', tone: 'draft' },
  { value: 'sent', label: 'Odoslaná', tone: 'issued' },
  { value: 'partial', label: 'Čiastočne prijatá', tone: 'issued' },
  { value: 'received', label: 'Prijatá', tone: 'paid' },
  { value: 'cancelled', label: 'Zrušená', tone: 'cancelled' },
]

/** Filtre zoznamu. */
export const porderViews = [
  { label: 'Otvorené', value: 'open' },
  { label: 'Všetky', value: 'all' },
  { label: 'Prijaté', value: 'received' },
  { label: 'Zrušené', value: 'cancelled' },
]

const open = (s) => s === 'draft' || s === 'sent' || s === 'partial'

/** Meškajúca: otvorená, očakávané dodanie už prešlo. */
export function porderLate(o, today) {
  return open(o?.status) && !!o?.expected && o.expected < today
}

/** Stav na zobrazenie. */
export function porderStatusOf(o, today = '') {
  const s = porderStatuses.find((x) => x.value === o?.status) ?? porderStatuses[0]
  if (today && porderLate(o, today)) return { label: `${s.label}, mešká`, tone: 'overdue' }
  return { label: s.label, tone: s.tone }
}

/** Patrí objednávka do zobrazenia? */
export function matchesPorderView(o, view) {
  if (view === 'open') return open(o.status)
  if (view === 'all') return true
  return o.status === view
}

/** Ešte neprijaté množstvo riadku zo servera (číslo). */
export function remaining(l) {
  return Math.max(0, Number(l.remaining ?? 0))
}

/** Nová objednávka. */
export function blankPorder(contact = null, today = '') {
  return { id: 0, contactId: contact?.id ?? 0, supplier: contact?.name ?? '', date: today, expected: '', note: '', lines: [] }
}

/** Objednávka zo servera na úpravu. */
export function porderForm(o) {
  return {
    id: o.id, contactId: o.contactId || 0, supplier: o.supplier ?? '', date: o.date || '', expected: o.expected || '', note: o.note ?? '',
    lines: (o.lines ?? []).map((l) => ({ ...l })),
  }
}

/** Nový riadok, voliteľne z položky cenníka (nákupná cena a kód dodávateľa). */
export function porderLine(item = null) {
  if (!item) return { itemId: 0, name: '', supplierCode: '', unit: 'ks', quantity: '', unitPrice: '', received: '0' }
  return { itemId: item.id, name: item.name, supplierCode: item.supplierCode ?? '', unit: item.unit || 'ks', quantity: '',
    unitPrice: item.purchasePrice && Number(item.purchasePrice) ? item.purchasePrice : '', received: '0' }
}

const num = (v) => {
  const t = String(v ?? '').replace(/\s/g, '').replace(',', '.')
  return t || '0'
}

/** Telo na uloženie. */
export function porderPayload(f) {
  return {
    contactId: f.contactId || 0,
    supplier: (f.supplier ?? '').trim(),
    date: f.date || '',
    expected: f.expected || '',
    note: (f.note ?? '').trim(),
    lines: (f.lines ?? []).map((l) => ({ itemId: l.itemId || 0, name: (l.name ?? '').trim(), supplierCode: (l.supplierCode ?? '').trim(),
      unit: l.unit || 'ks', quantity: num(l.quantity), unitPrice: num(l.unitPrice) })),
  }
}

/** Návrh príjmu: zostatky riadkov ako text do formulára. */
export function receiveDraft(o) {
  return (o.lines ?? []).map((l) => {
    return remaining(l) ? String(Number(l.remaining)) : ''
  })
}

/** Telo príjmu (množstvá podľa riadkov). */
export function receivePayload(date, warehouse, qty) {
  return { date: date || '', warehouse: warehouse || '', quantity: qty.map(num) }
}
