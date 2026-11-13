import { describe, expect, it } from 'vitest'
import {
  budgetTone, budgetWidth, receiptFacts, receiptRows, listTotals, setAllCategory, splitByCategory, filterObligations, fromAmount, newObligation, obligationBody, accountBody, obligationKinds, payeeContact, payeeOptions, setPayee, partnerAccounts, paymentRows, pickColumns, obligationCell, defaultObligationColumns, obligationTone, paidPercent, toAmount, categoriesFor, categoryErrors, chartBars, entryCategory, entryType, filterEntries, fromForm, newEntry, toForm, monthsLabel, planMonthLabel, filterPlan, ibanGroups,
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
    expect(t).toMatchObject({ amount: '100', category: '', toAccountId: 2, obligationId: 0 })
    expect(fromForm({ type: 'expense', amount: '150', category: 'Splátky', accountId: 1, obligationId: 4 }).obligationId).toBe(4)
    expect(toForm(rows[0])).toMatchObject({ type: 'expense', amount: '45,20' })
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

  it.each([
    [{ principal: '1000', state: { paid: '250' } }, 25],
    [{ principal: '100', state: { paid: '150' } }, 100],
    [{ principal: '0', state: { paid: '10' } }, null],
  ])('splatené %o → %s %%', (o, want) => expect(paidPercent(o)).toBe(want))

  it.each([
    [{ closed: true, state: {} }, 'cancelled'],
    [{ state: { paidOff: true } }, 'paid'],
    [{ state: { overdue: 2 } }, 'overdue'],
    [{ state: { overdue: 0 } }, 'issued'],
  ])('stav záväzku %o', (o, tone) => expect(obligationTone(o).tone).toBe(tone))

  it('filter záväzkov', () => {
    const list = [
      { id: 1, name: 'Hypotéka', creditor: 'VÚB', kind: 'loan', state: {} },
      { id: 2, name: 'Exekúcia EX 1/2024', creditor: 'JUDr. Kováč', kind: 'execution', state: {} },
      { id: 3, name: 'Splatená', kind: 'debt', state: { paidOff: true } },
      { id: 4, name: 'Zrušená', kind: 'regular', closed: true, state: {} },
    ]
    expect(filterObligations(list).map((o) => o.id)).toEqual([1, 2])
    expect(filterObligations(list, { all: true }).map((o) => o.id)).toEqual([1, 2, 3, 4])
    expect(filterObligations(list, { search: 'kovac' }).map((o) => o.id)).toEqual([2])
    expect(filterObligations(list, { search: 'exekúcia' }).map((o) => o.id)).toEqual([2])
  })

  it('nový záväzok a sumy', () => {
    const o = newObligation('regular', accounts, new Date(2026, 10, 6))
    expect(o).toMatchObject({ kind: 'regular', accountId: 1, firstDue: '2026-11-06', frequency: 'monthly' })
    expect(toAmount('1 234,50')).toBe('1234.50')
    expect(fromAmount('1234.50')).toBe('1234,50')
    expect(fromAmount('0.00')).toBe('')
  })

  it.each([
    ['upravený záväzok z detailu bez stavu, kalendára a platieb',
      { id: 7, kind: 'loan', name: 'Úver', principal: '1 200,50', installment: '100', firstDue: new Date(2026, 0, 15), frequency: 'monthly', months: [1, 2],
        state: { paid: '0' }, schedule: [{ n: 1 }], payments: [{ id: 1 }], companyId: 5 },
      { id: 7, kind: 'loan', name: 'Úver', principal: '1200.50', paidBefore: '0', installment: '100', firstDue: '2026-01-15', frequency: 'monthly', months: [1, 2],
        count: 0, accountId: 0, remindDays: 0 }],
    ['mesiace iba pri mesačnom opakovaní',
      { id: 0, kind: 'regular', name: 'Poistka', installment: '30,00', firstDue: new Date(2026, 5, 1), frequency: 'yearly', months: [6] },
      { id: 0, kind: 'regular', name: 'Poistka', principal: '0', paidBefore: '0', installment: '30.00', firstDue: '2026-06-01', frequency: 'yearly', months: [],
        count: 0, accountId: 0, remindDays: 0 }],
  ])('telo záväzku: %s', (_, form, want) => {
    expect(obligationBody(form)).toEqual(want)
  })

  it.each([
    ['upravený účet zo zoznamu bez zostatku', { id: 3, companyId: 1, name: 'Bežný', kind: 'bank', iban: 'SK31', opening: '1 240,50', closed: false, balance: '9', entries: 33 },
      { id: 3, name: 'Bežný', kind: 'bank', iban: 'SK31', opening: '1240.50', closed: false }],
    ['nový účet bez počiatočného stavu', { id: 0, name: 'Hotovosť', kind: 'cash', iban: '', opening: '', closed: false },
      { id: 0, name: 'Hotovosť', kind: 'cash', iban: '', opening: '0', closed: false }],
  ])('telo účtu: %s', (_, form, want) => {
    expect(accountBody(form)).toEqual(want)
  })

  it.each([
    ['prvý druh vo výbere', () => obligationKinds[0].value, 'regular'],
    ['predvolený druh nového záväzku', () => newObligation().kind, 'regular'],
    ['všetky druhy ostali', () => obligationKinds.map((k) => k.value).sort().join(','), 'debt,execution,fine,lent,loan,regular'],
  ])('druh záväzku: %s', (_, got, want) => {
    expect(got()).toBe(want)
  })

  it('bloček: kategórie položiek, popis, filter a hľadanie', () => {
    const e = { id: 9, date: '2026-10-27', accountId: 2, amount: '-6.29', category: 'Potraviny', payee: 'Lidl', split: [
      { category: 'Potraviny', amount: '-0.60', note: 'ROZOK KLASIK' },
      { category: 'Drogéria a domácnosť', amount: '-4.20', note: 'SAMPON' },
      { category: 'Potraviny', amount: '-1.49', note: 'MLIEKO' },
    ] }
    expect(splitByCategory(e.split)).toEqual([{ category: 'Drogéria a domácnosť', amount: '-4.20' }, { category: 'Potraviny', amount: '-2.09' }])
    expect(entryCategory(e, accounts)).toBe('Potraviny + 1 ďalšia')
    expect(filterEntries([e], { category: 'Drogéria a domácnosť' }, accounts)).toHaveLength(1)
    expect(filterEntries([e], { search: 'sampon' }, accounts)).toHaveLength(1)
    expect(setAllCategory(e.split, 'Iné výdavky').every((x) => x.category === 'Iné výdavky')).toBe(true)
    const back = fromForm({ ...toForm(e), source: 'ekasa:O-1' })
    expect(back.split).toHaveLength(3)
    expect(back).toMatchObject({ amount: '-6.29', source: 'ekasa:O-1' })
  })
})

