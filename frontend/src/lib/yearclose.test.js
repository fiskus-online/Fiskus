import { describe, expect, it } from 'vitest'
import { closeSummary, closingRows, eventLabel, filterClosing } from './yearclose.js'

it.each([
  [null, ''],
  [{ close: { closed: true, closedAt: '2026-01-15T10:00:00Z', closedBy: 'Ján' } }, 'uzavretý 15. 1. 2026, Ján'],
  [{ canClose: true, close: null, checks: [] }, 'pripravený na uzávierku'],
  [{ canClose: true, close: { closed: false }, checks: [] }, 'otvorený po oprave — pripravený na uzávierku'],
  [{ canClose: false, checks: [{ blocking: true, ok: false }] }, 'chýba 1 krok'],
  [{ canClose: false, checks: [{ blocking: true, ok: false }, { blocking: true, ok: false }, { blocking: false, ok: false }] }, 'chýba 2 kroky'],
])('stav uzávierky %#', (info, want) => {
  expect(closeSummary(info)).toBe(want)
})

it('história', () => {
  expect(eventLabel({ action: 'close' })).toBe('uzavretý')
  expect(eventLabel({ action: 'reopen' })).toBe('otvorený')
})

describe('closingRows', () => {
  const lc = {
    closingDate: '2026-12-31', openingDate: '2027-01-01',
    closing: [{ text: 'Uzavretie výnosových účtov', lines: [{ account: '602', name: 'Tržby z predaja služieb', debit: '400.00', credit: '0' }, { account: '710', name: 'Účet ziskov a strát', debit: '0', credit: '400.00' }] }],
    opening: [{ text: 'Otvorenie súvahových účtov', lines: [{ account: '311', name: 'Odberatelia', debit: '492.00', credit: '0' }] }],
  }
  it('rozloží zápisy na riadky so stranou MD alebo D', () => {
    const rows = closingRows(lc)
    expect(rows).toHaveLength(3)
    expect(rows[0]).toMatchObject({ date: '2026-12-31', first: true, account: '602', debit: '400.00', credit: '' })
    expect(rows[1]).toMatchObject({ first: false, account: '710', debit: '', credit: '400.00' })
    expect(rows[2]).toMatchObject({ part: 'opening', date: '2027-01-01', account: '311' })
    expect(closingRows(null)).toEqual([])
  })
  it('hľadá bez diakritiky a vo viacerých slovách', () => {
    const rows = closingRows(lc)
    expect(filterClosing(rows, 'ziskov 710').map((r) => r.account)).toEqual(['710'])
    expect(filterClosing(rows, 'odberatelia')).toHaveLength(1)
    expect(filterClosing(rows, '')).toHaveLength(3)
    // riadok bez prvého riadku zápisu dostane dátum a text zápisu
    expect(filterClosing(rows, '710')[0]).toMatchObject({ account: '710', first: true })
  })
})
