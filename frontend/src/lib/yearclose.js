// Uzávierka roka: texty stavu (kontroly a výpočty robí server).
import { formatDate } from './format.js'

/** Krátky stav uzávierky do hlavičky. */
export function closeSummary(info) {
  if (!info) return ''
  const c = info.close
  if (c?.closed) return `uzavretý ${formatDate(String(c.closedAt).slice(0, 10))}${c.closedBy ? ', ' + c.closedBy : ''}`
  if (info.canClose) return c ? 'otvorený po oprave — pripravený na uzávierku' : 'pripravený na uzávierku'
  const missing = (info.checks ?? []).filter((x) => x.blocking && !x.ok).length
  return missing ? `chýba ${missing === 1 ? '1 krok' : `${missing} kroky`}` : 'otvorený'
}

/** Udalosť v histórii uzávierky. */
export const eventLabel = (h) => (h.action === 'close' ? 'uzavretý' : 'otvorený')
