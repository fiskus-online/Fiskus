// Telefón mimo domu: texty a pomocné funkcie (logika je v backend/internal/app/sync.go).
import { reactive } from 'vue'

const dt = new Intl.DateTimeFormat('sk-SK', { day: 'numeric', month: 'numeric', hour: '2-digit', minute: '2-digit' })

/** „13. 11. 12:30“ z času RFC 3339 (prázdne = ''). */
export function syncTime(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '' : dt.format(d).replace(/\s+/g, ' ')
}

/** „1 záznam“, „3 záznamy“, „7 záznamov“. */
export function itemsText(n) {
  if (n === 1) return '1 záznam'
  if (n >= 2 && n <= 4) return `${n} záznamy`
  return `${n} záznamov`
}

/** Odkaz na pripojenie telefónu z adresy (#pripojit&…); inak ''. */
export function joinFragment(hash) {
  return typeof hash === 'string' && hash.startsWith('#pripojit&') ? hash : ''
}

/** Pruh na telefóne: odkiaľ je kópia, z kedy a čo čaká na prenos. */
export function phoneLine(p) {
  if (!p) return ''
  const parts = [`Kópia z počítača „${p.desktop}“`]
  parts.push(p.version ? `stav k ${syncTime(p.version)}` : 'čaká sa na prvú kópiu')
  if (p.pending) parts.push(`${itemsText(p.pending)} ${p.pending >= 2 && p.pending <= 4 ? 'čakajú' : 'čaká'} na prenos do počítača`)
  return parts.join(' · ')
}

/** Text o poslednom spojení so schránkou (pre chybu spojenia). */
export function phoneProblem(p) {
  if (!p?.lastError) return ''
  return /dostupná|fetch|network/i.test(p.lastError) ? 'Bez internetu — záznamy sa prenesú, keď bude telefón online.' : p.lastError
}

// stav telefónu pre celé rozhranie (pruh hore, skrytie pripomienky zálohy)
export const syncState = reactive({ phone: null })

// odkaz z QR kódu zachytený pri spustení (adresa sa hneď vyčistí, aby
// tajomstvo neostalo v histórii prehliadača)
let pending = ''
export function rememberJoin(fragment) {
  pending = fragment
}
export function takeJoin() {
  return pending
}

/** Meno počítača z odkazu (#pripojit&k=…&n=…). */
export function joinName(fragment) {
  const q = new URLSearchParams(String(fragment).replace(/^#?pripojit&/, ''))
  return q.get('n') || 'Počítač'
}