describe('plán platieb', () => {
  it.each([
    [[9, 10, 11, 12, 1, 2, 3, 4, 5, 6], 'september – jún'],
    [[1, 2, 3], 'január – marec'],
    [[7], 'júl'],
    [[1, 3, 4], 'január, marec – apríl'],
    [[], ''],
    [[1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12], ''],
  ])('mesiace %j → %s', (ms, want) => {
    expect(monthsLabel(ms)).toBe(want)
  })

  it('IBAN po štvoriciach', () => {
    expect(ibanGroups('SK3112000000198742637541')).toBe('SK31 1200 0000 1987 4263 7541')
    expect(ibanGroups('')).toBe('')
  })

  it('názov mesiaca', () => {
    expect(planMonthLabel('2026-11')).toBe('november 2026')
  })

  const plan = {
    overdue: [{ name: 'Exekúcia', kind: 'execution', rest: '120.00', date: '2026-10-20' }],
    months: [
      { month: '2026-11', items: [{ name: 'Obedy – Janko', creditor: 'ZŠ Moyzesova', kind: 'regular', rest: '32.40' }, { name: 'Pokuta', kind: 'fine', rest: '50.00' }, { name: 'Mišo', kind: 'lent', rest: '100.00', income: true }] },
      { month: '2026-12', items: [{ name: 'Obedy – Janko', creditor: 'ZŠ Moyzesova', kind: 'regular', rest: '32.40' }] },
    ],
  }
  it('súčty mesiacov a hľadanie', () => {
    const v = filterPlan(plan)
    expect(v.months[0]).toMatchObject({ pay: '82.40', receive: '100.00' })
    expect(v.total).toEqual({ pay: '114.80', receive: '100.00' })
    expect(v.overdueSum.pay).toBe('120.00')
    expect(v.rows.map((r) => r.month)).toEqual(['po splatnosti', 'november 2026', 'november 2026', 'november 2026', 'december 2026'])
    const s = filterPlan(plan, 'zs moyzes')
    expect(s.total.pay).toBe('64.80')
    expect(s.overdue).toEqual([])
    expect(s.months[1].items).toHaveLength(1)
  })
})

