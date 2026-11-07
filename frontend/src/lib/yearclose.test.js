import { expect, it } from 'vitest'
import { closeSummary, eventLabel } from './yearclose.js'

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
