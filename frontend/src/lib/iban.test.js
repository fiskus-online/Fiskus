import { describe, expect, it } from 'vitest'
import { bankOf, ibanGroups, ibanStatus, validIban } from './iban.js'

describe('IBAN', () => {
  it.each([
    ['slovenský', 'SK3112000000198742637541', true],
    ['s medzerami a malými', 'sk31 1200 0000 1987 4263 7541', true],
    ['preklep', 'SK3112000000198742637542', false],
    ['krátky slovenský', 'SK31120000001987426375', false],
    ['český', 'CZ6508000000192000145399', true],
    ['nemecký', 'DE89370400440532013000', true],
    ['zlý tvar', '1231120000001987426375', false],
    ['prázdny', '', false],
  ])('kontrola: %s', (_, v, want) => expect(validIban(v)).toBe(want))

  it.each([
    ['Tatra banka', 'SK2411000000002612345678', 'Tatra banka'],
    ['neznámy kód banky', 'SK3112000000198742637541', ''],
    ['Slovenská sporiteľňa', 'SK5809000000005028001234', 'Slovenská sporiteľňa'],
    ['ČSOB', 'SK8975000000000012345671', 'Československá obchodná banka (ČSOB)'],
    ['Česká spořitelna', 'CZ6508000000192000145399', 'Česká spořitelna'],
    ['iná krajina', 'DE89370400440532013000', ''],
  ])('banka: %s', (_, v, want) => expect(bankOf(v)).toBe(want))

  it.each([
    ['prázdny', '', { ok: false, bank: '', error: '' }],
    ['ešte sa píše', 'SK31 1200', { ok: false, bank: '', error: '' }],
    ['správny', 'SK2411000000002612345678', { ok: true, bank: 'Tatra banka', error: '' }],
    ['celý, ale zlý', 'SK3112000000198742637542', { ok: false, bank: '', error: 'IBAN nie je správny — nesedí kontrolný súčet alebo dĺžka' }],
    ['bez krajiny', '3112000000198742637541', { ok: false, bank: '', error: 'IBAN začína kódom krajiny a dvoma číslicami, napr. SK31…' }],
  ])('stav pri písaní: %s', (_, v, want) => expect(ibanStatus(v)).toEqual(want))

  it('štvorice', () => expect(ibanGroups('sk3112000000198742637541')).toBe('SK31 1200 0000 1987 4263 7541'))
})
