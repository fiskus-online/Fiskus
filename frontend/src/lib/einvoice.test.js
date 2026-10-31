import { describe, expect, it } from 'vitest'
import { postmanModes, sapiProviderOf, sentText } from './einvoice.js'

describe('postmanModes', () => {
  it.each([
    ['desktop', false, ['', 'sapi', 'folder', 'http']],
    ['server', true, ['', 'sapi', 'folder', 'http']],
    ['server', false, ['', 'sapi', 'http']],
  ])('%s správca=%s', (mode, sa, want) => expect(postmanModes(mode, sa).map((m) => m.value)).toEqual(want))
})

describe('sentText', () => {
  it.each([
    [[], ''],
    [[{ created: '2026-10-29T17:00:00Z', peppolId: '0245:2120000003' }], 'E-faktúra odoslaná 29. 10. 2026 na 0245:2120000003'],
    [[{ created: '2026-10-30T08:00:00Z', error: 'poštár neodpovedá' }], 'E-faktúra neodoslaná 30. 10. 2026: poštár neodpovedá'],
    [[{ created: '2026-10-30T08:00:00Z', error: 'x' }, { created: '2026-10-29T08:00:00Z' }], 'E-faktúra odoslaná 29. 10. 2026'],
  ])('%j', (list, want) => expect(sentText(list)).toBe(want))
})

it.each([
  ['https://epostak.sk/sapi/v1', 'ePošťák'],
  ['https://peppol.verteco.digital/sapi', 'Verteco'],
  ['https://iny.sk/sapi', 'Iný poskytovateľ'],
  ['', 'Iný poskytovateľ'],
])('sapiProviderOf %s', (url, want) => expect(sapiProviderOf(url)).toBe(want))
