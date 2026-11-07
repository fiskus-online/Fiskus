// Rozhodnutie colného úradu pri dovoze (JCD) na prijatej faktúre.
// Sumy (DPH, clo, na úhradu) počíta server — tu sa iba pripravuje zadanie.
import { EU, isForeign } from './codes.js'
import { formatAmount, parseNumber } from './format.js'

/** Prijaté faktúry za tovar od dodávateľa mimo EÚ, ku ktorým sa dá priradiť rozhodnutie. */
export function importCandidates(list, selfId = 0) {
  return (list ?? [])
    .filter((p) => p.id !== selfId && !p.customs && isForeign(p.supplier?.country) && !EU.has(p.supplier.country) && p.supplyKind === 'goods')
    .map((p) => ({ value: p.id, label: `${p.number} – ${p.supplier.name} (${formatAmount(p.summary?.total)} ${p.currency || '€'})` }))
}

/** Údaje rozhodnutia na odoslanie (clo s čiarkou → bodka). */
export const customsPayload = (c) => (c ? { duty: parseNumber(c.duty) ?? '0', importId: c.importId || 0 } : undefined)

/** Údaje rozhodnutia na úpravu (clo s čiarkou). */
export const customsForm = (c) => (c ? { duty: Number(c.duty) ? formatAmount(c.duty) : '', importId: c.importId || 0, importNumber: c.importNumber ?? '' } : null)
