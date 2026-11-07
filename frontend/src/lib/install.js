// Fiškus v prehliadači: funkcie, ktoré potrebujú nainštalovaný program,
// zostávajú v rozhraní; API namiesto nich vráti výzvu na inštaláciu
// (stav 501 s poľom „install“) a aplikácia zobrazí dialóg so stiahnutím.
import { reactive } from 'vue'

/** Zostavené pre prehliadač (npm run build:browser). */
export const inBrowser = import.meta.env.MODE === 'browser'

/** Stránka so stiahnutím nainštalovaného Fiškusa. */
export const downloadURL = 'https://fiskus.online/#stiahnut'

export const install = reactive({ feature: '' })

/** Zobrazí výzvu na inštaláciu pre funkciu feature. */
export function requireInstall(feature) {
  install.feature = feature || 'Táto funkcia'
}
