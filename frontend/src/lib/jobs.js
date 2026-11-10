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
  return { id: 0, name: '', contactId: contact?.id ?? 0, customer: contact?.name ?? '', status: 'lead', start: '', due: '', budget: '', note: '' }
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
  }
}

/** Marža na zobrazenie: „73 %“ alebo „—“. */
export function marginText(s) {
  return s?.margin == null ? '—' : `${s.margin} %`
}