describe('detail bločku', () => {
  const receipt = {
    id: 'O-7DBCDA8A56EE426DBCDA8A56EE426D1A', number: '42', issued: '2026-10-31T10:15:30+01:00', type: 'PD', cashRegister: '88812345678900001',
    unit: 'Námestie SNP 5, 96001 Zvolen', customer: '',
    seller: { name: 'ELEKTRO s.r.o.', ico: '36000001', dic: '2020000001', icDph: 'SK2020000001', street: 'Hlavná 12', city: 'Banská Bystrica', zip: '97401' },
    items: [{ name: 'Kábel', quantity: '2.00', unitPrice: '6.15', price: '12.30', vatRate: 23, type: 'K' }, { name: 'ZĽAVA', quantity: '1.00', unitPrice: '-1.00', price: '-1.00', vatRate: 23, type: 'Z' }],
    vat: [{ rate: 23, base: '9.19', vat: '2.11' }],
  }
  const split = [{ note: 'Kábel', amount: '-12.30', category: 'Domácnosť', quantity: '2' }, { note: 'ZĽAVA', amount: '1.00', category: 'Domácnosť' }]

  it('položky s cenou za kus a DPH', () => {
    const rows = receiptRows(split, receipt)
    expect(rows[0]).toMatchObject({ name: 'Kábel', quantity: '2.00', unitPrice: '6.15', vatRate: '23 %', kind: '', amount: '12.30' })
    expect(receiptRows(split, null)[0].amount).toBe('-12.30')
    expect(rows[1].kind).toBe('zľava')
  })

  it('bez údajov bločku alebo s iným počtom položiek', () => {
    expect(receiptRows(split, null)[0]).toMatchObject({ name: 'Kábel', quantity: '2', unitPrice: '' })
    expect(receiptRows([...split, { note: 'Rozdiel', amount: '0.01' }], receipt)[0].unitPrice).toBe('')
  })

  it('údaje dokladu', () => {
    const f = Object.fromEntries(receiptFacts(receipt))
    expect(f).toMatchObject({ Predajca: 'ELEKTRO s.r.o.', Sídlo: 'Hlavná 12, 97401 Banská Bystrica', Predajňa: 'Námestie SNP 5, 96001 Zvolen', 'IČ DPH': 'SK2020000001',
      'Číslo dokladu': '42', 'Druh dokladu': 'pokladničný doklad', 'Kód pokladnice': '88812345678900001' })
    expect(f.Kupujúci).toBeUndefined()
    expect(f.Vystavený).toMatch(/2026/)
    expect(receiptFacts(null)).toEqual([])
  })

  it('formulár ponechá údaje bločku', () => {
    expect(fromForm({ type: 'expense', amount: '12,30', split, receipt }).receipt).toBe(receipt)
  })
})

describe('partneri z adresára', () => {
  const contacts = [
    { id: 1, name: 'Ján Novák', city: 'Brezno' },
    { id: 2, name: 'Západoslovenská energetika' },
    { id: 3, name: 'Mária Malá' },
    { id: 4, name: 'Maria Mala' },
  ]
  it.each([
    ['presné meno', 'Ján Novák', 1],
    ['bez diakritiky a veľkých písmen', ' jan novak ', 1],
    ['dvaja s rovnakým menom', 'Mária Malá', 0],
    ['neznámy', 'Lidl', 0],
    ['prázdny', '', 0],
  ])('kontakt podľa mena: %s', (_, text, want) => {
    expect(payeeContact(text, contacts)).toBe(want)
  })

  it.each([
    ['kontakty pred históriou, bez duplicít', 'n', ['Ján Novák', 'Západoslovenská energetika', 'Lidl Brezno'], [1, 2, 0]],
    ['hľadanie bez diakritiky', 'zapado', ['Ján Novák', 'Západoslovenská energetika'], [2]],
    ['história mimo adresára bez duplicít', 'lidl', ['Lidl Brezno', 'Lidl'], [0, 0]],
  ])('návrhy: %s', (_, q, labels, ids) => {
    const payees = ['Ján Novák', 'Lidl Brezno', 'Lidl', 'LIDL']
    const got = payeeOptions(q, contacts, payees)
    expect(got.map((o) => o.contactId)).toEqual(ids)
    expect(got.every((o) => labels.includes(o.label))).toBe(true)
  })

  it.each([
    ['výber z návrhov', { payee: '', contactId: 0 }, { label: 'Ján Novák', contactId: 1 }, { payee: 'Ján Novák', contactId: 1 }],
    ['napísané meno z adresára', { payee: '', contactId: 0 }, 'jan novak', { payee: 'jan novak', contactId: 1 }],
    ['iný text zruší väzbu', { payee: 'Ján Novák', contactId: 1 }, 'Lidl', { payee: 'Lidl', contactId: 0 }],
    ['nezmenený text nechá väzbu z banky', { payee: 'ZSE ENERGIA', contactId: 2 }, 'ZSE ENERGIA', { payee: 'ZSE ENERGIA', contactId: 2 }],
  ])('zmena príjemcu: %s', (_, form, value, want) => {
    expect(setPayee({ ...form }, value, contacts)).toEqual(want)
  })

  it('pohyb a záväzok posielajú kontakt', () => {
    expect(fromForm({ type: 'expense', amount: '5', payee: 'Ján Novák', contactId: 1 }).contactId).toBe(1)
    expect(fromForm({ type: 'expense', amount: '5', payee: 'Lidl' }).contactId).toBe(0)
    expect(obligationBody({ kind: 'regular', creditor: 'Ján Novák', contactId: 1, firstDue: new Date(2026, 0, 1) }).contactId).toBe(1)
  })
})

