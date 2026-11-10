import { describe, expect, it } from 'vitest'
import { boardColumns, movePayload, pct, blankJob, blankMaterial, blankWork, jobForm, jobKindLabel, jobPayload, kindSearchText, pickItem, jobStatusOf, marginText, matchesJobView, setKind, normalizePlate, vehicleKey, fillFromVehicle, vehicleHistory, dueState } from './jobs.js'

describe('zákazky', () => {
  it.each([
    [{ status: 'active' }, 'V realizácii', 'issued'],
    [{ status: 'active', late: true }, 'V realizácii, po termíne', 'overdue'],
    [{ status: 'lost' }, 'Stratená', 'cancelled'],
    [{ status: 'offer' }, 'Ponuka', 'draft'],
    [{}, 'Dopyt', 'muted'],
  ])('stav %o', (j, label, tone) => expect(jobStatusOf(j)).toEqual({ label, tone }))

  it('zobrazenia', () => {
    const js = [{ status: 'lead' }, { status: 'active', late: true }, { status: 'done' }, { status: 'lost' }]
    expect(js.filter((j) => matchesJobView(j, 'open'))).toHaveLength(2)
    expect(js.filter((j) => matchesJobView(j, 'late'))).toHaveLength(1)
    expect(js.filter((j) => matchesJobView(j, 'done'))).toHaveLength(1)
    expect(js.filter((j) => matchesJobView(j, 'all'))).toHaveLength(4)
  })

  it('formulár', () => {
    expect(blankJob({ id: 3, name: 'Farmol' })).toMatchObject({ contactId: 3, customer: 'Farmol', status: 'lead' })
    expect(jobPayload({ name: ' RS ', budget: '8 690,50', contactId: 0, due: '2026-10-31' })).toEqual({
      name: 'RS', contactId: 0, status: 'lead', start: '', due: '2026-10-31', budget: '8690.50', note: '', rate: '0', work: [], material: [], kind: '' })
    expect(jobPayload({ name: 'x', budget: '' }).budget).toBe('0')
    expect(marginText({ margin: 73 })).toBe('73 %')
    expect(marginText({})).toBe('—')
  })

  it('výkaz práce a materiál', () => {
    const f = jobForm({ id: 5, name: 'RS', status: 'active', budget: '0.00', rate: '35.00',
      work: [{ id: 1, date: '2026-11-02', person: 'Palo', description: 'Montáž', hours: '4.00', rate: '35.00', costRate: '0.00', invoiceId: 9 }],
      material: null })
    expect(f.budget).toBe('')
    expect(f.work[0].costRate).toBe('')
    expect(f.material).toEqual([])
    expect(blankWork(f, '2026-11-10')).toEqual({ id: 0, date: '2026-11-10', person: 'Palo', description: '', hours: '', rate: '35.00', costRate: '' })
    expect(blankWork(blankJob(), '2026-11-10').rate).toBe('')
    const m = pickItem(blankMaterial('2026-11-10'), { id: 4, name: 'Kábel CYKY', unit: 'm', price: '1.60' })
    expect(m).toMatchObject({ itemId: 4, name: 'Kábel CYKY', unit: 'm', unitPrice: '1.60' })
    expect(pickItem(m, null).itemId).toBe(0)
    f.work.push({ ...blankWork(f, '2026-11-10'), description: ' Revízia ', hours: '1,5' })
    f.material.push({ ...m, quantity: '25', unitCost: '' })
    const p = jobPayload(f)
    expect(p.work[1]).toEqual({ id: 0, date: '2026-11-10', person: 'Palo', description: 'Revízia', hours: '1.5', rate: '35.00', costRate: '0' })
    expect(p.material[0]).toEqual({ id: 0, date: '2026-11-10', itemId: 4, name: 'Kábel CYKY', unit: 'm', quantity: '25', unitPrice: '1.60', unitCost: '0' })
    expect(p.rate).toBe('35.00')
  })

  it('nástenka', () => {
    const rows = [{ id: 1, status: 'lead' }, { id: 2, status: 'active' }, { id: 3, status: 'active' }]
    const cols = boardColumns(rows)
    expect(cols.map((c) => c.value)).toEqual(['lead', 'offer', 'active', 'done', 'lost'])
    expect(cols[2].jobs.map((j) => j.id)).toEqual([2, 3])
    const p = movePayload({ id: 2, name: 'RS', status: 'active', budget: '9500.00', rate: '35.00', work: [{ id: 4, date: '2026-11-02', hours: '2.00', rate: '35.00', costRate: '0.00' }] }, 'done')
    expect(p).toMatchObject({ name: 'RS', status: 'done', budget: '9500.00', rate: '35.00' })
    expect(p.work[0]).toMatchObject({ id: 4, hours: '2.00' })
    expect(pct(57)).toBe('57 %')
    expect(pct(null)).toBe('—')
  })
})

