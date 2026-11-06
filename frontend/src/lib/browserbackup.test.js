import { describe, expect, it } from 'vitest'
import { agoText, lastBackupText, passwordErrors, reminderText } from './browserbackup.js'

describe('záloha v prehliadači', () => {
  it.each([
    [0, 'dnes'],
    [1, 'včera'],
    [9, 'pred 9 dňami'],
  ])('pred %i dňami', (d, want) => expect(agoText(d)).toBe(want))

  it('texty o poslednej zálohe', () => {
    expect(lastBackupText(null)).toMatch(/žiadnu/)
    expect(lastBackupText({ last: '2026-11-06T10:00:00Z', days: 1 })).toMatch(/^Posledná záloha: .*2026.*\(včera\)\.$/)
    expect(reminderText({ remind: false, last: null })).toBe('')
    expect(reminderText({ remind: true })).toMatch(/zatiaľ nemáte/)
    expect(reminderText({ remind: true, last: '2026-10-01T00:00:00Z', days: 36 })).toMatch(/pred 36 dňami/)
  })

  it.each([
    ['kratke', 'kratke', { password: 'Heslo musí mať aspoň 8 znakov' }],
    ['dlhe-heslo', 'ine-heslo', { confirm: 'Heslá sa nezhodujú' }],
    ['dlhe-heslo', 'dlhe-heslo', {}],
    ['žžžžžžžž', 'žžžžžžžž', {}],
  ])('heslo %s', (p, c, want) => expect(passwordErrors(p, c)).toEqual(want))
})
