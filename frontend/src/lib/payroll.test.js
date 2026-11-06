import { describe, expect, it } from 'vitest'
import {
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
})
