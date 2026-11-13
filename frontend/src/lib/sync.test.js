import { describe, expect, it } from 'vitest'
import { appleMobile, itemsText, joinFragment, joinName, linkFragment, phoneLine, phoneProblem } from './sync.js'

describe('telefón mimo domu', () => {
  it.each([
    [1, '1 záznam'],
    [3, '3 záznamy'],
    [5, '5 záznamov'],
    [0, '0 záznamov'],
  ])('%i → %s', (n, want) => expect(itemsText(n)).toBe(want))

  it.each([
    ['#pripojit&k=abc&n=PC', '#pripojit&k=abc&n=PC'],
    ['#faktury', ''],
    ['', ''],
    [undefined, ''],
  ])('odkaz %s', (hash, want) => expect(joinFragment(hash)).toBe(want))

  it('pruh na telefóne', () => {
    expect(phoneLine({ desktop: 'Novákovci', version: '', pending: 0 })).toBe('Kópia z počítača „Novákovci“ · čaká sa na prvú kópiu')
    expect(phoneLine({ desktop: 'PC', version: '2026-11-13T11:30:00Z', pending: 2 })).toMatch(/^Kópia z počítača „PC“ · stav k 13\. 11\. .* · 2 záznamy čakajú na prenos do počítača$/)
    expect(phoneLine(null)).toBe('')
  })

  it.each([
    [{ lastError: 'schránka nie je dostupná: Get …' }, /Bez internetu/],
    [{ lastError: 'schránka odmietla prístup — spárujte telefón znova' }, /spárujte/],
    [{}, /^$/],
  ])('problém %#', (p, want) => expect(phoneProblem(p)).toMatch(want))
})

it.each([
  ['#pripojit&k=abc&n=Domácnosť+Novákovci', 'Domácnosť Novákovci'],
  ['pripojit&k=abc', 'Počítač'],
])('meno počítača z odkazu %s', (f, want) => expect(joinName(f)).toBe(want))

it.each([
  ['https://app.fiskus.online/#pripojit&k=abc&n=PC', '#pripojit&k=abc&n=PC'],
  ['  pripojit&k=abc ', '#pripojit&k=abc'],
  ['https://fiskus.online/', ''],
  ['O-1234-5678', ''],
  [null, ''],
])('odkaz z QR kódu %s', (t, want) => expect(linkFragment(t)).toBe(want))

it.each([
  ['Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15', true],
  ['Mozilla/5.0 (Linux; Android 15) Chrome/140', false],
])('iPhone %#', (ua, want) => expect(appleMobile(ua)).toBe(want))
