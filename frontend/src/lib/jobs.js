// Zákazky (CRM): stavy, filtre a formulár. Ekonomiku počíta server.

/** Stavy v poradí priebehu (tone = farba štítku ako pri dokladoch). */
export const jobStatuses = [
  { value: 'lead', label: 'Dopyt', tone: 'muted' },
  { value: 'offer', label: 'Ponuka', tone: 'draft' },
  { value: 'active', label: 'V realizácii', tone: 'issued' },
  { value: 'done', label: 'Dokončená', tone: 'paid' },
  { value: 'lost', label: 'Stratená', tone: 'cancelled' },
]

/** Druhy zákaziek podľa odboru (pridajú vlastné údaje). */
export const jobKinds = [
  { value: '', label: 'Všeobecná', icon: 'pi pi-briefcase' },
  { value: 'auto', label: 'Autoservis', icon: 'pi pi-car' },
  { value: 'install', label: 'Montáž', icon: 'pi pi-wrench' },
  { value: 'service', label: 'Servis zariadenia', icon: 'pi pi-cog' },
]

/** Názov druhu. */
export const jobKindLabel = (k) => jobKinds.find((x) => x.value === (k ?? ''))?.label ?? 'Všeobecná'

/** Prázdne údaje druhu. */
export function kindDetails(kind) {
  switch (kind) {
    case 'auto':
      return { vehicle: { plate: '', vin: '', make: '', model: '', year: '', mileage: '', fuel: '', fault: '', damage: '' } }
    case 'install':
      return { site: { address: '', workers: '', contact: '', scope: '' } }
    case 'service':
      return { device: { type: '', make: '', model: '', serial: '', warranty: false, purchased: '', fault: '', accessories: '' } }
  }
  return {}
}

/** Adresa kontaktu do miesta montáže. */
export function contactAddress(c) {
  if (!c) return ''
  return [c.street, [c.zip, c.city].filter(Boolean).join(' ')].filter(Boolean).join(', ')
}

/** Text na hľadanie podľa údajov druhu (EČV, VIN, výrobné číslo…). */
export function kindSearchText(j) {
  const v = j?.vehicle ?? {}
  const d = j?.device ?? {}
  const s = j?.site ?? {}
  return [j?.subject, v.plate, v.vin, v.make, v.model, d.serial, d.type, d.make, d.model, s.address, s.workers].filter(Boolean).join(' ')
}

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

/** Nová zákazka (druh napr. podľa poslednej založenej). */
export function blankJob(contact = null, kind = '') {
  const j = { id: 0, name: '', contactId: contact?.id ?? 0, customer: contact?.name ?? '', status: 'lead', start: '', due: '', budget: '', note: '',
    rate: '', work: [], material: [], kind, ...kindDetails(kind) }
  if (kind === 'install') j.site.address = contactAddress(contact)
  return j
}

/** Zmena druhu vo formulári: údaje nového druhu, pri montáži adresa zákazníka. */
export function setKind(f, kind, contact = null) {
  const { vehicle, site, device, ...rest } = f
  const keep = { auto: vehicle && { vehicle }, install: site && { site }, service: device && { device } }[kind]
  const out = { ...rest, kind, ...(keep || kindDetails(kind)) }
  if (kind === 'install' && !out.site.address) out.site = { ...out.site, address: contactAddress(contact) }
  return out
}

const zero = (v) => (v && Number(v) !== 0 ? v : '')

/** Zákazka zo servera na úpravu vo formulári. */
export function jobForm(j) {
  return {
    id: j.id, name: j.name, contactId: j.contactId || 0, customer: j.customer, status: j.status, start: j.start || '', due: j.due || '',
    budget: zero(j.budget), note: j.note, rate: zero(j.rate),
    work: (j.work ?? []).map((w) => ({ ...w, costRate: zero(w.costRate) })),
    material: (j.material ?? []).map((m) => ({ ...m, unitCost: zero(m.unitCost) })),
    kind: j.kind ?? '',
    ...kindDetails(j.kind ?? ''),
    ...(j.vehicle && { vehicle: { ...j.vehicle, year: j.vehicle.year || '', mileage: j.vehicle.mileage || '' } }),
    ...(j.site && { site: { ...j.site } }),
    ...(j.device && { device: { ...j.device, purchased: j.device.purchased || '' } }),
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
    kind: f.kind ?? '',
    ...kindPayload(f),
  }
}

const int = (v) => {
  const n = parseInt(String(v ?? '').replace(/\s/g, ''), 10)
  return Number.isFinite(n) ? n : 0
}
const str = (o) => Object.fromEntries(Object.entries(o).map(([k, v]) => [k, typeof v === 'string' ? v.trim() : v]))

/** Údaje druhu na uloženie (iba údaje zvoleného druhu). */
function kindPayload(f) {
  switch (f.kind) {
    case 'auto':
      return { vehicle: { ...str(f.vehicle ?? {}), year: int(f.vehicle?.year), mileage: int(f.vehicle?.mileage) } }
    case 'install':
      return { site: str(f.site ?? {}) }
    case 'service':
      return { device: { ...str(f.device ?? {}), warranty: !!f.device?.warranty, purchased: f.device?.warranty ? f.device?.purchased || '' : '' } }
  }
  return {}
}

/** Marža na zobrazenie: „73 %“ alebo „—“. */
export function marginText(s) {
  return s?.margin == null ? '—' : `${s.margin} %`
}

/** Stĺpce nástenky v poradí priebehu so zákazkami (stratené na konci). */
export function boardColumns(rows) {
  return jobStatuses.map((s) => ({ ...s, jobs: rows.filter((r) => r.status === s.value) }))
}

/** Telo na zmenu stavu zákazky z nástenky (ostatné údaje ostanú). */
export function movePayload(row, status) {
  return jobPayload({ ...jobForm(row), status })
}

/** Marža zo súhrnu ziskovosti na zobrazenie. */
export const pct = (v) => (v == null ? '—' : `${v} %`)
