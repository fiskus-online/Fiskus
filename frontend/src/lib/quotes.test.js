import { describe, expect, it } from 'vitest'
import { matchesQuoteView, partDraft, partPayload, partPercent, quoteActions, quoteStatusOf, validDays } from './quotes.js'

describe('quoteStatusOf', () => {
  it.each([
    [{ status: 'draft' }, 'Koncept', 'draft'],
    [{ status: 'issued' }, 'Odoslaná', 'issued'],
    [{ status: 'issued', expired: true }, 'Po platnosti', 'overdue'],
    [{ status: 'accepted' }, 'Prijatá', 'paid'],
    [{ status: 'rejected' }, 'Odmietnutá', 'cancelled'],
    [{ status: 'invoiced' }, 'Vyfakturovaná', 'paid'],
  ])('%o → %s', (q, label, tone) => {
    expect(quoteStatusOf(q)).toEqual({ label, tone })
  })
})

describe('quoteActions', () => {
  const on = (q) => Object.entries(quoteActions(q)).filter(([, v]) => v).map(([k]) => k).sort()
  it.each([
    [{}, ['edit', 'issue']],
    [{ id: 1, status: 'draft' }, ['copy', 'edit', 'issue']],
    [{ id: 1, status: 'issued' }, ['accept', 'copy', 'invoice', 'reject', 'renumber']],
    [{ id: 1, status: 'accepted' }, ['copy', 'invoice', 'reject', 'renumber']],
    [{ id: 1, status: 'rejected' }, ['accept', 'copy', 'renumber']],
    [{ id: 1, status: 'invoiced' }, ['copy', 'renumber']],
  ])('%o', (q, want) => {
    expect(on(q)).toEqual(want)
  })
})

describe('matchesQuoteView', () => {
  const rows = [
    { id: 1, status: 'draft' },
    { id: 2, status: 'issued' },
    { id: 3, status: 'issued', expired: true },
    { id: 4, status: 'accepted' },
    { id: 5, status: 'rejected' },
    { id: 6, status: 'invoiced' },
  ]
  it.each([
    ['all', [1, 2, 3, 4, 5, 6]],
    ['draft', [1]],
    ['open', [2]],
    ['accepted', [4]],
    ['invoiced', [6]],
    ['lost', [3, 5]],
  ])('%s', (view, ids) => {
    expect(rows.filter((r) => matchesQuoteView(r, view)).map((r) => r.id)).toEqual(ids)
  })
})

it('validDays', () => {
  expect(validDays('2026-10-29', '2026-11-28')).toBe(30)
  expect(validDays('2026-03-01', '2026-03-31')).toBe(30) // zmena času
  expect(validDays('', '2026-01-01')).toBeNull()
})

describe('čiastková fakturácia ponuky', () => {
  const q = { status: 'accepted', lines: [{ quantity: '1.00' }, { quantity: '40.00' }, { quantity: '-1.00' }], billed: [{ invoiceId: 3 }], remaining: ['0.50', '20.00', '-0.50'] }
  it('stav a akcie', () => {
    expect(quoteStatusOf(q)).toEqual({ label: 'Čiastočne vyfakturovaná (1)', tone: 'issued' })
    expect(quoteStatusOf({ ...q, status: 'invoiced' }).label).toBe('Vyfakturovaná')
    expect(quoteActions(q)).toMatchObject({ invoice: true, reject: false })
  })
  it('návrh množstiev', () => {
    expect(partDraft(q)).toEqual(['0.5', '20', '-0.5'])
    expect(partDraft({ lines: [{ quantity: '2.00' }] })).toEqual(['2'])
    expect(partPercent(q, '30')).toEqual(['0.3', '12', '-0.3'])
    expect(partPercent(q, '80')).toEqual(['0.5', '20', '-0.5']) // nie viac ako zostatok
    expect(partPercent(q, '')).toEqual(['', '', ''])
    expect(partPayload(['0,5', '', ' 12 '])).toEqual({ quantity: ['0.5', '0', '12'] })
  })
})
