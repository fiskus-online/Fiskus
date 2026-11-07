// Tlačivá: definície a sumy sú zo servera (/api/forms), tu sa iba
// pripravujú hodnoty formulára, hľadá v uložených a skladá tlač.
import { formatDate } from './format.js'
import { matches, searchTerms } from './search.js'

export const fieldTypes = [
  { value: 'text', label: 'Text' },
  { value: 'textarea', label: 'Viac riadkov' },
  { value: 'money', label: 'Suma €' },
  { value: 'number', label: 'Číslo' },
  { value: 'date', label: 'Dátum' },
]

export const paperLabel = (p) => (p === 'A5' ? 'A5' : 'A4')

/** Hodnoty nového tlačiva: hlavička firmy, dnešný dátum. */
export function blankValues(form, { issuer = '', today = '' } = {}) {
  const v = {}
  for (const f of form?.fields ?? []) {
    v[f.key] = f.role === 'issuer' ? issuer : f.role === 'date' ? today : f.type === 'rate' ? '0' : ''
  }
  return v
}

/** Nové tlačivo zo vzoru: hodnoty vzoru, dnešný dátum, bez čísla. */
export function fromTemplate(tpl, form, { today = '' } = {}) {
  const v = { ...blankValues(form, { today }), ...tpl.values }
  for (const f of form?.fields ?? []) {
    if (f.role === 'date') v[f.key] = today
    if (f.role === 'number') v[f.key] = ''
  }
  return { values: v, items: (tpl.items ?? []).map((i) => ({ ...i })) }
}

export const blankItem = () => ({ desc: '', qty: '1', unit: 'ks', price: '' })

/** Položky na odoslanie: prázdne čísla ako nula. */
export const itemsForApi = (items) => (items ?? []).map((i) => ({ desc: i.desc ?? '', unit: i.unit ?? '', qty: numOr0(i.qty), price: numOr0(i.price) }))

function numOr0(s) {
  const t = String(s ?? '').replace(/[\s ]/g, '').replace(',', '.')
  return /^-?\d+(\.\d+)?$/.test(t) ? t : '0'
}

/** Hodnota poľa podľa roly (partner, purpose …). */
export function byRole(form, values, role) {
  const f = form?.fields?.find((x) => x.role === role)
  return f ? (values?.[f.key] ?? '') : ''
}

/** Polia do tela listu: všetko okrem hlavičky, strán, čísla, dátumu a podpisov. */
export function bodyFields(form) {
  const skip = new Set(['issuer', 'partner', 'number', 'date', 'sign1', 'sign2', 'amount', 'rate'])
  return (form?.fields ?? []).filter((f) => !skip.has(f.role))
}

/** Text hodnoty na tlač (dátum, suma). */
export function printValue(field, value) {
  if (!value) return ''
  if (field.type === 'date') return formatDate(value)
  return value
}

/** Hľadanie v uložených tlačivách (bez diakritiky, viac slov). */
export function filterDocs(docs, forms, q, templates = false) {
  const terms = searchTerms(q)
  const name = (k) => forms.find((f) => f.key === k)?.name ?? k
  return (docs ?? [])
    .filter((d) => !!d.template === templates)
    .filter((d) => matches(terms, [d.title, name(d.formKey), d.cashNumber, formatDate(d.updated), ...Object.values(d.values ?? {}), ...(d.items ?? []).map((i) => i.desc)]))
    .sort((a, b) => (b.updated ?? '').localeCompare(a.updated ?? '') || b.id - a.id)
}

/** Popis tlačiva v zozname: „Dodací list · A4 · položky“. */
export function formMeta(form) {
  const parts = [paperLabel(form.paper)]
  if (form.items) parts.push('položky')
  if (form.cash) parts.push('zápis do pokladne')
  if (!form.builtin) parts.push('vlastné')
  return parts.join(', ')
}

/** Nová definícia vlastného tlačiva. */
export const blankDef = () => ({ key: '', name: '', description: '', paper: 'A4', items: false, fields: [{ key: '', label: '', type: 'text', required: false, wide: false }] })
