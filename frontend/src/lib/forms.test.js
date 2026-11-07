import { describe, expect, it } from 'vitest'
import { blankValues, bodyFields, byRole, filterDocs, formMeta, fromTemplate, itemsForApi } from './forms.js'

const ppd = {
  key: 'ppd', name: 'Príjmový pokladničný doklad', paper: 'A5', cash: 'in', builtin: true,
  fields: [
    { key: 'issuer', role: 'issuer' }, { key: 'number', role: 'number' }, { key: 'date', role: 'date', type: 'date' },
    { key: 'partner', role: 'partner' }, { key: 'purpose', role: 'purpose' }, { key: 'amount', role: 'amount' }, { key: 'note', role: 'note' },
  ],
}
const dl = { key: 'dodaci-list', name: 'Dodací list', paper: 'A4', items: true, builtin: true, fields: [] }

describe('tlačivá', () => {
  it('nové tlačivo s hlavičkou a dátumom', () => {
    const v = blankValues(ppd, { issuer: 'Firma\nIČO 1', today: '2026-11-07' })
    expect(v.issuer).toBe('Firma\nIČO 1')
    expect(v.date).toBe('2026-11-07')
    expect(v.purpose).toBe('')
  })
  it('zo vzoru: dnešný dátum, bez čísla, kópia položiek', () => {
    const tpl = { values: { issuer: 'F', number: 'PPD1', date: '2026-01-01', purpose: 'Tržba' }, items: [{ desc: 'A' }] }
    const r = fromTemplate(tpl, ppd, { today: '2026-11-07' })
    expect(r.values).toMatchObject({ issuer: 'F', number: '', date: '2026-11-07', purpose: 'Tržba' })
    r.items[0].desc = 'B'
    expect(tpl.items[0].desc).toBe('A')
  })
  it('položky pre API', () => {
    expect(itemsForApi([{ desc: 'X', qty: '2,5', unit: 'm', price: '' }])).toEqual([{ desc: 'X', unit: 'm', qty: '2.5', price: '0' }])
  })
  it('telo listu bez hlavičky a podpisov', () => {
    expect(bodyFields(ppd).map((f) => f.key)).toEqual(['purpose', 'note'])
    expect(byRole(ppd, { partner: 'Ján' }, 'partner')).toBe('Ján')
  })
  it.each([
    ['', false, 2],
    ['novak', false, 1],
    ['ppd2026', false, 1],
    ['dodaci', false, 1],
    ['', true, 1],
  ])('hľadanie „%s“ (vzory %s)', (q, tpl, n) => {
    const docs = [
      { id: 1, formKey: 'ppd', title: 'PPD – Ján Novák', values: { partner: 'Ján Novák' }, cashNumber: 'PPD20260001', updated: '2026-11-07' },
      { id: 2, formKey: 'dodaci-list', title: 'DL 5', values: {}, items: [{ desc: 'Kábel' }], updated: '2026-11-06' },
      { id: 3, formKey: 'ppd', title: 'Vzor tržba', values: {}, template: true },
    ]
    expect(filterDocs(docs, [ppd, dl], q, tpl)).toHaveLength(n)
  })
  it('popis tlačiva', () => {
    expect(formMeta(ppd)).toBe('A5, zápis do pokladne')
    expect(formMeta({ ...dl, builtin: false })).toBe('A4, položky, vlastné')
  })
})
