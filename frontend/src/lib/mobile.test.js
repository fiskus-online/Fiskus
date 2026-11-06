import { describe, expect, it } from 'vitest'
import { ago, deviceName, diagnose, pairCode, remaining } from './mobile.js'

describe('mobil v sieti', () => {
  it.each([
    ['Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)', 'iPhone'],
    ['Mozilla/5.0 (Linux; Android 14; SM-S911B Build/UP1A) AppleWebKit', 'SM-S911B'],
    ['Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36', 'Android'],
    ['Mozilla/5.0 (X11; Linux x86_64)', 'Telefón'],
  ])('meno zariadenia %#', (ua, want) => expect(deviceName(ua)).toBe(want))

  it('zostávajúci čas', () => {
    const now = Date.parse('2026-10-31T17:00:00Z')
    expect(remaining('2026-10-31T17:05:00Z', now)).toBe('5:00')
    expect(remaining('2026-10-31T17:00:09Z', now)).toBe('0:09')
    expect(remaining('2026-10-31T16:59:00Z', now)).toBe('')
  })

  it.each([
    ['?kod=ab3k9z', 'AB3K9Z'],
    ['?kod=AB<script>', 'ABSCRI'],
    ['', ''],
  ])('kód %s', (q, want) => expect(pairCode(q)).toBe(want))
})

import { addressLabel, addressOptions, OTHER } from './mobile.js'

it('addressLabel', () => {
  expect(addressLabel({ ip: '192.168.1.20', name: 'Wi-Fi', primary: true })).toBe('192.168.1.20 — Wi-Fi (hlavná sieť)')
  expect(addressLabel({ ip: '172.24.80.1', name: 'vEthernet (WSL)', virtual: true })).toBe('172.24.80.1 — vEthernet (WSL) (virtuálna — telefón ju nevidí)')
})

it('addressOptions', () => {
  const st = { address: '192.168.0.50', addresses: [{ ip: '192.168.1.20', name: 'Wi-Fi', primary: true }] }
  const v = addressOptions(st).map((o) => o.value)
  expect(v).toEqual(['', '192.168.1.20', '192.168.0.50', OTHER])
  expect(addressOptions(st)[0].label).toBe('Automaticky (192.168.1.20)')
  expect(addressOptions({ addresses: [] }).map((o) => o.value)).toEqual(['', OTHER])
})

describe('diagnostika mobilu', () => {
  const now = Date.parse('2026-11-06T19:00:30Z')
  it.each([
    ['2026-11-06T19:00:18Z', 'pred 12 s'],
    ['2026-11-06T18:57:30Z', 'pred 3 min'],
    ['2026-11-06T17:00:00Z', 'pred 2 h'],
  ])('ago %s', (at, want) => expect(ago(at, now)).toBe(want))

  it('nič neprišlo, firewall blokuje, verejná sieť', () => {
    const d = diagnose({ lastRequest: null }, { supported: true, blocked: true, allowed: false, public: ['Wi-Fi'] }, now)
    expect(d.map((x) => x.tone)).toEqual(['bad', 'bad', 'warn'])
    expect(d[1].fix).toBe(true)
    expect(d[2].text).toContain('Wi-Fi')
  })

  it('telefón sa pripojil, firewall povolený', () => {
    const d = diagnose({ lastRequest: { ip: '192.168.1.35', at: '2026-11-06T19:00:20Z', paired: true } }, { supported: true, allowed: true, public: [] }, now)
    expect(d).toEqual([
      { tone: 'ok', text: 'Telefón sa k počítaču dostane — posledná požiadavka z 192.168.1.35 pred 10 s (spárovaný).' },
      { tone: 'ok', text: 'Brána Windows Firewall Fiškus povoľuje.' },
    ])
  })

  it('mimo Windows iba sieť', () => {
    expect(diagnose({ lastRequest: null }, { supported: false }, now)).toHaveLength(1)
  })
})
