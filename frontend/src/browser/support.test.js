import { describe, expect, it } from 'vitest'
import { diagnosis, missingFeatures, progressShare, progressText } from './support.js'

const full = {
  WebAssembly: {},
  DecompressionStream: function () {},
  Worker: function () {},
  navigator: { serviceWorker: {}, storage: { getDirectory() {} }, locks: {} },
}

describe('Fiškus v prehliadači', () => {
  it('zistí, čo prehliadaču chýba', () => {
    expect(missingFeatures(full)).toEqual([])
    expect(missingFeatures({ ...full, navigator: { serviceWorker: {}, locks: {} } })).toEqual(['úložisko súborov (OPFS)'])
    expect(missingFeatures({})).toHaveLength(6)
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

describe('podrobnosti chyby', () => {
  it.each([
    [{ step: 'úložisko databázy', detail: 'UnknownError: WebKit encountered an internal error', storage: 'opfs' }, ['Krok: úložisko databázy', 'Chyba: UnknownError: WebKit encountered an internal error', 'Úložisko: úložisko súborov (OPFS)']],
    [{ message: 'databáza: chyba' }, ['Krok: nezistený', 'Chyba: databáza: chyba', 'Úložisko: nezistené']],
  ])('%#', (info, lines) => {
    const out = diagnosis(info, 'Mozilla/5.0 (iPhone)')
    for (const l of lines) expect(out).toContain(l)
    expect(out).toContain('Prehliadač: Mozilla/5.0 (iPhone)')
  })
})
