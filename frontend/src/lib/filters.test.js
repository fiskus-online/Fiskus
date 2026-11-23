import { describe, expect, it } from 'vitest'
import { anyPicked, inPicked, onlyPicked, picked, withoutAll } from './filters.js'
import { matchesJobView } from './jobs.js'
import { matchesPorderView } from './porders.js'
import { matchesQuoteView } from './quotes.js'
import { filterEmails } from './emails.js'

describe('filtre s viacerými voľbami', () => {
  it.each([
    [null, []],
    [undefined, []],
    ['', []],
    ['all', []],
    [3, [3]],
    [[1, 2], [1, 2]],
  ])('picked(%o)', (v, want) => expect(picked(v)).toEqual(want))

  it('prázdny výber prepustí všetko, inak aspoň jedna voľba', () => {
    expect(inPicked([], 'paid')).toBe(true)
    expect(inPicked(['open', 'ignored'], 'ignored')).toBe(true)
    expect(inPicked(['open'], 'matched')).toBe(false)
    expect(inPicked('open', 'open')).toBe(true)
    expect(anyPicked(['a', 'b'], (p) => p === 'b')).toBe(true)
  })

  it('jediná voľba a voľby bez Všetky', () => {
    expect(onlyPicked([5])).toBe(5)
    expect(onlyPicked([5, 6])).toBe(null)
    expect(onlyPicked(null)).toBe(null)
    expect(withoutAll([{ value: 'all' }, { value: 'x' }])).toEqual([{ value: 'x' }])
  })
})


describe('zobrazenia zoznamov so zaškrtnutím viacerých', () => {
  it('zákazky: bežiace alebo stratené', () => {
    const rows = [{ status: 'active' }, { status: 'done' }, { status: 'lost' }]
    expect(rows.filter((j) => matchesJobView(j, ['open', 'lost'])).map((j) => j.status)).toEqual(['active', 'lost'])
    expect(rows.filter((j) => matchesJobView(j, [])).length).toBe(3)
    expect(rows.filter((j) => matchesJobView(j, 'done')).length).toBe(1) // jedna hodnota ako doteraz
  })
  it('objednávky, ponuky a e-maily', () => {
    expect(matchesPorderView({ status: 'cancelled' }, ['open', 'cancelled'])).toBe(true)
    expect(matchesPorderView({ status: 'received' }, ['open', 'cancelled'])).toBe(false)
    expect(matchesQuoteView({ status: 'issued', expired: true }, ['lost'])).toBe(true)
    expect(filterEmails([{ error: 'x' }, {}], ['failed'], '').length).toBe(1)
    expect(filterEmails([{ error: 'x' }, {}], [], '').length).toBe(2)
  })
})
