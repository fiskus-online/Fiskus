import { describe, expect, it } from 'vitest'
import { boardColumns, movePayload, pct, blankJob, blankMaterial, blankWork, jobForm, jobPayload, pickItem, jobStatusOf, marginText, matchesJobView } from './jobs.js'

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
      name: 'RS', contactId: 0, status: 'lead', start: '', due: '2026-10-31', budget: '8690.50', note: '', rate: '0', work: [], material: [] })
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
