// Zákazky (CRM): stavy, filtre a formulár. Ekonomiku počíta server.

/** Stavy v poradí priebehu (tone = farba štítku ako pri dokladoch). */
export const jobStatuses = [
  { value: 'lead', label: 'Dopyt', tone: 'muted' },
  { value: 'offer', label: 'Ponuka', tone: 'draft' },
  { value: 'active', label: 'V realizácii', tone: 'issued' },
  { value: 'done', label: 'Dokončená', tone: 'paid' },
  { value: 'lost', label: 'Stratená', tone: 'cancelled' },
]

/** Filtre zoznamu. */
export const jobViews = [
  { label: 'Bežiace', value: 'open' },
  { label: 'Všetky', value: 'all' },
  { label: 'Po termíne', value: 'late' },
  { label: 'Dokončené', value: 'done' },
  { label: 'Stratené', value: 'lost' },
]

const open = (s) => s === 'lead' || s === 'offer' || s === 'active'

/** Stav na zobrazenie (otvorená po termíne = červená). */
export function jobStatusOf(j) {
  const s = jobStatuses.find((x) => x.value === j?.status) ?? jobStatuses[0]
  if (j?.late) return { label: `${s.label}, po termíne`, tone: 'overdue' }
  return { label: s.label, tone: s.tone }
}

/** Patrí zákazka do zobrazenia? */
export function matchesJobView(j, view) {
  switch (view) {
    case 'open':
      return open(j.status)
    case 'late':
      return !!j.late
    case 'done':
      return j.status === 'done'
    case 'lost':
      return j.status === 'lost'
  }
  return true
}

/** Nová zákazka. */
export function blankJob(contact = null) {
  return { id: 0, name: '', contactId: contact?.id ?? 0, customer: contact?.name ?? '', status: 'lead', start: '', due: '', budget: '', note: '',
    rate: '', work: [], material: [] }
}

const zero = (v) => (v && Number(v) !== 0 ? v : '')

/** Zákazka zo servera na úpravu vo formulári. */
export function jobForm(j) {
  return {
    id: j.id, name: j.name, contactId: j.contactId || 0, customer: j.customer, status: j.status, start: j.start || '', due: j.due || '',
    budget: zero(j.budget), note: j.note, rate: zero(j.rate),
    work: (j.work ?? []).map((w) => ({ ...w, costRate: zero(w.costRate) })),
    material: (j.material ?? []).map((m) => ({ ...m, unitCost: zero(m.unitCost) })),
  }
}

/** Nový riadok výkazu: dnešok, posledný pracovník a sadzba zákazky. */
export function blankWork(f, today) {
  const last = f.work.at(-1)
  return { id: 0, date: today, person: last?.person ?? '', description: '', hours: '', rate: last?.rate || f.rate || '', costRate: last?.costRate ?? '' }
}

/** Nový riadok materiálu. */
export function blankMaterial(today) {
  return { id: 0, date: today, itemId: 0, name: '', unit: 'ks', quantity: '', unitPrice: '', unitCost: '' }
}

/** Materiál z cenníka: názov, jednotka a predajná cena položky. */
export function pickItem(row, item) {
  if (!item) return { ...row, itemId: 0 }
  return { ...row, itemId: item.id, name: item.name, unit: item.unit || 'ks', unitPrice: item.price ?? row.unitPrice }
}

const num = (v) => {
  const t = String(v ?? '').replace(/\s/g, '').replace(',', '.')
  return t || '0'
}

/** Telo na uloženie (čísla a dátumy v tvare pre API). */
export function jobPayload(f) {
  const budget = String(f.budget ?? '').replace(/\s/g, '').replace(',', '.')
  return {
    name: (f.name ?? '').trim(),
    contactId: f.contactId || 0,
    status: f.status || 'lead',
    start: f.start || '',
    due: f.due || '',
    budget: budget || '0',
    note: (f.note ?? '').trim(),
    rate: num(f.rate),
    work: (f.work ?? []).map((w) => ({ id: w.id || 0, date: w.date || '', person: (w.person ?? '').trim(), description: (w.description ?? '').trim(),
      hours: num(w.hours), rate: num(w.rate), costRate: num(w.costRate) })),
    material: (f.material ?? []).map((m) => ({ id: m.id || 0, date: m.date || '', itemId: m.itemId || 0, name: (m.name ?? '').trim(), unit: m.unit || 'ks',
      quantity: num(m.quantity), unitPrice: num(m.unitPrice), unitCost: num(m.unitCost) })),
  }
}

/** Marža na zobrazenie: „73 %“ alebo „—“. */
export function marginText(s) {
  return s?.margin == null ? '—' : `${s.margin} %`
}
