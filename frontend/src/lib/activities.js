// CRM aktivity: druhy, nový záznam a texty termínu úlohy.

/** Druhy záznamov s ikonou (PrimeIcons). */
export const activityKinds = [
  { value: 'note', label: 'Poznámka', icon: 'pi pi-pencil' },
  { value: 'call', label: 'Telefonát', icon: 'pi pi-phone' },
  { value: 'email', label: 'E-mail', icon: 'pi pi-envelope' },
  { value: 'meeting', label: 'Stretnutie', icon: 'pi pi-users' },
  { value: 'task', label: 'Úloha', icon: 'pi pi-check-square' },
]

export const activityKind = (k) => activityKinds.find((x) => x.value === k) ?? activityKinds[0]

/** Nový záznam pre kontakt, zákazku alebo ponuku. */
export function blankActivity(scope = {}, today = '') {
  return { kind: 'note', text: '', date: today, due: '', contactId: scope.contactId || 0, jobId: scope.jobId || 0, quoteId: scope.quoteId || 0 }
}

/** Telo na uloženie (termín iba pri úlohe). */
export function activityPayload(f) {
  return {
    kind: f.kind || 'note',
    text: (f.text ?? '').trim(),
    date: f.date || '',
    due: f.kind === 'task' ? f.due || '' : '',
    done: !!f.done,
    contactId: f.contactId || 0,
    jobId: f.jobId || 0,
    quoteId: f.quoteId || 0,
  }
}

const day = 86400000
/** Termín úlohy relatívne k dnešku: „dnes“, „zajtra“, „o 3 dni“, „včera“, „pred 4 dňami“. */
export function dueText(due, today) {
  if (!due || !today) return ''
  const n = Math.round((Date.parse(due) - Date.parse(today)) / day)
  if (n === 0) return 'dnes'
  if (n === 1) return 'zajtra'
  if (n === -1) return 'včera'
  if (n > 1) return n < 5 ? `o ${n} dni` : `o ${n} dní`
  return `pred ${-n} dňami`
}

/** Hľadanie v zázname (text, autor, čoho sa týka). */
export const activityText = (a) => [a.text, a.author, a.about, activityKind(a.kind).label]

/** Poradie: nesplnené úlohy podľa termínu (najbližší hore), potom ostatné od najnovších. */
export function sortActivities(list) {
  const open = (a) => a.kind === 'task' && !a.done
  return [...list].sort((a, b) => {
    if (open(a) !== open(b)) return open(a) ? -1 : 1
    if (open(a)) return String(a.due).localeCompare(String(b.due)) || a.id - b.id
    return String(b.date).localeCompare(String(a.date)) || b.id - a.id
  })
}
