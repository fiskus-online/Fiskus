import { describe, expect, it } from 'vitest'
import { defaults, filterSchedule, resultRows, toRequest } from './calc.js'

const today = new Date(2026, 10, 7)

describe('kalkulačky', () => {
  const d = defaults(today, { vatRates: [23, 19, 5, 0], wageYears: [2025, 2026] })
  it.each([
    ['vat', { amount: '100', rate: 23, gross: false }],
    ['late', { amount: '1000', due: '2026-10-07', paid: '2026-11-07', debtor: 'business', rate: '0' }],
    ['loan', { amount: '10000', rate: '6', months: 60, fee: '0', monthlyFee: '0' }],
    ['savings', { initial: '1000', monthly: '100', rate: '3', months: 60, tax: '19' }],
    ['wage', { year: 2026, month: 11, gross: '1500', contract: 'tpp', regular: false, nczd: true, pension: '', children: [] }],
    ['fx', { amount: '100', currency: 'CZK', date: '2026-11-07', toEur: false }],
  ])('%s: požiadavka z predvolených hodnôt', (key, want) => {
    expect(toRequest(key, d[key])).toEqual(want)
  })

  it.each([
    ['vat', { amount: '' }],
    ['late', { ...d.late, debtor: 'custom', rate: '' }],
    ['loan', { ...d.loan, months: 0 }],
    ['savings', { ...d.savings, initial: '', monthly: '0' }],
    ['wage', { ...d.wage, gross: 'abc' }],
  ])('%s: nevyplnené sa neposiela', (key, f) => {
    expect(toRequest(key, f)).toBeFalsy()
  })

  it('čiarka v sume', () => {
    expect(toRequest('vat', { amount: '1 234,5', rate: 19, gross: true }).amount).toBe('1234.5')
  })

  it('výsledok úroku s paušálom a konečnou sumou', () => {
    const rows = resultRows('late', { from: '2026-10-01', days: 30, ecb: '2.65', margin: '9', rate: '11.65', interest: '9.58', fee: '40', total: '1049.58' })
    expect(rows.map((r) => r.label)).toEqual(['Omeškanie', 'Sadzba', 'Úrok z omeškania', 'Paušálna náhrada nákladov', 'Spolu s istinou'])
    expect(rows[0].value).toBe('30 dní od 1. 10. 2026')
    expect(rows.at(-1).total).toBe(true)
  })

  it('mzda bez bonusu nemá riadok bonusu', () => {
    const r = { gross: '1500', socialEmployee: '141', healthEmployee: '75', nczd: '497.23', tax: '149.48', taxBonus: '0', net: '1134.52', contribER: '543', cost: '2043' }
    expect(resultRows('wage', r).some((x) => x.label.includes('bonus'))).toBe(false)
  })

  it('hľadanie v splátkovom kalendári', () => {
    const rows = [{ n: 1, payment: '100', balance: '900', interest: '5' }, { n: 12, payment: '100', balance: '0', interest: '1' }]
    expect(filterSchedule(rows, '12.')).toHaveLength(1)
    expect(filterSchedule(rows, '')).toHaveLength(2)
  })
})
