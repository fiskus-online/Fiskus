import { describe, expect, it } from 'vitest'
import { activityKind, activityPayload, blankActivity, dueText, sortActivities } from './activities.js'

describe('CRM aktivity', () => {
  it('nový záznam a telo', () => {
    const f = blankActivity({ jobId: 7 }, '2026-11-10')
    expect(f).toEqual({ kind: 'note', text: '', date: '2026-11-10', due: '', contactId: 0, jobId: 7, quoteId: 0 })
    expect(activityPayload({ ...f, text: ' Volal ', due: '2026-11-12' })).toMatchObject({ kind: 'note', text: 'Volal', due: '', done: false, jobId: 7 })
    expect(activityPayload({ ...f, kind: 'task', text: 'Zavolať', due: '2026-11-12' }).due).toBe('2026-11-12')
    expect(activityKind('call').label).toBe('Telefonát')
    expect(activityKind('x').value).toBe('note')
  })
  it.each([
    ['2026-11-10', 'dnes'],
    ['2026-11-11', 'zajtra'],
    ['2026-11-13', 'o 3 dni'],
    ['2026-11-20', 'o 10 dní'],
    ['2026-11-09', 'včera'],
    ['2026-11-06', 'pred 4 dňami'],
    ['', ''],
  ])('termín %s', (due, want) => expect(dueText(due, '2026-11-10')).toBe(want))
  it('poradie: otvorené úlohy podľa termínu, potom história', () => {
    const l = [
      { id: 1, kind: 'call', date: '2026-11-05' },
      { id: 2, kind: 'task', date: '2026-11-07', due: '2026-11-14' },
      { id: 3, kind: 'task', date: '2026-11-08', due: '2026-11-09' },
      { id: 4, kind: 'task', date: '2026-11-09', due: '2026-11-01', done: true },
      { id: 5, kind: 'note', date: '2026-11-10' },
    ]
    expect(sortActivities(l).map((a) => a.id)).toEqual([3, 2, 5, 4, 1])
  })
})
