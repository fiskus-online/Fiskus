// Napojenie na Fio API: pomocné funkcie pre panel a hlásenia po stiahnutí.

// importSummary zhrnie výsledok načítania pohybov (výpis aj Fio API).
// V domácnosti sa pohyby párujú s bločkami (platba kartou).
export function importSummary(r, household = false) {
  const parts = [`${r.imported} nových pohybov`]
  if (r.autoMatched) parts.push(household ? `${r.autoMatched} spárovaných s bločkami` : `${r.autoMatched} automaticky spárovaných`)
  if (r.open) parts.push(`${r.open} na spárovanie`)
  if (r.duplicates) parts.push(`${r.duplicates} už načítaných`)
  return parts.join(', ')
}

// validToken: token z internetbankingu Fio sú písmená a číslice (zvyčajne 64).
export function validToken(t) {
  return /^[A-Za-z0-9]{20,128}$/.test((t || '').trim())
}

// fioPayload pripraví uloženie; prázdny token pri úprave ponechá uložený.
export function fioPayload(form) {
  const out = { id: form.id || 0, label: (form.label || '').trim() }
  if (form.householdAccountId) out.householdAccountId = form.householdAccountId
  const token = (form.token || '').replace(/\s+/g, '')
  if (token) out.token = token
  return out
}

// fioError: chybové hlásenia polí z API ako mapa pole → text.
export function fioError(err) {
  const map = {}
  for (const f of err?.fields || []) map[f.field] = f.message
  return map
}

// lastSyncLabel: kedy sa naposledy sťahovalo.
export function lastSyncLabel(acc, fmt) {
  return acc.lastSync ? `stiahnuté do ${fmt(acc.lastSync)}` : 'ešte nestiahnuté'
}

// householdAccountName: názov účtu domácnosti pri napojení.
export function householdAccountName(acc, list) {
  return list.find((x) => x.id === acc.householdAccountId)?.name ?? ''
}

// defaultHouseholdAccount: pri novom napojení bankový účet domácnosti
// s rovnakým IBAN, inak prvý bankový (nie hotovosť).
export function defaultHouseholdAccount(list, iban = '') {
  const open = list.filter((x) => !x.closed && x.kind !== 'cash')
  const norm = (s) => (s || '').replace(/\s+/g, '').toUpperCase()
  return (iban && open.find((x) => norm(x.iban) === norm(iban))?.id) || open[0]?.id || 0
}
