// Fiškus v prehliadači: čo musí prehliadač vedieť a texty úvodnej obrazovky.

/** Chýbajúce schopnosti prehliadača (prázdne = Fiškus sa dá spustiť). */
export function missingFeatures(g = globalThis) {
  const out = []
  if (typeof g.WebAssembly !== 'object') out.push('WebAssembly')
  if (typeof g.Worker !== 'function') out.push('Web Worker')
  if (!g.navigator?.serviceWorker) out.push('Service Worker')
  if (typeof g.navigator?.storage?.getDirectory !== 'function') out.push('úložisko súborov (OPFS)')
  if (!g.navigator?.locks) out.push('Web Locks')
  if (typeof g.DecompressionStream !== 'function') out.push('DecompressionStream')
  return out
}

/** Text priebehu načítania programu (bajty → „Načítavam program… 12 / 25 MB“). */
export function progressText(loaded, total) {
  const mb = (n) => Math.round(n / 1048576)
  if (!loaded) return 'Spúšťam Fiškus…'
  if (total && loaded < total) return `Načítavam program… ${mb(loaded)} / ${mb(total)} MB`
  return 'Otváram databázu…'
}

/** Podiel načítania 0–1 (bez známej veľkosti null). */
export function progressShare(loaded, total) {
  if (!total) return null
  return Math.min(1, loaded / total)
}

/** Podrobnosti chyby pri spúšťaní (na skopírovanie a nahlásenie). */
export function diagnosis(info = {}, ua = globalThis.navigator?.userAgent ?? '') {
  const storage = { opfs: 'úložisko súborov (OPFS)', indexeddb: 'IndexedDB', memory: 'iba v pamäti' }[info.storage] ?? 'nezistené'
  return [
    `Krok: ${info.step || 'nezistený'}`,
    `Chyba: ${info.detail || info.message || 'neznáma'}`,
    `Úložisko: ${storage}`,
    `Prehliadač: ${ua}`,
  ].join('\n')
}
