import { describe, expect, it } from 'vitest'
import { deviceName, pairCode, remaining } from './mobile.js'

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
