import { describe, expect, it } from 'vitest'
import { candidates, docPath, pickable } from './links.js'

const forms = [{ key: 'dodaci-list', name: 'Dodací list', fields: [{ key: 'number', role: 'number' }, { key: 'partner', role: 'partner' }, { key: 'date', role: 'date' }] }]

describe('prepojenia dokladov', () => {
  it.each([
    [{ kind: 'invoice', id: 3 }, '/faktury/3'],
    [{ kind: 'quote', id: 4 }, '/ponuky/4'],
    [{ kind: 'purchase', id: 5 }, '/prijate/5'],
    [{ kind: 'form', id: 6, formKey: 'dodaci-list' }, '/tlaciva?t=dodaci-list&doc=6'],
    [{ kind: 'x', id: 1 }, ''],
  ])('adresa %o', (d, want) => expect(docPath(d)).toBe(want))

  it('kandidáti z tlačív a faktúr', () => {
    const fs = candidates('form', [
      { id: 1, formKey: 'dodaci-list', values: { number: 'DL-7', partner: 'Farmol s.r.o.\nLevice', date: '2026-10-02' } },
      { id: 2, formKey: 'dodaci-list', template: true, values: {} },
    ], forms)
    expect(fs).toEqual([{ kind: 'form', id: 1, formKey: 'dodaci-list', number: 'DL-7', partner: 'Farmol s.r.o.', date: '2026-10-02', type: 'Dodací list' }])
    const inv = candidates('invoice', [{ id: 9, number: '', customer: { name: 'A' }, issueDate: '2026-10-01', summary: { total: '10.00' } }])
    expect(inv[0]).toMatchObject({ number: 'koncept', draft: true, total: '10.00' })
  })

  it('výber: bez seba a prepojených, hľadanie, najnovšie prvé', () => {
    const list = [
      { kind: 'invoice', id: 1, number: '20260001', partner: 'Farmol', date: '2026-09-01' },
      { kind: 'invoice', id: 2, number: '20260002', partner: 'COLAB', date: '2026-10-01' },
      { kind: 'invoice', id: 3, number: '20260003', partner: 'Farmol Slovakia', date: '2026-10-20' },
    ]
    expect(pickable(list, { self: { kind: 'invoice', id: 2 } }).map((c) => c.id)).toEqual([3, 1])
    expect(pickable(list, { linked: [{ kind: 'invoice', id: 3 }], q: 'farmol' }).map((c) => c.id)).toEqual([1])
    expect(pickable(list, { q: '0002' }).map((c) => c.id)).toEqual([2])
  })
})
