// Uzávierka roka: texty stavu (kontroly a výpočty robí server).
import { formatDate } from './format.js'
import { matches, searchTerms } from './search.js'

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

/**
 * Uzávierkové a otváracie zápisy (zo servera) ako riadky tabuľky a exportu:
 * jeden riadok na účet, zápis a dátum pri každom riadku.
 */
export function closingRows(lc) {
  if (!lc) return []
  const rows = []
  const add = (list, date, part) =>
    (list ?? []).forEach((e, i) =>
      e.lines.forEach((l, j) => rows.push({ key: `${part}-${i}-${j}`, part, first: j === 0, n: i + 1, date, text: e.text,
        account: l.account, name: l.name ?? '', debit: Number(l.debit) ? l.debit : '', credit: Number(l.credit) ? l.credit : '' })))
  add(lc.closing, lc.closingDate, 'closing')
  add(lc.opening, lc.openingDate, 'opening')
  return rows
}

/** Hľadanie v zápisoch: text zápisu, účet, názov účtu, suma. */
export function filterClosing(rows, q) {
  const terms = searchTerms(q)
  // dátum a text zápisu pri prvom zobrazenom riadku zápisu
  let last = ''
  return rows
    .filter((r) => matches(terms, [r.text, r.account, r.name, r.debit, r.credit]))
    .map((r) => {
      const entry = r.key.split('-').slice(0, 2).join('-')
      const first = entry !== last
      last = entry
      return { ...r, first }
    })
}
