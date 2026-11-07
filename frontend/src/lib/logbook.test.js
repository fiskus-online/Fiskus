import { describe, expect, it } from 'vitest'
import { addPlace, chainOdometer, gpsDrafts, gpsToSave, isCoordName, limitedVAT, vatHint, blankTrip, filterFuel, filterTrips, fuelUnit, logbookPosting, methodHint, methodOptions, purposeSuggestions, returnTrip, tripRoute, tripTime, vehicleLabel } from './logbook.js'

const trips = [
  { id: 1, date: '2026-10-01', from: 'Banská Bystrica', to: 'Zvolen', purpose: 'Montáž rozvádzača', kind: 'business', km: 25, driver: 'Ján' },
  { id: 2, date: '2026-10-02', from: 'Banská Bystrica', to: 'Žilina', purpose: 'Obhliadka', kind: 'business', km: 110 },
  { id: 3, date: '2026-10-03', from: 'Zvolen', to: 'Tesco', purpose: '', kind: 'private', km: 8 },
  { id: 4, date: '2026-10-04', from: 'BB', to: 'ZV', purpose: 'Montáž rozvádzača', kind: 'business', km: 25 },
]

describe('kniha jázd', () => {
  it.each([
    ['', 'all', [1, 2, 3, 4]],
    ['zilina', 'all', [2]],
    ['montaz', 'business', [1, 4]],
    ['', 'private', [3]],
    ['110', 'all', [2]],
    ['1. 10. 2026', 'all', [1]],
  ])('hľadanie „%s“ (%s)', (q, kind, want) => expect(filterTrips(trips, q, kind).map((t) => t.id)).toEqual(want))

  it('tankovanie podľa stanice a dokladu', () => {
    const fuel = [{ id: 1, date: '2026-10-01', station: 'Slovnaft Zvolen', docNumber: 'VPD20260007' }, { id: 2, date: '2026-10-05', station: 'OMV' }]
    expect(filterFuel(fuel, 'slovnaft').map((f) => f.id)).toEqual([1])
    expect(filterFuel(fuel, 'vpd2026').map((f) => f.id)).toEqual([1])
  })

  it('popisy', () => {
    expect(tripRoute(trips[0])).toBe('Banská Bystrica – Zvolen')
    expect(tripTime({ depart: '07:30', arrive: '08:10' })).toBe('07:30–08:10')
    expect(tripTime({})).toBe('')
    expect(vehicleLabel({ plate: 'BB123AB', name: 'Octavia', archived: true })).toBe('BB123AB Octavia – archivované')
    expect(fuelUnit('electric')).toBe('kWh')
    expect(methodOptions('private')[1].label).toBe('Paušál 50 % PHM')
    expect(methodOptions('business')[1].label).toBe('Paušál 80 % PHM')
    expect(methodHint({ ownership: 'private', method: 'log' })).toContain('základná náhrada')
  })

  it('účely podľa častosti', () => {
    expect(purposeSuggestions(trips)).toEqual(['Montáž rozvádzača', 'Obhliadka'])
  })

  it('nová a spiatočná jazda', () => {
    const t = blankTrip({ id: 5, driver: 'Ján', lastOdometer: 12040 }, '2026-10-30')
    expect([t.vehicleId, t.odoStart, t.driver, t.kind]).toEqual([5, 12040, 'Ján', 'business'])
    const back = returnTrip({ id: 9, from: 'BB', to: 'ZV', odoStart: 100, odoEnd: 125, km: 25, purpose: 'Montáž' })
    expect([back.id, back.from, back.to, back.odoStart, back.odoEnd, back.purpose]).toEqual([0, 'ZV', 'BB', 125, 150, 'Montáž'])
  })

  it.each([
    [{ posting: '0', posted: '0' }, 'draft', ''],
    [{ posting: '-30.00', posted: '0' }, 'overdue', 'Zaúčtovať do uzávierky'],
    [{ posting: '-30.00', posted: '-25.00' }, 'overdue', 'Zaúčtovať znova'],
    [{ posting: '102.10', posted: '102.10' }, 'paid', 'Zaúčtovať znova'],
  ])('zaúčtovanie %#', (r, tone, action) => {
    const s = logbookPosting(r)
    expect([s.tone, s.action]).toEqual([tone, action])
  })
})

describe('DPH vozidla', () => {
  it.each([
    [{ category: 'N1' }, 'plnej výške'],
    [{ category: 'M1' }, '50 % DPH'],
    [{ category: 'M1', fullVat: true, vatReason: 'exclusive' }, 'oznámenie daňovému úradu'],
    [{ category: 'L3e', fullVat: true, vatReason: 'transport' }, 'výnimky'],
  ])('vysvetlenie %#', (v, part) => expect(vatHint(v)).toContain(part))
  it('obmedzené kategórie', () => {
    expect(['M1', 'N1', 'L3e', 'L1e'].map(limitedVAT)).toEqual([true, false, true, true])
  })
})

describe('import z GPS', () => {
  const preview = {
    trips: [
      { date: '2026-11-03', from: 'Sídlo', to: '48.57670, 19.12630', km: 18, odoStart: 1000, odoEnd: 1018, duplicate: false, fromPoint: [48.7363, 19.1462], toPoint: [48.5767, 19.1263], meters: 17800 },
      { date: '2026-11-03', from: 'Sklad', to: 'Sídlo', km: 18, odoStart: 0, odoEnd: 0, duplicate: true },
      { date: '2026-11-04', from: 'Sídlo', to: 'Zvolen', km: 20, odoStart: 1036, odoEnd: 1056, duplicate: false },
    ],
  }
  it.each([
    ['Sídlo', false],
    ['48.57670, 19.12630', true],
    ['-1.50000, -60.10000', true],
    ['', false],
  ])('isCoordName(%s)', (s, want) => expect(isCoordName(s)).toBe(want))

  it('duplicitné sa nevyberú a tachometer nadväzuje', () => {
    const d = gpsDrafts(preview)
    expect(d.map((x) => x.pick)).toEqual([true, false, true])
    expect(d[2].odoStart).toBe(1018)
    expect(d[2].odoEnd).toBe(1038)
  })
  it('zmena km posunie ďalšie jazdy', () => {
    const d = gpsDrafts(preview)
    d[0].km = 25
    const c = chainOdometer(d)
    expect([c[0].odoEnd, c[2].odoStart, c[2].odoEnd]).toEqual([1025, 1025, 1045])
  })
  it('bez stavu tachometra iba km', () => {
    const c = chainOdometer([{ pick: true, km: 5, odoStart: 0 }, { pick: true, km: 7 }])
    expect(c.map((x) => [x.odoStart, x.odoEnd])).toEqual([[0, 0], [0, 0]])
  })
  it('na uloženie idú iba vybrané bez pomocných polí', () => {
    const t = gpsToSave(gpsDrafts(preview))
    expect(t).toHaveLength(2)
    expect(Object.keys(t[0])).not.toContain('fromPoint')
    expect(Object.keys(t[0])).not.toContain('pick')
  })
  it('addPlace nahradí miesto na rovnakom bode', () => {
    let p = addPlace([], [48.5767, 19.1263], ' Sklad ')
    p = addPlace(p, [48.5767, 19.1263], 'Sklad Zvolen')
    expect(p).toEqual([{ name: 'Sklad Zvolen', lat: 48.5767, lon: 19.1263, radius: 300 }])
    expect(addPlace(p, [1, 2], '  ')).toBe(p)
  })
})
