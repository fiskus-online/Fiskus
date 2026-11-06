import { describe, expect, it } from 'vitest'
import { fitSize, receiptId } from './receipt.js'

describe('bloček z mobilu', () => {
  it.each([
    [4000, 3000, 1600, { w: 1600, h: 1200 }],
    [3000, 4000, 1000, { w: 750, h: 1000 }],
    [800, 600, 1600, { w: 800, h: 600 }],
  ])('fitSize(%i, %i, %i)', (w, h, max, want) => expect(fitSize(w, h, max)).toEqual(want))

  it.each([
    ['O-7DBCDA8A56EE426DBCDA8A56EE426D1A', 'O-7DBCDA8A56EE426DBCDA8A56EE426D1A'],
    ['x o-7dbcda8a56ee426dbcda8a56ee426d1a y', 'O-7DBCDA8A56EE426DBCDA8A56EE426D1A'],
    ['OKP:1:2', ''],
    ['', ''],
  ])('receiptId(%s)', (t, want) => expect(receiptId(t)).toBe(want))
})
