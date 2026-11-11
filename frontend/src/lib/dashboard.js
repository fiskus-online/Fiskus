// Prehľad firmy: pomocné texty a výška stĺpcov grafu. Sumy počíta server.
import { chartBars } from './household.js'

/** Stĺpce grafu príjmov a výdavkov po mesiacoch (v % z najväčšieho). */
export function monthBars(months = []) {
  return chartBars(
    months.map((m) => m.income),
    months.map((m) => m.expense),
  )
}

/** Adresa dokladu z neuhradenej položky (faktúra, zálohová, dobropis, prijatá). */
export function openItemLink(it) {
  return it.kind === 'purchase' ? `/prijate/${it.id}` : `/faktury/${it.id}`
}

/** Text splatnosti neuhradenej položky. */
export function dueText(it) {
  if (it.daysOverdue > 0) return it.daysOverdue === 1 ? 'po splatnosti 1 deň' : it.daysOverdue < 5 ? `po splatnosti ${it.daysOverdue} dni` : `po splatnosti ${it.daysOverdue} dní`
  return ''
}

/** Riadky rozpracovanej práce (iba nenulové), s odkazom. */
export function workRows(w) {
  if (!w) return []
  const n = (v, one, few, many) => `${v} ${v === 1 ? one : v < 5 ? few : many}`
  return [
    w.drafts && { label: n(w.drafts, 'koncept faktúry', 'koncepty faktúr', 'konceptov faktúr'), to: '/faktury' },
    w.jobs && { label: n(w.jobs, 'bežiaca zákazka', 'bežiace zákazky', 'bežiacich zákaziek'), sub: w.jobsLate ? `${w.jobsLate} po termíne` : '', late: !!w.jobsLate,
      amount: w.pipeline, to: '/zakazky' },
    w.orders && { label: n(w.orders, 'objednávka na dodanie', 'objednávky na dodanie', 'objednávok na dodanie'), sub: w.ordersLate ? `${w.ordersLate} po termíne` : '',
      late: !!w.ordersLate, to: '/prijate-objednavky' },
    w.quotes && { label: n(w.quotes, 'platná ponuka', 'platné ponuky', 'platných ponúk'), amount: w.quotesTotal, to: '/ponuky' },
    w.stockLow && { label: n(w.stockLow, 'položka na objednanie', 'položky na objednanie', 'položiek na objednanie'), to: '/sklad?tab=reorder' },
  ].filter(Boolean)
}
