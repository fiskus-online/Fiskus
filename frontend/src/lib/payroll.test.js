import { describe, expect, it } from 'vitest'
import {
  certificateLines,
  monthGrid,
  settleLines, settleMonthOptions,
  filterSettlements, settlementFromForm, settlementOutcome, settlementToForm,
  postingPairs, payTotal, payDefault, paySettingsForm,
  activeIn, contractLabel, employeeFromForm, employeeName, employeeState, employeeToForm, filterEmployees, inputFromForm, inputToForm, newEmployee,
  oopAllowed, periodLabel, shiftMonth,
} from './payroll.js'

const list = [
  { id: 1, firstName: 'Jana', lastName: 'Nová', contract: 'tpp', insurer: '25', start: '2026-01-01', end: '', position: 'účtovníčka' },
  { id: 2, firstName: 'Peter', lastName: 'Šťastný', contract: 'dovp', insurer: '24', start: '2026-03-01', end: '2026-03-31' },
]

describe('mzdy', () => {
  it('meno a druh', () => {
    expect(employeeName({ title: 'Ing.', firstName: 'Jana', lastName: 'Nová' })).toBe('Ing. Jana Nová')
    expect(contractLabel('dobps')).toBe('Dohoda o brigádnickej práci študentov')
  })

  it.each([
    ['2026-02', [1]],
    ['2026-03', [1, 2]],
    ['2026-04', [1]],
  ])('aktívni v %s', (ym, ids) => {
    expect(list.filter((e) => activeIn(e, ym)).map((e) => e.id)).toEqual(ids)
  })

  it('stav k mesiacu', () => {
    expect(employeeState(list[1], '2026-02').label).toBe('nastúpi')
    expect(employeeState(list[1], '2026-03').label).toBe('pracuje')
    expect(employeeState(list[1], '2026-05').label).toBe('ukončený')
  })

  it.each([
    [{ search: 'stastny' }, [2]],
    [{ search: 'dohoda vykonanie' }, []],
    [{ search: 'dohoda vykonaní' }, [2]],
    [{ search: 'VšZP' }, [1]],
    [{ search: 'uctovnicka' }, [1]],
    [{ all: false, ym: '2026-04' }, [1]],
  ])('hľadanie %o', (f, ids) => expect(filterEmployees(list, f).map((e) => e.id)).toEqual(ids))

  it('výnimka 200 € iba pri DoBPŠ a dôchodcovi na dohode', () => {
    expect(oopAllowed({ contract: 'dobps' })).toBe(true)
    expect(oopAllowed({ contract: 'dovp', pension: 'old' })).toBe(true)
    expect(oopAllowed({ contract: 'tpp', pension: 'old' })).toBe(false)
    expect(oopAllowed({ contract: 'dopc', pension: '' })).toBe(false)
  })

  it('formulár zamestnanca', () => {
    const f = newEmployee(new Date(2026, 2, 15))
    expect(f.start).toBe('2026-03-01')
    const body = employeeFromForm({ ...f, lastName: 'Nová', wage: '1 500,50', contract: 'dovp', regular: false, oop: true, bonus: false, children: [{ name: 'x' }] })
    expect(body).toMatchObject({ wage: '1500.50', hours: '40', regular: false, oop: false, children: [] })
    expect(employeeFromForm({ ...f, contract: 'tpp', regular: false }).regular).toBe(true)
    const back = employeeToForm({ ...body, wage: '1500.50', children: [{ name: 'Ema', rc: '130620/0005' }] })
    expect(back.wage).toBe('1500,50')
    expect(back.children[0].months).toHaveLength(12)
  })

  it('vstupy mesiaca', () => {
    const f = inputToForm({ employeeId: 3, hours: '12.5', extra: '0', sickPay: '40.00' })
    expect(f).toMatchObject({ hours: '12,5', extra: '', sickPay: '40,00' })
    expect(inputFromForm({ ...f, extra: '' })).toMatchObject({ employeeId: 3, hours: '12.5', extra: '0', sickPay: '40.00' })
  })

  it('obdobie', () => {
    expect(periodLabel(2026, 3)).toBe('marec 2026')
    expect(shiftMonth(2026, 1, -1)).toEqual({ year: 2025, month: 12 })
    expect(shiftMonth(2026, 12, 1)).toEqual({ year: 2027, month: 1 })
  })
  it('dvojice zaúčtovania miezd', () => {
    const lines = [
      { account: '521', debit: '1500.00', credit: '0' },
      { account: '331', debit: '0', credit: '1500.00' },
      { account: '331', debit: '34.52', credit: '0' },
      { account: '379', debit: '0', credit: '34.52' },
    ]
    expect(postingPairs(lines)).toEqual([
      { md: '521', d: '331', amount: '1500.00', label: 'Hrubé mzdy' },
      { md: '331', d: '379', amount: '34.52', label: 'Zrážky zo mzdy' },
    ])
    expect(postingPairs()).toEqual([])
  })
  it('úhrady miezd: súčet a predvýber', () => {
    const pays = [
      { key: 'wage:1', amount: '1100.10' },
      { key: 'social', amount: '550.65' },
      { key: 'tax', amount: '-10.00', problem: 'nie je čo odviesť' },
    ]
    expect(payTotal(pays, ['wage:1', 'social'])).toBe('1650.75')
    expect(payTotal(pays, [])).toBe('0.00')
    expect(payDefault(pays)).toEqual(['wage:1', 'social'])
    const f = paySettingsForm({ socialVs: '1234567890', health: { 25: { iban: 'SK1', vs: '9' } } })
    expect(f.socialVs).toBe('1234567890')
    expect(f.health['25']).toEqual({ iban: 'SK1', vs: '9' })
    expect(f.health['24']).toEqual({ iban: '', vs: '' })
    expect(f.payDay).toBe(null)
    expect(paySettingsForm({ payDay: 10 }).payDay).toBe(10)
  })
  it('ročné zúčtovanie: filter, výsledok, formulár', () => {
    const list = [
      { employee: { firstName: 'Jana', lastName: 'Nová', position: 'účtovníčka' }, request: { requested: true } },
      { employee: { firstName: 'Peter', lastName: 'Malý' }, request: {} },
    ]
    expect(filterSettlements(list, { search: 'nova' }).length).toBe(1)
    expect(filterSettlements(list, { requested: true }).length).toBe(1)
    expect(filterSettlements(list).length).toBe(2)
    expect(settlementOutcome('12.50')).toMatch(/nedoplatok/)
    expect(settlementOutcome('-3.00')).toMatch(/preplatok/)
    expect(settlementOutcome('0.00')).toBe('bez rozdielu')
    const f = settlementToForm({ requested: true, dds: '60.00', spouse: { name: 'Eva', months: 6, ownIncome: '100.00' } })
    expect(f.spouse).toBe(true)
    expect(settlementFromForm({ ...f, otherIncome: '1 000,50' })).toMatchObject({
      requested: true, dds: '60.00', spouse: { name: 'Eva', months: 6, ownIncome: '100.00' }, otherIncome: '1000.50', otherPrepaid: '0',
    })
    expect(settlementFromForm({ ...settlementToForm({}), spouse: false }).spouse).toBe(null)
  })
  it('ročné zúčtovanie vo mzde', () => {
    expect(settleLines({ settleTax: '-120.50', settleBonus: '30.00' })).toEqual([
      { label: 'Ročné zúčtovanie — preplatok dane', sign: '+', amount: '120.50' },
      { label: 'Ročné zúčtovanie — doplatok bonusu', sign: '+', amount: '30.00' },
    ])
    expect(settleLines({ settleTax: '5.00', settleBonus: '-2.00' }).map((l) => l.sign)).toEqual(['−', '−'])
    expect(settleLines({ settleTax: '0.00' })).toEqual([])
    const o = settleMonthOptions(2026)
    expect(o.length).toBe(5)
    expect(o[3]).toEqual({ label: 'mzda za marec 2027', value: '2027-03' })
  })
})

describe('potvrdenie o príjmoch', () => {
  it.each([
    [[], 0],
    [[1, 2, 3], 3],
    [[1, 12], 2],
  ])('mriežka mesiacov %j', (months, marked) => {
    const g = monthGrid(months)
    expect(g).toHaveLength(12)
    expect(g.filter(Boolean)).toHaveLength(marked)
    months.forEach((m) => expect(g[m - 1]).toBe(true))
  })
  it('riadky potvrdenia majú kódy zo servera', () => {
    expect(certificateLines.map(([c]) => c)).toEqual(['01', '01a', '02', '02a', '02b', '03', '04', '11'])
  })
})
