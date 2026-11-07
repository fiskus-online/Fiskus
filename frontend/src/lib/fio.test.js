import { describe, expect, it } from 'vitest'
import { fioError, fioPayload, importSummary, lastSyncLabel, validToken } from './fio.js'

describe('fio', () => {
  it.each([
    [{ imported: 3, autoMatched: 1, open: 2, duplicates: 0 }, '3 nových pohybov, 1 automaticky spárovaných, 2 na spárovanie'],
    [{ imported: 0, duplicates: 3 }, '0 nových pohybov, 3 už načítaných'],
  ])('zhrnutie %#', (r, want) => expect(importSummary(r)).toBe(want))

  it.each([
    ['a'.repeat(64), true],
    [' ' + 'A1'.repeat(32) + ' ', true],
    ['krátky', false],
    ['a'.repeat(63) + '-', false],
    ['', false],
  ])('token %s', (t, ok) => expect(validToken(t)).toBe(ok))

  it('uloženie bez tokenu ponechá uložený', () => {
    expect(fioPayload({ id: 2, label: ' Fio ', token: '' })).toEqual({ id: 2, label: 'Fio' })
    expect(fioPayload({ label: 'x', token: 'ab cd' })).toEqual({ id: 0, label: 'x', token: 'abcd' })
  })

  it('chyby polí a stav', () => {
    expect(fioError({ fields: [{ field: 'token', message: 'zlý' }] })).toEqual({ token: 'zlý' })
    expect(fioError(null)).toEqual({})
    expect(lastSyncLabel({ lastSync: '' }, (d) => d)).toBe('ešte nestiahnuté')
    expect(lastSyncLabel({ lastSync: '2026-10-29' }, (d) => d)).toBe('stiahnuté do 2026-10-29')
  })
})