describe('účty partnera', () => {
  it.each([
    ['bez kontaktu', null, []],
    ['iba hlavný', { iban: 'SK31 1200 0000 1987 4263 7541' }, ['hlavný účet']],
    ['hlavný a ďalšie, bez duplicít a prázdnych', { iban: 'SK8256000000001234560001', accounts: [
      { iban: 'SK64 5600 0000 0012 3456 1004', label: 'stravné' }, { iban: 'SK8256000000001234560001', label: 'to isté' }, { iban: '' }, { iban: 'SK9783300000002900123456' }] },
    ['hlavný účet', 'stravné', 'účet']],
    ['bez hlavného', { iban: '', accounts: [{ iban: 'SK9783300000002900123456', label: 'rodičovský' }] }, ['rodičovský']],
  ])('%s', (_, contact, labels) => {
    expect(partnerAccounts(contact).map((a) => a.label)).toEqual(labels)
  })
})

describe('platobné údaje splátky', () => {
  const o = { kind: 'regular', name: 'Obedy', creditor: 'ZŠ Moyzesova', payeeName: 'Základná škola, Moyzesova 18', iban: 'sk64 5600 0000 0012 3456 1004',
    vs: '20261', ss: '', message: 'stravné Janko 4.B', address: 'Moyzesova 18, 974 01 Banská Bystrica' }
  it.each([
    ['všetko v poradí banky', o, '1032.4', [
      ['payee', 'Základná škola, Moyzesova 18', 'Základná škola, Moyzesova 18'],
      ['iban', 'SK64 5600 0000 0012 3456 1004', 'SK6456000000001234561004'],
      ['amount', '1032,40', '1032,40'],
      ['vs', '20261', '20261'],
      ['message', 'stravné Janko 4.B', 'stravné Janko 4.B'],
      ['address', 'Moyzesova 18, 974 01 Banská Bystrica', 'Moyzesova 18, 974 01 Banská Bystrica'],
    ]],
    ['bez mena v banke → veriteľ, bez sumy', { kind: 'loan', creditor: 'Home Credit', vs: '4471023' }, '0', [
      ['payee', 'Home Credit', 'Home Credit'], ['vs', '4471023', '4471023'],
    ]],
    ['požičané (vracajú nám)', { kind: 'lent', creditor: 'Michal', iban: 'SK64...' }, '100', []],
  ])('%s', (_, ob, amount, want) => {
    expect(paymentRows(ob, amount).map((r) => [r.key, r.value, r.copy])).toEqual(want)
  })
})

describe('stĺpce zoznamu záväzkov', () => {
  it.each([
    ['nič uložené', null, defaultObligationColumns],
    ['prázdny výber', [], defaultObligationColumns],
    ['poradie podľa definície, neznáme preč', ['next', 'x', 'category'], ['category', 'next']],
    ['predvolené bez druhu', undefined, ['creditor', 'category', 'installment', 'rest', 'next']],
    ['nie pole', 'kind', defaultObligationColumns],
  ])('výber: %s', (_, saved, want) => expect(pickColumns(saved)).toEqual(want))

  const o = { kind: 'regular', creditor: 'ZŠ', category: 'Deti a škola', installment: '32.40', frequency: 'monthly', months: [9, 10, 11, 12, 1, 2, 3, 4, 5, 6],
    accountId: 2, vs: '20261', iban: 'SK6456000000001234561004' }
  it.each([
    ['kind', 'Pravidelná platba'],
    ['category', 'Deti a škola'],
    ['installment', '32,40 € mesačne'],
    ['months', 'september – jún'],
    ['account', 'Bežný účet'],
    ['iban', 'SK64 5600 0000 0012 3456 1004'],
    ['rest', ''],
  ])('bunka %s', (key, want) => {
    expect(obligationCell(o, key, [{ id: 2, name: 'Bežný účet' }]).replace(/\u00a0/g, ' ')).toBe(want)
  })
})
