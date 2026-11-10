import { describe, expect, it } from 'vitest'
import { blankJob, jobPayload, jobStatusOf, marginText, matchesJobView } from './jobs.js'

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
      name: 'RS', contactId: 0, status: 'lead', start: '', due: '2026-10-31', budget: '8690.50', note: '' })
    expect(jobPayload({ name: 'x', budget: '' }).budget).toBe('0')
    expect(marginText({ margin: 73 })).toBe('73 %')
    expect(marginText({})).toBe('—')
  })
})
