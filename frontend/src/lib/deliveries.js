// Dodacie listy: stavy, filtre, formulár a výber na zbernú faktúru.
// Sumy počíta server.

/** Stavy (tone = farba štítku ako pri dokladoch). */
export const deliveryStatuses = [
  { value: 'draft', label: 'Koncept', tone: 'draft' },
  { value: 'issued', label: 'Vydaný', tone: 'issued' },
  { value: 'invoiced', label: 'Vyfakturovaný', tone: 'paid' },
  { value: 'cancelled', label: 'Stornovaný', tone: 'cancelled' },
]

/** Filtre zoznamu. */
export const deliveryViews = [
  { label: 'Na fakturáciu', value: 'issued' },
  { label: 'Všetky', value: 'all' },
  { label: 'Koncepty', value: 'draft' },
  { label: 'Vyfakturované', value: 'invoiced' },
]

/** Stav na zobrazenie. */
export function deliveryStatusOf(n) {
  const s = deliveryStatuses.find((x) => x.value === n?.status) ?? deliveryStatuses[0]
  return { label: s.label, tone: s.tone }
}

/** Patrí dodací list do zobrazenia? */
export function matchesDeliveryView(n, view) {
  return view === 'all' || n.status === view
}

/** Kľúč odberateľa (na zbernú faktúru musia byť rovnakí). */
export const customerKey = (n) => (n.contactId ? `c${n.contactId}` : `n${(n.customer?.name ?? '').trim().toLowerCase()}`)

/** Dá sa výber dať na jednu faktúru? Vráti text prekážky alebo ''. */
export function invoiceBlocker(list) {
  if (!list.length) return 'Vyberte vydané dodacie listy'
  if (list.some((n) => n.status !== 'issued')) return 'Na faktúru idú iba vydané, nevyfakturované dodacie listy'
  if (new Set(list.map(customerKey)).size > 1) return 'Vybrané dodacie listy majú rôznych odberateľov'
  if (new Set(list.map((n) => n.priceMode || 'net')).size > 1) return 'Vybrané dodacie listy majú ceny s DPH aj bez DPH'
  return ''
}

/** Nový dodací list. */
export function blankDelivery(contact = null, today = '') {
  return {
    id: 0, contactId: contact?.id ?? 0, customer: contact ? partyOf(contact) : { name: '', country: 'SK' }, date: today, place: '', order: '', note: '', receiver: '',
    priceMode: 'net', lines: [],
  }
}

/** Odberateľ z kontaktu adresára. */
export function partyOf(c) {
  return { name: c.name, street: c.street ?? '', city: c.city ?? '', zip: c.zip ?? '', country: c.country || 'SK', ico: c.ico ?? '', dic: c.dic ?? '', icdph: c.icdph ?? '', email: c.email ?? '' }
}

/** Dodací list zo servera na úpravu. */
export function deliveryForm(n) {
  return {
    id: n.id, contactId: n.contactId || 0, customer: { ...n.customer }, date: n.date || '', place: n.place ?? '', order: n.order ?? '', note: n.note ?? '',
    receiver: n.receiver ?? '', priceMode: n.priceMode || 'net', lines: (n.lines ?? []).map((l) => ({ ...l })),
  }
}

/** Riadok z položky cenníka (cena podľa režimu, neplatiteľ bez DPH). */
export function deliveryLine(it = null, { priceMode = 'net', nonVat = false, rate = 23 } = {}) {
  if (!it) return { itemId: 0, description: '', quantity: '1', unit: 'ks', unitPrice: '0', vatRate: nonVat ? 0 : rate }
  return { itemId: it.id, description: it.name, quantity: '1', unit: it.unit || 'ks', unitPrice: !nonVat && priceMode === 'gross' ? it.priceGross : it.priceNet,
    vatRate: nonVat ? 0 : it.vatRate }
}

const num = (v) => {
  const t = String(v ?? '').replace(/\s/g, '').replace(',', '.')
  return t || '0'
}

/** Telo na uloženie. */
export function deliveryPayload(f) {
  return {
    contactId: f.contactId || 0,
    customer: { ...f.customer, name: (f.customer?.name ?? '').trim() },
    date: f.date || '',
    place: (f.place ?? '').trim(),
    order: (f.order ?? '').trim(),
    note: (f.note ?? '').trim(),
    receiver: (f.receiver ?? '').trim(),
    priceMode: f.priceMode || 'net',
    lines: (f.lines ?? []).map((l) => ({ itemId: l.itemId || 0, description: (l.description ?? '').trim(), quantity: num(l.quantity), unit: l.unit || 'ks',
      unitPrice: num(l.unitPrice), vatRate: Number(l.vatRate ?? 0) })),
  }
}
