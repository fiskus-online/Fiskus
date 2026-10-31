// E-faktúry: možnosti pripojenia k poštárovi a stav odoslania.
import { formatDate } from './format.js'

/** Spôsoby pripojenia. Priečinky na serveri iba pre správcu inštalácie. */
export function postmanModes(mode, superAdmin) {
  const out = [
    { label: 'Vypnuté', value: '' },
    { label: 'SAPI-SK', value: 'sapi' },
  ]
  if (mode !== 'server' || superAdmin) out.push({ label: 'Priečinky', value: 'folder' })
  out.push({ label: 'Vlastný adaptér', value: 'http' })
  return out
}

/**
 * Poskytovatelia s rozhraním SAPI-SK 1.0 (podľa ich verejnej dokumentácie).
 * Iný poskytovateľ so SAPI-SK sa zadá ručne adresou z jeho dokumentácie.
 */
export const sapiProviders = [
  { label: 'ePošťák', url: 'https://epostak.sk/sapi/v1' },
  { label: 'ePošťák – testovacie prostredie', url: 'https://dev.epostak.sk/sapi/v1' },
  { label: 'Verteco', url: 'https://peppol.verteco.digital/sapi' },
  { label: 'Iný poskytovateľ', url: '' },
]

/** Poskytovateľ podľa adresy (na predvyplnenie výberu). */
export const sapiProviderOf = (url) => sapiProviders.find((p) => p.url && p.url === url)?.label ?? 'Iný poskytovateľ'

/** Text o poslednom odoslaní e-faktúry („Odoslaná 29. 10. 2026“). */
export function sentText(list = []) {
  const last = list.find((e) => !e.error) ?? list[0]
  if (!last) return ''
  const day = formatDate(String(last.created).slice(0, 10))
  if (last.error) return `E-faktúra neodoslaná ${day}: ${last.error}`
  const state = deliveryText(last)
  return `E-faktúra odoslaná ${day}${last.peppolId ? ` na ${last.peppolId}` : ''}${state ? ` — ${state}` : ''}`
}

/** Stav doručenia od poštára (prázdny, kým nie je nič nové). */
export function deliveryText(e) {
  switch (e.status) {
    case 'delivered':
      return e.delivered ? `doručená ${formatDate(String(e.delivered).slice(0, 10))}` : 'doručená'
    case 'undeliverable':
      return 'nedoručiteľná (príjemca nie je v sieti Peppol)'
    case 'failed':
    case 'rejected':
      return 'doručenie zlyhalo'
    case 'sent':
      return 'na ceste k príjemcovi'
  }
  return ''
}

/** Má zmysel pýtať sa poštára na stav (odoslaná, ešte nie konečný stav)? */
export const trackable = (e) => !!e && !e.error && !!e.messageId && !['delivered', 'undeliverable', 'failed', 'rejected'].includes(e.status)
