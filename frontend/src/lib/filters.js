// Filtre zoznamov s viacerými voľbami (zaškrtávanie): prázdny výber = všetko.

/** Výber filtra ako pole: null/undefined → [], hodnota → [hodnota]. */
export function picked(v) {
  if (v === null || v === undefined || v === '' || v === 'all') return []
  return Array.isArray(v) ? v : [v]
}

/** Spĺňa riadok filter? Prázdny výber prepustí všetko, inak aspoň jedna voľba. */
export function anyPicked(v, test) {
  const p = picked(v)
  return p.length === 0 || p.some(test)
}

/** Hodnota je medzi vybranými (prázdny výber = áno). */
export function inPicked(v, x) {
  return anyPicked(v, (p) => p === x)
}

/** Jediná vybraná hodnota (napr. predvolený účet nového pohybu), inak null. */
export function onlyPicked(v) {
  const p = picked(v)
  return p.length === 1 ? p[0] : null
}

/** Voľby filtra bez „Všetky“ (prázdny výber je všetko). */
export function withoutAll(options) {
  return options.filter((o) => o.value !== 'all')
}
