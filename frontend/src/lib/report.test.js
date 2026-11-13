import { describe, expect, it } from 'vitest'
import { cleanSettings, defaultSettings, printText, reportCSS, reportHTML, reportModel, reportSheet } from './report.js'
import { xlsxCell } from './export.js'

const columns = [
  { header: 'Číslo', value: (r) => r.no },
  { header: 'Odberateľ', value: (r) => r.who },
  { header: 'Dátum', value: (r) => r.date, type: 'date' },
  { header: 'Suma €', value: (r) => r.sum, type: 'money' },
]
const rows = [
  { no: 'FA1', who: 'Alfa', date: '2026-11-01', sum: '1234.50' },
  { no: 'FA2', who: 'Beta <b>', date: '2026-11-02', sum: '-34.50' },
  { no: 'FA3', who: 'Alfa', date: '', sum: '0.10' },
]

describe('nastavenie zostavy', () => {
  it.each([
    ['nič uložené', null, defaultSettings('Faktúry', columns)],
    ['neznáme stĺpce a zlá orientácia', { columns: ['X', 'Suma €', 'Číslo', 'Suma €'], orientation: 'boky', groupBy: 'Y', totals: false, title: ' ' },
      { title: 'Faktúry', columns: ['Suma €', 'Číslo'], orientation: 'portrait', groupBy: '', totals: false, format: 'pdf' }],
    ['platné', { columns: ['Odberateľ'], orientation: 'landscape', groupBy: 'Odberateľ', title: 'Podľa odberateľa', format: 'xlsx' },
      { title: 'Podľa odberateľa', columns: ['Odberateľ'], orientation: 'landscape', groupBy: 'Odberateľ', totals: true, format: 'xlsx' }],
  ])('%s', (_, saved, want) => expect(cleanSettings(saved, 'Faktúry', columns)).toEqual(want))

  it('veľa stĺpcov → na šírku, prvé tri, dva dátumy a štyri sumy', () => {
    const many = [
      ...['Číslo', 'Druh', 'Odberateľ', 'IČO', 'DIČ'].map((h) => ({ header: h, value: () => '' })),
      ...['Vystavená', 'Dodanie', 'Splatná'].map((h) => ({ header: h, type: 'date', value: () => '' })),
      ...['Základ', 'DPH', 'Spolu', 'Uhradené', 'Zostáva'].map((h) => ({ header: h, type: 'money', value: () => '' })),
    ]
    const d = defaultSettings('X', many)
    expect(d.orientation).toBe('landscape')
    expect(d.columns).toEqual(['Číslo', 'Druh', 'Odberateľ', 'Vystavená', 'Dodanie', 'Základ', 'DPH', 'Spolu', 'Uhradené'])
  })
})

describe('model zostavy', () => {
  const base = { title: 'Faktúry', columns: ['Číslo', 'Suma €'], orientation: 'portrait', groupBy: '', totals: true }
  it.each([
    ['bez skupín, súčet v centoch', base, [''], { 'Suma €': '1200.10' }],
    ['skupiny podľa odberateľa v poradí výskytu', { ...base, groupBy: 'Odberateľ' }, ['Alfa', 'Beta <b>'], { 'Suma €': '1200.10' }],
    ['bez súčtov', { ...base, totals: false }, [''], null],
    ['bez peňažného stĺpca žiadny súčet', { ...base, columns: ['Číslo'] }, [''], null],
  ])('%s', (_, settings, labels, totals) => {
    const m = reportModel(columns, rows, settings)
    expect(m.groups.map((g) => g.label)).toEqual(labels)
    expect(m.totals).toEqual(totals)
    expect(m.count).toBe(3)
  })

  it('medzisúčty skupín', () => {
    const m = reportModel(columns, rows, { ...base, groupBy: 'Odberateľ' })
    expect(m.groups.map((g) => g.totals['Suma €'])).toEqual(['1234.60', '-34.50'])
  })
})

describe('výstup', () => {
  const m = reportModel(columns, rows, { title: 'Faktúry <2026>', columns: ['Číslo', 'Odberateľ', 'Dátum', 'Suma €'], orientation: 'landscape', groupBy: '', totals: true })
  it('HTML: escapovanie, sumy, dátum a súčet', () => {
    const html = reportHTML(m, { company: 'Demo', printed: '13. 11. 2026' })
    expect(html).toContain('Faktúry &lt;2026&gt;')
    expect(html).toContain('Beta &lt;b&gt;')
    expect(html).not.toContain('<b>')
    expect(html).toContain('1 234,50')
    expect(html).toContain('−34,50')
    expect(html).toContain('1. 11. 2026')
    expect(html).toContain('<tfoot><tr class="tot"><td>Spolu</td>')
    expect(html).toContain('1 200,10')
    expect(html).toContain('3 riadky')
  })
  it.each([
    ['money', '1234.5', '1 234,50'],
    ['date', '2026-01-05', '5. 1. 2026'],
    [undefined, '=SUM(A1)', '=SUM(A1)'],
  ])('text bunky %s', (type, v, want) => expect(printText({ type, value: () => v }, {})).toBe(want))

  it('Excel: názov, hlavička, riadky a súčet ako číslo', () => {
    const data = reportSheet(m, { company: 'Demo', printed: '13. 11. 2026' }, xlsxCell)
    expect(data[0][0].value).toBe('Faktúry <2026>')
    expect(data[3].map((c) => c.value)).toEqual(['Číslo', 'Odberateľ', 'Dátum', 'Suma €'])
    expect(data).toHaveLength(4 + 3 + 1)
    expect(data.at(-1)[3]).toMatchObject({ value: 1200.1, type: Number })
    expect(data.at(-1)[0].value).toBe('Spolu')
  })
})

describe('päta strany', () => {
  it.each([['portrait'], ['landscape']])('%s: logo a názov v jednom obrázku', (o) => {
    const css = reportCSS(o)
    expect(css).toContain(`size: A4 ${o}`)
    const logo = decodeURIComponent(css.match(/@bottom-left \{ content: url\("data:image\/svg\+xml,([^"]+)"\); \}/)[1])
    expect(logo).toContain('>Fiškus</text>')
    expect(logo).toContain('dominant-baseline="central"')
  })
})