describe('druhy zákaziek', () => {
  const ct = { id: 3, name: 'Hron', street: 'Lesná 2', zip: '97401', city: 'Banská Bystrica' }
  it('nová montáž s adresou zákazníka', () => {
    const j = blankJob(ct, 'install')
    expect(j.kind).toBe('install')
    expect(j.site.address).toBe('Lesná 2, 97401 Banská Bystrica')
    expect(j.vehicle).toBeUndefined()
  })
  it('zmena druhu zachová údaje druhu, ku ktorému sa vrátim', () => {
    let f = blankJob(null, 'auto')
    f.vehicle.plate = 'BB123AB'
    f = setKind(f, 'service', ct)
    expect(f.vehicle).toBeUndefined()
    expect(f.device.type).toBe('')
    f = setKind({ ...f, vehicle: { plate: 'BB123AB' } }, 'auto')
    expect(f.vehicle.plate).toBe('BB123AB')
    expect(setKind(f, 'install', ct).site.address).toContain('Lesná 2')
  })
  it('telo na uloženie iba s údajmi druhu', () => {
    const f = jobForm({ id: 1, name: 'Brzdy', status: 'active', kind: 'auto', vehicle: { plate: 'BB123AB', year: 2019, mileage: 0 } })
    expect(f.vehicle.mileage).toBe('')
    f.vehicle.mileage = '154 300'
    f.vehicle.make = ' Škoda '
    const p = jobPayload(f)
    expect(p.kind).toBe('auto')
    expect(p.vehicle).toMatchObject({ plate: 'BB123AB', make: 'Škoda', year: 2019, mileage: 154300 })
    expect(p.site).toBeUndefined()
    const d = jobPayload({ ...blankJob(null, 'service'), name: 'X' })
    expect(d.device).toMatchObject({ warranty: false, purchased: '' })
    expect(jobPayload(blankJob()).vehicle).toBeUndefined()
  })
  it('hľadanie podľa EČV a výrobného čísla, názov druhu', () => {
    expect(kindSearchText({ subject: 'BB123AB Škoda', vehicle: { vin: 'WVW1' } })).toBe('BB123AB Škoda WVW1')
    expect(kindSearchText({ device: { serial: 'X9' } })).toBe('X9')
    expect(jobKindLabel('service')).toBe('Servis zariadenia')
    expect(jobKindLabel(undefined)).toBe('Všeobecná')
  })
})

describe('vozidlá', () => {
  const cards = [{ key: 'BB482KE', vin: 'TMB1', make: 'Škoda', model: 'Octavia', year: 2019, contactId: 7, inspection: '2027-05-01', nextService: '' }]
  it('EČV a kľúč ako na serveri', () => {
    expect(normalizePlate(' bb-482 ke ')).toBe('BB482KE')
    expect(vehicleKey({ plate: '', vin: 'wvw 1' })).toBe('WVW1')
  })
  it('doplnenie známeho vozidla nepremaže vyplnené', () => {
    const f = { contactId: 0, vehicle: { plate: 'bb 482ke', vin: '', make: 'Skoda', model: '', year: '', inspection: '' } }
    const g = fillFromVehicle(f, cards)
    expect(g.vehicle).toMatchObject({ vin: 'TMB1', make: 'Skoda', model: 'Octavia', year: 2019, inspection: '2027-05-01' })
    expect(g.contactId).toBe(7)
    expect(fillFromVehicle({ ...f, contactId: 3 }, cards).contactId).toBe(3)
    expect(fillFromVehicle({ vehicle: { plate: 'ZV1' } }, cards)).toBeNull()
  })
  it('história vozidla bez aktuálnej zákazky', () => {
    const rows = [
      { id: 1, start: '2025-04-10', vehicle: { plate: 'BB482KE' } },
      { id: 2, start: '2026-11-10', vehicle: { plate: 'BB482KE' } },
      { id: 3, created: '2026-01-01', vehicle: { plate: 'ZV1' } },
      { id: 4, created: '2026-02-01' },
    ]
    expect(vehicleHistory(rows, { plate: 'bb482ke' }, 2).map((r) => r.id)).toEqual([1])
    expect(vehicleHistory(rows, { plate: 'BB482KE' }).map((r) => r.id)).toEqual([2, 1])
    expect(vehicleHistory(rows, { plate: '' })).toEqual([])
  })
})

describe('termín STK', () => {
  it.each([
    ['', ''],
    ['2026-11-01', 'late'],
    ['2026-11-10', 'soon'],
    ['2026-12-10', 'soon'],
    ['2026-12-11', ''],
  ])('%s', (d, want) => expect(dueState(d, '2026-11-10')).toBe(want))
})
