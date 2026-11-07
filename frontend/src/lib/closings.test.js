import { describe, expect, it } from 'vitest'
import { blankClosing, closingPayload, closingTotal, errorMap, filterClosings } from './closings.js'

describe('uzávierky e-kasy', () => {
  it('nová uzávierka má riadky v kladných sadzbách', () => {
    const c = blankClosing('2026-10-12', [23, 19, 5, 0], 3)
    expect(c.rows.map((r) => r.vatRate)).toEqual([23, 19, 5])
    expect(c.registerId).toBe(3)
  })
  it('na odoslanie: čiarky, prázdne riadky preč', () => {
    const f = { ...blankClosing('', [23, 5]), number: '41', receipts: '37', card: '1 200,50' }
    f.rows[0].gross = '1 845,30'
    expect(closingPayload(f, '2026-10-12')).toEqual({
      date: '2026-10-12', registerId: 0, orp: '', number: '41', receipts: 37, rows: [{ vatRate: 23, gross: '1845.30' }], card: '1200.50', goods: false, note: '',
    })
    expect(closingPayload({ ...f, card: '' }, '2026-10-12').card).toBe('0')
  })
  it('chyby podľa poľa', () => {
    expect(errorMap([{ field: 'card', message: 'X' }])).toEqual({ card: 'X' })
  })
  it.each([
    ['', true, 3],
    ['', false, 2],
    ['41', true, 1],
    ['ppd2026', true, 2],
    ['storno', true, 1],
  ])('hľadanie „%s“ (stornované %s)', (q, all, n) => {
    const list = [
      { id: 1, date: '2026-10-12', number: '41', orp: '888', rows: [{ gross: '351' }], inNumber: 'PPD20260001', outNumber: 'VPD20260001' },
      { id: 2, date: '2026-10-13', number: '42', orp: '888', rows: [{ gross: '10' }], inNumber: 'PPD20260002' },
      { id: 3, date: '2026-10-11', number: '40', orp: '888', rows: [], cancelled: true },
    ]
    expect(filterClosings(list, q, all)).toHaveLength(n)
  })
  it('súčet obratu', () => {
    expect(closingTotal({ rows: [{ gross: '123.00' }, { gross: '105.00' }] })).toBe(228)
  })
})
