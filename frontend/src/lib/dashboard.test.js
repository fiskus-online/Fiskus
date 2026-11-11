import { describe, expect, it } from 'vitest'
import { dueText, monthBars, openItemLink, workRows } from './dashboard.js'

describe('prehľad firmy', () => {
  it('graf: výška podľa najväčšej sumy', () => {
    const b = monthBars([{ month: 1, income: '200', expense: '50' }, { month: 2, income: '100', expense: '0' }])
    expect(b[0]).toMatchObject({ income: 100, expense: 25 })
    expect(b[1].income).toBe(50)
  })
  it.each([
    [{ kind: 'invoice', id: 3 }, '/faktury/3'],
    [{ kind: 'credit_note', id: 4 }, '/faktury/4'],
    [{ kind: 'purchase', id: 5 }, '/prijate/5'],
  ])('odkaz %o', (it_, want) => expect(openItemLink(it_)).toBe(want))
  it.each([
    [0, ''],
    [1, 'po splatnosti 1 deň'],
    [3, 'po splatnosti 3 dni'],
    [12, 'po splatnosti 12 dní'],
  ])('splatnosť %i', (d, want) => expect(dueText({ daysOverdue: d })).toBe(want))
  it('rozpracovaná práca iba nenulová', () => {
    const rows = workRows({ drafts: 2, jobs: 1, jobsLate: 1, pipeline: '4200.00', orders: 0, quotes: 5, quotesTotal: '9000', stockLow: 0 })
    expect(rows.map((r) => r.label)).toEqual(['2 koncepty faktúr', '1 bežiaca zákazka', '5 platných ponúk'])
    expect(rows[1]).toMatchObject({ sub: '1 po termíne', late: true, amount: '4200.00' })
    expect(workRows(null)).toEqual([])
  })
})
