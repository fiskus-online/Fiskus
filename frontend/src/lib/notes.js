// Poznámky k účtovnej závierke malej účtovnej jednotky (Úč POD 3-01):
// formulár ↔ API a časti na tlač. Čísla počíta server; tu sa len skladajú
// texty a vynechajú prázdne časti (§ 4 ods. 3 opatrenia).
import { formatDate, parseNumber } from './format.js'

const textFields = ['activity', 'employees', 'group', 'bodies', 'policies', 'errors', 'secured', 'unusual', 'contingent', 'offBalance', 'events', 'other']

/** Formulár z odpovede servera. */
export function notesForm(n) {
  const i = n?.input ?? {}
  const f = {}
  for (const k of textFields) f[k] = i[k] ?? ''
  f.prevApproved = i.prevApproved || ''
  f.notGoingConcern = !!i.notGoingConcern
  f.liabilities5y = Number(i.liabilities5y) ? String(i.liabilities5y).replace('.', ',') : ''
  return f
}

/** Telo pre API; neplatná suma → null (chyba v poli). */
export function notesBody(f) {
  const body = { notGoingConcern: !!f.notGoingConcern, prevApproved: f.prevApproved || null }
  for (const k of textFields) body[k] = (f[k] ?? '').trim()
  const amt = (f.liabilities5y ?? '').trim()
  if (amt === '') body.liabilities5y = '0'
  else {
    const v = parseNumber(amt)
    if (v === null) return null
    body.liabilities5y = v
  }
  return body
}

/**
 * Časti poznámok na tlač: { title, paragraphs[] } v poradí článkov;
 * prázdne časti sa vynechajú. Zásady oceňovania sú zadaný text, inak návrh
 * Fiškusu.
 */
export function notesSections(n, money) {
  if (!n) return []
  const i = n.input ?? {}
  const c = n.company ?? {}
  const t = (s) => (s ?? '').trim()
  const paras = (s) => t(s).split(/\n\s*\n/).map((p) => p.trim()).filter(Boolean)
  const out = []
  const one = [`${c.name}${c.address ? `, ${c.address}` : ''}${c.ico ? `, IČO ${c.ico}` : ''}.`]
  if (t(i.activity)) one.push(`Predmet činnosti: ${t(i.activity)}`)
  if (i.prevApproved) one.push(`Účtovná závierka za predchádzajúce účtovné obdobie bola schválená ${formatDate(i.prevApproved)}.`)
  one.push(n.basis)
  one.push(...paras(i.group))
  if (t(i.employees)) one.push(`Priemerný prepočítaný počet zamestnancov: ${t(i.employees)}.`)
  out.push({ title: 'Čl. I. Všeobecné údaje', paragraphs: one })
  if (t(i.bodies)) out.push({ title: 'Čl. II. Informácie o orgánoch účtovnej jednotky', paragraphs: paras(i.bodies) })
  const three = [
    i.notGoingConcern
      ? 'Účtovná závierka nie je zostavená za predpokladu nepretržitého pokračovania v činnosti.'
      : 'Účtovná závierka je zostavená za predpokladu, že účtovná jednotka bude nepretržite pokračovať v činnosti.',
    ...paras(t(i.policies) || n.defaultPolicies),
    ...paras(i.errors),
  ]
  out.push({ title: 'Čl. III. Použité účtovné zásady a účtovné metódy', paragraphs: three })
  const four = []
  if (Number(i.liabilities5y)) four.push(`Záväzky so zostatkovou dobou splatnosti dlhšou ako päť rokov: ${money(i.liabilities5y)}.`)
  four.push(...paras(i.secured), ...paras(i.unusual))
  out.push({ title: 'Čl. IV. Informácie, ktoré vysvetľujú a dopĺňajú súvahu a výkaz ziskov a strát', paragraphs: four, table: (n.rows ?? []).length > 0 })
  if (t(i.contingent) || t(i.offBalance)) out.push({ title: 'Čl. V. Informácie o inom aktíve a inom pasíve', paragraphs: [...paras(i.contingent), ...paras(i.offBalance)] })
  if (t(i.events)) out.push({ title: 'Čl. VI. Informácie o udalostiach, ktoré nastali po dni, ku ktorému sa zostavuje účtovná závierka', paragraphs: paras(i.events) })
  if (t(i.other)) out.push({ title: 'Čl. VII. Ostatné informácie', paragraphs: paras(i.other) })
  return out
}
