import { describe, expect, it } from 'vitest'
import { notesBody, notesForm, notesSections } from './notes.js'

const server = {
  year: 2026,
  basis: 'Účtovná závierka je riadna.',
  defaultPolicies: 'Majetok sa oceňuje obstarávacou cenou.\n\nZásoby váženým priemerom.',
  company: { name: 'Vzorová s.r.o.', address: 'Hlavná 1, 811 01 Bratislava', ico: '12345678' },
  input: { activity: 'Elektroinštalácie', prevApproved: null, liabilities5y: '0' },
  rows: [{ statement: 'aktíva', code: '01', label: 'SPOLU MAJETOK', current: '1000', previous: '0' }],
}
const money = (v) => `${v} €`

describe('notesForm a notesBody', () => {
  it('prázdny dátum a nulová suma sú prázdne polia a späť', () => {
    const f = notesForm(server)
    expect(f).toMatchObject({ activity: 'Elektroinštalácie', prevApproved: '', liabilities5y: '', notGoingConcern: false })
    expect(notesBody(f)).toMatchObject({ activity: 'Elektroinštalácie', prevApproved: null, liabilities5y: '0' })
  })
  it('suma so slovenskou čiarkou, neplatná suma', () => {
    expect(notesBody({ ...notesForm(server), liabilities5y: '12 000,50' }).liabilities5y).toBe('12000.50')
    expect(notesBody({ ...notesForm(server), liabilities5y: 'veľa' })).toBeNull()
  })
})

describe('notesSections', () => {
  it('prázdne články vynechá, zásady sú návrh Fiškusu', () => {
    const s = notesSections(server, money)
    expect(s.map((x) => x.title.split('.')[1])).toEqual([' I', ' III', ' IV'])
    expect(s[0].paragraphs[0]).toBe('Vzorová s.r.o., Hlavná 1, 811 01 Bratislava, IČO 12345678.')
    expect(s[1].paragraphs).toEqual([
      'Účtovná závierka je zostavená za predpokladu, že účtovná jednotka bude nepretržite pokračovať v činnosti.',
      'Majetok sa oceňuje obstarávacou cenou.',
      'Zásoby váženým priemerom.',
    ])
    expect(s[2].table).toBe(true)
  })
  it('zadané texty a čísla', () => {
    const n = { ...server, input: { ...server.input, policies: 'Vlastné zásady.', events: 'Zmena spoločníka.', liabilities5y: '12000', prevApproved: '2026-03-20', employees: '2,5' } }
    const s = notesSections(n, money)
    expect(s[0].paragraphs).toContain('Účtovná závierka za predchádzajúce účtovné obdobie bola schválená 20. 3. 2026.')
    expect(s[0].paragraphs).toContain('Priemerný prepočítaný počet zamestnancov: 2,5.')
    expect(s[1].paragraphs[1]).toBe('Vlastné zásady.')
    expect(s[2].paragraphs[0]).toBe('Záväzky so zostatkovou dobou splatnosti dlhšou ako päť rokov: 12000 €.')
    expect(s.at(-1).title).toMatch(/^Čl. VI\./)
    expect(notesSections(null, money)).toEqual([])
  })
})
