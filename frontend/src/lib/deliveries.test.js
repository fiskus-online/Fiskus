import { describe, expect, it } from 'vitest'
import { blankDelivery, deliveryForm, deliveryLine, deliveryPayload, deliveryStatusOf, invoiceBlocker, matchesDeliveryView } from './deliveries.js'

describe('dodacie listy', () => {
  it('stavy a zobrazenia', () => {
    expect(deliveryStatusOf({ status: 'issued' })).toEqual({ label: 'Vydaný', tone: 'issued' })
    expect(deliveryStatusOf({})).toEqual({ label: 'Koncept', tone: 'draft' })
    const ns = [{ status: 'draft' }, { status: 'issued' }, { status: 'invoiced' }]
    expect(ns.filter((n) => matchesDeliveryView(n, 'issued'))).toHaveLength(1)
    expect(ns.filter((n) => matchesDeliveryView(n, 'all'))).toHaveLength(3)
  })

  it.each([
    [[], 'Vyberte vydané dodacie listy'],
    [[{ status: 'issued', contactId: 4 }, { status: 'issued', contactId: 4 }], ''],
    [[{ status: 'issued', contactId: 4 }, { status: 'invoiced', contactId: 4 }], 'Na faktúru idú iba vydané, nevyfakturované dodacie listy'],
    [[{ status: 'issued', contactId: 4 }, { status: 'issued', contactId: 5 }], 'Vybrané dodacie listy majú rôznych odberateľov'],
    [[{ status: 'issued', customer: { name: 'Agro ' } }, { status: 'issued', customer: { name: 'agro' } }], ''],
    [[{ status: 'issued', contactId: 4, priceMode: 'gross' }, { status: 'issued', contactId: 4 }], 'Vybrané dodacie listy majú ceny s DPH aj bez DPH'],
  ])('výber na faktúru %#', (list, want) => expect(invoiceBlocker(list)).toBe(want))

  it('formulár', () => {
    const b = blankDelivery({ id: 4, name: 'Agro', city: 'Liptovský Mikuláš', ico: '00612341' }, '2026-11-10')
    expect(b).toMatchObject({ contactId: 4, date: '2026-11-10', customer: { name: 'Agro', city: 'Liptovský Mikuláš', country: 'SK', ico: '00612341' } })
    expect(blankDelivery().customer).toEqual({ name: '', country: 'SK' })
    const it = { id: 13, name: 'Kábel', unit: 'm', priceNet: '2.19', priceGross: '2.69', vatRate: 23 }
    expect(deliveryLine(it)).toMatchObject({ itemId: 13, unitPrice: '2.19', vatRate: 23 })
    expect(deliveryLine(it, { priceMode: 'gross' }).unitPrice).toBe('2.69')
    expect(deliveryLine(it, { priceMode: 'gross', nonVat: true })).toMatchObject({ unitPrice: '2.19', vatRate: 0 })
    expect(deliveryLine(null, { rate: 19 }).vatRate).toBe(19)
    const f = deliveryForm({ id: 7, contactId: 4, customer: { name: 'Agro' }, date: '2026-11-10', place: null, lines: [{ ...deliveryLine(it), quantity: '40.00', amount: '87.60' }] })
    f.lines.push({ ...deliveryLine(), description: ' Montáž ', quantity: '2', unitPrice: '35,5', vatRate: '23' })
    expect(deliveryPayload(f)).toEqual({ contactId: 4, customer: { name: 'Agro' }, date: '2026-11-10', place: '', order: '', note: '', receiver: '', priceMode: 'net', lines: [
      { itemId: 13, description: 'Kábel', quantity: '40.00', unit: 'm', unitPrice: '2.19', vatRate: 23 },
      { itemId: 0, description: 'Montáž', quantity: '2', unit: 'ks', unitPrice: '35.5', vatRate: 23 },
    ] })
  })
})
