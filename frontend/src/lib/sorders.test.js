import { describe, expect, it } from 'vitest'
import { isLate, matchesSalesOrderView, pickDraft, pickPayload, salesOrderPayload, salesOrderStatusOf, shortage } from './sorders.js'

describe('prijaté objednávky', () => {
  it.each([
    ['prijatá', { status: 'open' }, 'Prijatá', 'issued'],
    ['po termíne', { status: 'partial', due: '2026-11-01' }, 'Čiastočne dodaná, po termíne', 'overdue'],
    ['vybavená po termíne nie je omeškaná', { status: 'done', due: '2026-11-01' }, 'Vybavená', 'paid'],
  ])('stav: %s', (_, o, label, tone) => {
    expect(salesOrderStatusOf(o, '2026-11-10')).toEqual({ label, tone })
  })

  it('omeškanie a zobrazenia', () => {
    expect(isLate({ status: 'open', due: '2026-11-10' }, '2026-11-10')).toBe(false)
    expect(isLate({ status: 'open', due: '' }, '2026-11-10')).toBe(false)
    expect(matchesSalesOrderView({ status: 'partial' }, 'active')).toBe(true)
    expect(matchesSalesOrderView({ status: 'done' }, 'active')).toBe(false)
    expect(matchesSalesOrderView({ status: 'cancelled' }, 'cancelled')).toBe(true)
  })

  it('telo na uloženie zachová kľúč riadku', () => {
    const p = salesOrderPayload({ customer: { name: ' Agro ' }, ref: ' OBJ ', lines: [{ key: 3, description: ' Kábel ', quantity: '1,5', unitPrice: '', vatRate: '23' }] })
    expect(p.customer.name).toBe('Agro')
    expect(p.ref).toBe('OBJ')
    expect(p.lines[0]).toEqual({ key: 3, itemId: 0, description: 'Kábel', quantity: '1.5', unit: 'ks', unitPrice: '0', vatRate: 23 })
  })

  it('výber na dodanie', () => {
    const o = { lines: [{ key: 1, remaining: '25.00' }, { key: 2, remaining: '0.00' }, { key: 3, remaining: '1.50' }] }
    expect(pickDraft(o)).toEqual({ 1: '25', 3: '1.5' })
    expect(pickPayload('delivery', { 1: '10,5', 2: '0', 3: '' })).toEqual({ to: 'delivery', qty: { 1: '10.5' } })
  })

  it('chýba na sklade', () => {
    expect(shortage({ remaining: '5' })).toBe(0)
    expect(shortage({ short: '3.00' })).toBe(3)
  })
})
