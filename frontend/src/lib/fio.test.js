import { describe, expect, it } from 'vitest'
import { defaultHouseholdAccount, fioError, fioPayload, householdAccountName, importSummary, lastSyncLabel, validToken } from './fio.js'

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

describe('fio v domácnosti', () => {
  const accs = [
    { id: 1, name: 'Hotovosť', kind: 'cash' },
    { id: 2, name: 'Tatra', kind: 'bank', iban: 'SK31 1100 0000 0000 0000 0001' },
    { id: 3, name: 'Fio', kind: 'bank', iban: 'SK2683300000002801234567' },
    { id: 4, name: 'Starý', kind: 'bank', closed: true },
  ]
  it('zhrnutie s bločkami', () => {
    expect(importSummary({ imported: 5, autoMatched: 2, duplicates: 1 }, true)).toBe('5 nových pohybov, 2 spárovaných s bločkami, 1 už načítaných')
  })
  it('účet domácnosti', () => {
    expect(defaultHouseholdAccount(accs)).toBe(2)
    expect(defaultHouseholdAccount(accs, 'sk26 8330 0000 0028 0123 4567')).toBe(3)
    expect(defaultHouseholdAccount([{ id: 1, kind: 'cash' }])).toBe(0)
    expect(householdAccountName({ householdAccountId: 3 }, accs)).toBe('Fio')
    expect(householdAccountName({}, accs)).toBe('')
    expect(fioPayload({ id: 0, label: 'Fio', token: 'abc', householdAccountId: 3 })).toEqual({ id: 0, label: 'Fio', token: 'abc', householdAccountId: 3 })
  })
})
