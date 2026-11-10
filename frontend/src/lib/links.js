// Krížové prepojenia dokladov: adresy, druhy a výber dokladu na prepojenie.
import { matches, searchTerms } from './search.js'

/** Druhy dokladov, ktoré sa dajú prepojiť (perm = právo na čítanie). */
export const linkKinds = [
  { value: 'invoice', label: 'Faktúra', short: 'Faktúry', perm: 'invoices.read' },
  { value: 'quote', label: 'Cenová ponuka', short: 'Ponuky', perm: 'invoices.read' },
  { value: 'purchase', label: 'Prijatá faktúra', short: 'Prijaté', perm: 'purchases.read' },
  { value: 'form', label: 'Tlačivo (dodací list…)', short: 'Tlačivá', perm: 'invoices.read' },
]

/** Adresa dokladu v programe. formKey je kľúč tlačiva (pri druhu form). */
export function docPath(d) {
  switch (d.kind) {
    case 'invoice':
      return `/faktury/${d.id}`
    case 'quote':
      return `/ponuky/${d.id}`
    case 'purchase':
      return `/prijate/${d.id}`
    case 'form':
      return d.formKey ? `/tlaciva?t=${encodeURIComponent(d.formKey)}&doc=${d.id}` : '/tlaciva'
  }
  return ''
}

/** Kandidáti na prepojenie zo zoznamu dokladov daného druhu (jednotný tvar). */
export function candidates(kind, list, forms = []) {
  const formName = (key) => forms.find((f) => f.key === key)?.name ?? ''
  const role = (doc, r) => {
    const f = forms.find((x) => x.key === doc.formKey)
    const field = f?.fields?.find((x) => x.role === r)
    return field ? String(doc.values?.[field.key] ?? '') : ''
  }
  return (list ?? [])
    .map((d) => {
      switch (kind) {
        case 'invoice':
          return { kind, id: d.id, number: d.number || 'koncept', partner: d.customer?.name ?? d.customerName ?? '', date: d.issueDate, total: d.summary?.total ?? d.total, draft: !d.number }
        case 'quote':
          return { kind, id: d.id, number: d.number || 'koncept', partner: d.customer?.name ?? '', date: d.issueDate, total: d.summary?.total ?? d.total, draft: !d.number }
        case 'purchase':
          return { kind, id: d.id, number: d.supplierNumber || d.number, partner: d.supplier?.name ?? '', date: d.issueDate, total: d.summary?.total ?? d.total }
        case 'form':
          if (d.template) return null
          return { kind, id: d.id, formKey: d.formKey, number: role(d, 'number') || d.title || formName(d.formKey), partner: role(d, 'partner').split('\n')[0],
            date: role(d, 'date') || d.updated, type: formName(d.formKey) }
      }
      return null
    })
    .filter(Boolean)
}

/** Kandidáti bez dokladu samotného a už prepojených, podľa hľadania, najnovšie prvé. */
export function pickable(list, { self, linked = [], q = '', limit = 50 }) {
  const terms = searchTerms(q)
  const taken = new Set(linked.map((l) => `${l.kind}:${l.id}`))
  if (self) taken.add(`${self.kind}:${self.id}`)
  return list
    .filter((c) => !taken.has(`${c.kind}:${c.id}`) && matches(terms, [c.number, c.partner, c.type, c.total]))
    .sort((a, b) => String(b.date ?? '').localeCompare(String(a.date ?? '')))
    .slice(0, limit)
}
