import { describe, expect, it } from 'vitest'
import {
  budgetTone, budgetWidth, listTotals, categoriesFor, categoryErrors, chartBars, entryCategory, entryType, filterEntries, fromForm, newEntry, toForm,
} from './household.js'

const accounts = [
  { id: 1, name: 'Účet VÚB', kind: 'bank' },
  { id: 2, name: 'Peňaženka', kind: 'cash' },
  { id: 3, name: 'Starý účet', kind: 'bank', closed: true },
]
const rows = [
  { id: 1, date: '2026-10-02', accountId: 1, amount: '-45.20', category: 'Potraviny', payee: 'Lidl' },
  { id: 2, date: '2026-10-03', accountId: 1, toAccountId: 2, amount: '100.00', category: '' },
  { id: 3, date: '2026-09-30', accountId: 1, amount: '1800.00', category: 'Mzda', payee: 'Zamestnávateľ', note: 'výplata' },
  { id: 4, date: '2026-10-05', accountId: 2, amount: '-8.00', category: 'Reštaurácie', payee: 'Bistro Čierna' },
]

describe('domácnosť', () => {
  it.each([
    [rows[0], 'expense'],
    [rows[1], 'transfer'],
    [rows[2], 'income'],
  ])('druh pohybu %#', (e, want) => expect(entryType(e)).toBe(want))

  it('formulár: výdavok so záporným znamienkom, prevod bez kategórie', () => {
    expect(fromForm({ type: 'expense', amount: '12,5', category: 'Potraviny', accountId: 1 }).amount).toBe('-12.5')
    expect(fromForm({ type: 'income', amount: '1 800', category: 'Mzda', accountId: 1 }).amount).toBe('1800')
    const t = fromForm({ type: 'transfer', amount: '100', category: 'Potraviny', accountId: 1, toAccountId: 2 })
    expect(t).toMatchObject({ amount: '100', category: '', toAccountId: 2 })
    expect(toForm(rows[0])).toMatchObject({ type: 'expense', amount: '45.20' })
    expect(fromForm(toForm(rows[0])).amount).toBe('-45.20')
  })

  it('nový pohyb: výdavok z hotovosti, príjem na účet, zrušený účet nie', () => {
    const d = new Date(2026, 9, 29)
    expect(newEntry('expense', accounts, d)).toMatchObject({ accountId: 2, date: '2026-10-29', type: 'expense' })
    expect(newEntry('income', accounts, d).accountId).toBe(1)
    expect(newEntry('expense', [accounts[2]], d).accountId).toBe(null)
  })

  it.each([
    [{ search: 'lidl' }, [1]],
    [{ search: 'cierna' }, [4]],
    [{ search: '45,2' }, [1]],
    [{ accountId: 2 }, [2, 4]], // aj prevod na účet
    [{ type: 'income' }, [3]],
    [{ category: 'Potraviny' }, [1]],
    [{ month: 9 }, [3]],
    [{ search: 'peňaženka' }, [2, 4]],
  ])('filter %o', (f, ids) => {
    expect(filterEntries(rows, f, accounts).map((r) => r.id)).toEqual(ids)
  })

  it('popis prevodu', () => {
    expect(entryCategory(rows[1], accounts)).toBe('Prevod → Peňaženka')
  })

  it('kategórie podľa druhu', () => {
    const cats = [{ name: 'Mzda', income: true }, { name: 'Potraviny' }]
    expect(categoriesFor(cats, 'income')).toEqual(['Mzda'])
    expect(categoriesFor(cats, 'expense')).toEqual(['Potraviny'])
  })

  it('graf: výška podľa najvyššieho mesiaca', () => {
    const b = chartBars(['1000.00', '0'], ['500.00', '2000.00'])
    expect(b).toHaveLength(12)
    expect(b[0]).toMatchObject({ month: 'I', income: 50, expense: 25 })
    expect(b[1].expense).toBe(100)
    expect(chartBars([], [])[0].income).toBe(0)
  })

  it.each([
    [{ budget: '400', percent: 112, over: true }, 'over', 100],
    [{ budget: '400', percent: 95, over: false }, 'near', 95],
    [{ budget: '400', percent: 30, over: false }, 'ok', 30],
    [{ budget: '0', percent: 0, over: false }, 'ok', 100],
  ])('rozpočet %o', (b, tone, width) => {
    expect(budgetTone(b)).toBe(tone)
    expect(budgetWidth(b)).toBe(width)
  })

  it('súčty zoznamu bez prevodov', () => {
    expect(listTotals(rows)).toEqual({ income: '1800.00', expense: '53.20', count: 4 })
    expect(listTotals([{ amount: '0.10' }, { amount: '0.20' }]).income).toBe('0.30')
  })

  it('kontrola kategórií', () => {
    expect(categoryErrors([{ name: 'A' }, { name: ' ' }, { name: 'a' }])).toEqual({ 1: 'Zadajte názov', 2: 'Kategória je dvakrát' })
  })
})
