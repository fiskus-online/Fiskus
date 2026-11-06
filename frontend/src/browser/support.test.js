import { describe, expect, it } from 'vitest'
import { missingFeatures, progressShare, progressText } from './support.js'

const full = {
  WebAssembly: {},
  Worker: function () {},
  navigator: { serviceWorker: {}, storage: { getDirectory() {} }, locks: {} },
}

describe('Fiškus v prehliadači', () => {
  it('zistí, čo prehliadaču chýba', () => {
    expect(missingFeatures(full)).toEqual([])
    expect(missingFeatures({ ...full, navigator: { serviceWorker: {}, locks: {} } })).toEqual(['úložisko súborov (OPFS)'])
    expect(missingFeatures({})).toHaveLength(5)
  })

  it.each([
    [0, 0, 'Spúšťam Fiškus…', null],
    [12 * 1048576, 25 * 1048576, 'Načítavam program… 12 / 25 MB', 0.48],
    [25 * 1048576, 25 * 1048576, 'Otváram databázu…', 1],
    [5 * 1048576, 0, 'Otváram databázu…', null],
  ])('priebeh %i / %i', (loaded, total, text, share) => {
    expect(progressText(loaded, total)).toBe(text)
    expect(progressShare(loaded, total)).toBe(share)
  })
})
