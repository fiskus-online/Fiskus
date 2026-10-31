import { expect, it } from 'vitest'
import { businessRate, visibleRows } from './taxreturn.js'

it.each([
  [{ 41: '18000', 135: '804.99' }, ['41', '135'], ['42', '136', '38', '75']],
  [{ 42: '-2000', 136: '500' }, ['42', '136'], ['41', '135']],
  [{ 41: '1', 38: '17320', 152: '3.5' }, ['38', '152'], []],
])('visibleRows %j', (rows, has, hasNot) => {
  const codes = visibleRows(rows).map((r) => r.code)
  for (const c of has) expect(codes).toContain(c)
  for (const c of hasNot) expect(codes).not.toContain(c)
})

it('businessRate', () => {
  expect(businessRate({ 95: '30000' })).toMatch(/^15 %/)
  expect(businessRate({ 95: '150000' })).toMatch(/progresívne/)
})
