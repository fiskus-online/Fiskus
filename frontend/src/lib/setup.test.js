import { describe, expect, it } from 'vitest'
import { afterSetup, bookAllows, bookkeepingLabel, homePath, pathFits, migrationNote, stepErrors, vatAllows } from './setup.js'

describe('úvodné nastavenie', () => {
  it.each([
    [{ book: 'simple' }, 'simple', true],
    [{ book: 'simple' }, 'double', false],
    [{ book: 'double' }, 'double', true],
    [{ book: 'double' }, '', true],
    [{}, 'double', true],
    [{ book: 'household' }, '', false],
    [{ book: 'household' }, 'simple', false],
    [{ book: 'household' }, 'household', true],
    [{}, 'household', false],
    [{ home: true }, 'household', true],
    [{ book: 'simple' }, 'household', false],
  ])('menu %j pri %s → %s', (item, book, want) => expect(bookAllows(item, book)).toBe(want))

  it.each([
    [0, { name: ' ' }, ['name']],
    [0, { name: 'Ján Novák' }, []],
    [1, { vatPayer: true, icdph: 'SK 2120000003' }, []],
    [1, { vatPayer: true, icdph: '2120000003' }, ['icdph']],
    [1, { vatPayer: false }, []],
    [2, { bookkeeping: '' }, ['bookkeeping']],
    [2, { bookkeeping: 'simple' }, []],
  ])('krok %i %j', (step, f, keys) => expect(Object.keys(stepErrors(step, f))).toEqual(keys))

  it('popisy a smerovanie', () => {
    expect(bookkeepingLabel('double')).toBe('Podvojné účtovníctvo')
    expect(bookkeepingLabel('')).toContain('Nezvolené')
    expect(afterSetup('migrate')).toBe('/prenos')
    expect(afterSetup('new')).toBe('/prehlad')
    expect(homePath({ bookkeeping: 'household' })).toBe('/domacnost')
    expect(homePath({ bookkeeping: 'simple' })).toBe('/prehlad')
  })

  it.each([
    ['/domacnost/pohyby', 'household', true],
    ['/audit', 'household', true],
    ['/nastavenia/firma', 'household', true],
    ['/faktury', 'household', false],
    ['/banka', 'household', false],
    ['/domacnost', 'simple', false],
    ['/domacnost', '', false],
    ['/faktury', '', true],
  ])('obrazovka %s pri %s → %s', (path, bk, want) => {
    expect(pathFits(path, bk, (p) => p.startsWith('/nastavenia/'))).toBe(want)
  })

  it('poznámka o prenose', () => {
    expect(migrationNote({})).toBe(null)
    expect(migrationNote({ migratedFrom: 'POHODA', migratedAt: '2026-10-31T12:00:00Z' })).toContain('z programu POHODA (31. 10. 2026)')
  })
})

it.each([
  [{ vat: true }, { vatPayer: true }, true],
  [{ vat: true }, { vatPayer: false }, false],
  [{}, { vatPayer: false }, true],
  [{ vat: true }, null, false],
])('vatAllows %j %j', (item, co, want) => expect(vatAllows(item, co)).toBe(want))
