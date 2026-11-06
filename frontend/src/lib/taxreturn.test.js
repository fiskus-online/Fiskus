import { expect, it } from 'vitest'
import { allYear, businessRate, lossRows, lossesOut, rcHint, visibleRows, visibleRows as vis } from './taxreturn.js'

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

it('rodina: riadky bonusu a strát sa ukážu, iba keď sú', () => {
  const codes = (rows) => vis(rows).map((r) => r.code)
  expect(codes({ '116': '100' })).not.toContain('117')
  expect(codes({ '116': '100', '117': '1200', '118': '-1100', '121': '1100' })).toEqual(expect.arrayContaining(['117', '118', '121']))
  expect(codes({ '45': '1000' })).not.toContain('55')
  expect(codes({ '45': '1000', '54': '500', '55': '500' })).toEqual(expect.arrayContaining(['54', '55']))
  expect(codes({ '74': '5260.61' })).toContain('74')
})

it('straty za 5 rokov a rodné číslo', () => {
  const rows = lossRows(2025, [{ year: 2023, amount: '3000.00', used: '0' }])
  expect(rows.map((r) => r.year)).toEqual([2020, 2021, 2022, 2023, 2024])
  expect(rows[3]).toMatchObject({ amount: 3000, used: null })
  expect(lossesOut(rows)).toEqual([{ year: 2023, amount: '3000', used: '0' }])
  expect(allYear(Array(12).fill(false))).toBe(true)
  expect(allYear([false, true])).toBe(false)
  expect(rcHint('130620/0005')).toBe('')
  expect(rcHint('130620/0006')).toMatch(/kontrolný/)
  expect(rcHint('131320/0005')).toMatch(/mesiac/)
  expect(rcHint('12')).toMatch(/9 alebo 10/)
  expect(rcHint('')).toBe('')
})
