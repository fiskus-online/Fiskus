import { describe, expect, it } from 'vitest'
import { blankRows, detailRows, printSigns, printTitle } from './jobPrint.js'

describe('tlač zákazky', () => {
  it.each([
    ['auto', 'Zákazkový list'],
    ['install', 'Montážny protokol'],
    ['service', 'Servisný list'],
    ['', 'Zákazkový list'],
  ])('názov %s', (k, want) => expect(printTitle(k)).toBe(want))

  it('vozidlo bez prázdnych údajov', () => {
    const rows = detailRows({ kind: 'auto', vehicle: { plate: 'BB482KE', vin: '', make: 'Škoda', model: 'Octavia', year: 2019, mileage: 154300, fault: 'Brzdy' } })
    expect(rows.map((r) => r.label)).toEqual(['EČV', 'Vozidlo', 'Rok výroby', 'Stav tachometra pri príjme', 'Závada / požiadavka zákazníka'])
    expect(rows[3].value).toMatch(/^154\s300 km$/)
    expect(rows[4].wide).toBe(true)
  })

  it('zariadenie so zárukou a bez nej', () => {
    const w = detailRows({ kind: 'service', device: { type: 'Menič', warranty: true, purchased: '2026-01-15' } })
    expect(w.find((r) => r.label === 'Záruka').value).toBe('áno, kúpené 15. 1. 2026')
    expect(detailRows({ kind: 'service', device: { type: 'Menič' } }).find((r) => r.label === 'Záruka').value).toBe('nie, platená oprava')
    expect(detailRows({ kind: '', vehicle: { plate: 'X' } })).toEqual([])
  })

  it('podpisy a prázdne riadky', () => {
    expect(printSigns('auto').signs).toHaveLength(3)
    expect(printSigns('install').remarks).toBe(true)
    expect(printSigns('').statement).toBe('')
    expect(blankRows({ work: [], material: [] })).toBe(6)
    expect(blankRows({ work: [{}] })).toBe(0)
  })
})
