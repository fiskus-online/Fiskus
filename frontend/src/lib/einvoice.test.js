import { describe, expect, it } from 'vitest'
import { postmanModes, sapiProviderOf, sentText, trackable } from './einvoice.js'

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
    [[{ created: '2027-01-04T08:00:00Z', status: 'delivered', delivered: '2027-01-04T09:00:00Z' }], 'E-faktúra odoslaná 4. 1. 2027 — doručená 4. 1. 2027'],
    [[{ created: '2027-01-04T08:00:00Z', status: 'undeliverable' }], 'E-faktúra odoslaná 4. 1. 2027 — nedoručiteľná (príjemca nie je v sieti Peppol)'],
    [[{ created: '2027-01-04T08:00:00Z', status: 'accepted' }], 'E-faktúra odoslaná 4. 1. 2027'],
  ])('%j', (list, want) => expect(sentText(list)).toBe(want))
})

it.each([
  ['https://epostak.sk/sapi/v1', 'ePošťák'],
  ['https://peppol.verteco.digital/sapi', 'Verteco'],
  ['https://iny.sk/sapi', 'Iný poskytovateľ'],
  ['', 'Iný poskytovateľ'],
])('sapiProviderOf %s', (url, want) => expect(sapiProviderOf(url)).toBe(want))

it.each([
  [{ messageId: 'm', status: 'accepted' }, true],
  [{ messageId: 'm', status: 'delivered' }, false],
  [{ messageId: 'm', status: 'accepted', error: 'x' }, false],
  [{ status: 'accepted' }, false],
  [undefined, false],
])('trackable %j', (e, want) => expect(trackable(e)).toBe(want))
