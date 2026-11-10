import { describe, expect, it } from 'vitest'
import { blankPorder, matchesPorderView, porderForm, porderLine, porderPayload, porderStatusOf, receiveDraft, receivePayload, remaining } from './porders.js'

describe('objednávky dodávateľom', () => {
  it.each([
    [{ status: 'sent' }, '', 'Odoslaná', 'issued'],
    [{ status: 'sent', expected: '2026-11-05' }, '2026-11-10', 'Odoslaná, mešká', 'overdue'],
    [{ status: 'received', expected: '2026-11-05' }, '2026-11-10', 'Prijatá', 'paid'],
    [{}, '', 'Rozpracovaná', 'draft'],
  ])('stav %o', (o, today, label, tone) => expect(porderStatusOf(o, today)).toEqual({ label, tone }))

  it('zobrazenia', () => {
    const os = [{ status: 'draft' }, { status: 'partial' }, { status: 'received' }, { status: 'cancelled' }]
    expect(os.filter((o) => matchesPorderView(o, 'open'))).toHaveLength(2)
    expect(os.filter((o) => matchesPorderView(o, 'received'))).toHaveLength(1)
    expect(os.filter((o) => matchesPorderView(o, 'all'))).toHaveLength(4)
  })

  it('formulár a riadky', () => {
    expect(blankPorder({ id: 2, name: 'Elektro BB' }, '2026-11-10')).toMatchObject({ contactId: 2, supplier: 'Elektro BB', date: '2026-11-10', lines: [] })
    const l = porderLine({ id: 13, name: 'Kábel', unit: 'm', supplierCode: 'CYKY', purchasePrice: '1.35' })
    expect(l).toMatchObject({ itemId: 13, name: 'Kábel', unit: 'm', supplierCode: 'CYKY', unitPrice: '1.35' })
    expect(porderLine({ id: 1, name: 'x', purchasePrice: '0.00' }).unitPrice).toBe('')
    expect(porderLine().itemId).toBe(0)
    const f = porderForm({ id: 7, supplier: 'Elektro BB', date: '2026-11-10', expected: null, lines: [{ ...l, quantity: '100.00', received: '60.00' }] })
    expect(f.expected).toBe('')
    f.lines.push({ ...porderLine(), name: ' Hmoždinky ', quantity: '2', unitPrice: '3,5' })
    expect(porderPayload(f)).toEqual({ contactId: 0, supplier: 'Elektro BB', date: '2026-11-10', expected: '', note: '', lines: [
      { itemId: 13, name: 'Kábel', supplierCode: 'CYKY', unit: 'm', quantity: '100.00', unitPrice: '1.35' },
      { itemId: 0, name: 'Hmoždinky', supplierCode: '', unit: 'ks', quantity: '2', unitPrice: '3.5' },
    ] })
  })

  it('príjem', () => {
    const o = { lines: [{ remaining: '40.00' }, { remaining: '0.00' }, { remaining: '2.50' }] }
    expect(remaining(o.lines[0])).toBe(40)
    expect(receiveDraft(o)).toEqual(['40', '', '2.5'])
    expect(receivePayload('2026-11-12', 'ZV', ['40', '', '1,5'])).toEqual({ date: '2026-11-12', warehouse: 'ZV', quantity: ['40', '0', '1.5'] })
  })
})
