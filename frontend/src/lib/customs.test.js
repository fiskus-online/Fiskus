import { describe, expect, it } from 'vitest'
import { customsForm, customsPayload, importCandidates } from './customs.js'

describe('rozhodnutie colného úradu', () => {
  const list = [
    { id: 1, number: 'PF1', supplier: { name: 'Shenzhen', country: 'CN' }, supplyKind: 'goods', summary: { total: '900' }, currency: '' },
    { id: 2, number: 'PF2', supplier: { name: 'Bosch', country: 'DE' }, supplyKind: 'goods', summary: { total: '10' } },
    { id: 3, number: 'PF3', supplier: { name: 'AWS', country: 'US' }, supplyKind: 'services', summary: { total: '5' } },
    { id: 4, number: 'PF4', supplier: { name: 'Colný úrad', country: 'SK' }, customs: { duty: '1' }, summary: { total: '1' } },
    { id: 5, number: 'PF5', supplier: { name: 'Acme', country: 'US' }, supplyKind: 'goods', summary: { total: '100' }, currency: 'USD' },
  ]
  it('iba tovar z krajín mimo EÚ, bez samotného rozhodnutia', () => {
    expect(importCandidates(list).map((x) => x.value)).toEqual([1, 5])
    expect(importCandidates(list, 5).map((x) => x.value)).toEqual([1])
    expect(importCandidates(list)[1].label).toBe('PF5 – Acme (100,00 USD)')
  })
  it('odoslanie a úprava', () => {
    expect(customsPayload({ duty: '1 036,5', importId: 1 })).toEqual({ duty: '1036.5', importId: 1 })
    expect(customsPayload({ duty: '' })).toEqual({ duty: '0', importId: 0 })
    expect(customsPayload(null)).toBeUndefined()
    expect(customsForm({ duty: '36.00', importId: 1, importNumber: 'PF1' })).toEqual({ duty: '36,00', importId: 1, importNumber: 'PF1' })
    expect(customsForm(null)).toBeNull()
  })
})
