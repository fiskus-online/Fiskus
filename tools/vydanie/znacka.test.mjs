import { test } from 'node:test'
import assert from 'node:assert/strict'
import { check } from './znacka.mjs'

const releases = ['v0.9.0', 'V0.10.0', 'v0.10.1', 'v0.8.0']

test('kontrola značky', () => {
  const cases = [
    ['ďalšia oprava', 'v0.10.2', releases, ''],
    ['nová vedľajšia', 'v0.11.0', releases, ''],
    ['prvé vydanie', 'v0.1.0', [], ''],
    ['nižšia (0.1.2 < 0.10.1)', 'v0.1.2', releases, 'nie je vyššia'],
    ['opakované spustenie', 'v0.10.1', releases, ''],
    ['nižšia než iná rovnaká verzia', 'v0.10.0', ['V0.10.0', 'v0.10.1'], 'nie je vyššia'],
    ['nižšia než veľké V', 'v0.10.0', ['V0.10.0'], 'nie je vyššia'],
    ['nula na začiatku', 'v0.1.02', releases, 'tvar'],
    ['veľké V', 'V0.10.2', releases, 'tvar'],
    ['bez v', '0.10.2', releases, 'tvar'],
    ['dve čísla', 'v0.11', releases, 'tvar'],
  ]
  for (const [name, tag, existing, want] of cases) {
    const got = check(tag, existing)
    if (want === '') assert.equal(got, '', name)
    else assert.match(got, new RegExp(want), name)
  }
})
