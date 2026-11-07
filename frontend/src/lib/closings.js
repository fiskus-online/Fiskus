// Uzávierky e-kasy: základ, DPH, súčet a hotovosť počíta server
// (POST …/cash/closings/preview). Tu sa iba skladá zadanie a hľadá v zozname.
import { formatAmount, formatDate } from './format.js'
import { matches, searchTerms } from './search.js'

const num = (s) => String(s ?? '').replace(/[\s ]/g, '').replace(',', '.')

/** Nová uzávierka: dnešný dátum, riadky v sadzbách platných v ten deň. */
export function blankClosing(date, rates, registerId = 0) {
  return { date, registerId, orp: '', number: '', receipts: null, rows: rates.filter((r) => r > 0).map((r) => ({ vatRate: r, gross: '' })), card: '', goods: false, note: '' }
}

/** Uzávierka na odoslanie (čiarka → bodka, prázdne sumy preč). */
export function closingPayload(f, isoDate) {
  return {
    date: isoDate,
    registerId: f.registerId || 0,
    orp: f.orp ?? '',
    number: f.number ?? '',
    receipts: Number(f.receipts) || 0,
    rows: f.rows.filter((r) => num(r.gross) !== '').map((r) => ({ vatRate: Number(r.vatRate), gross: num(r.gross) })),
    card: num(f.card) || '0',
    goods: !!f.goods,
    note: f.note ?? '',
  }
}

/** Chyby z náhľadu podľa poľa (rows.0.gross …). */
export const errorMap = (errors) => Object.fromEntries((errors ?? []).map((e) => [e.field, e.message]))

/** Hľadanie v uzávierkach (číslo, ORP, doklady, dátum, suma). */
export function filterClosings(list, q, showCancelled = true) {
  const terms = searchTerms(q)
  return (list ?? [])
    .filter((c) => showCancelled || !c.cancelled)
    .filter((c) => matches(terms, [c.number, c.orp, c.inNumber, c.outNumber, formatDate(c.date), formatAmount(closingTotal(c)), c.note, c.cancelled ? 'stornovaná' : '']))
    .sort((a, b) => b.date.localeCompare(a.date) || b.id - a.id)
}

/** Súčet obratu uloženej uzávierky (súčet súm zo servera). */
export const closingTotal = (c) => (c.rows ?? []).reduce((s, r) => s + Number(r.gross), 0)
/** Súčet DPH uloženej uzávierky. */
export const closingVAT = (c) => (c.rows ?? []).reduce((s, r) => s + Number(r.vat), 0)
