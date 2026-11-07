// Kalkulačky: všetky výpočty robí server (/api/calc/*), tu sa iba skladá
// požiadavka z formulára a popisujú výsledky.
import { formatAmount, formatMoney, parseNumber, toISODate } from './format.js'
import { matches, searchTerms } from './search.js'

export const calculators = [
  { key: 'vat', label: 'DPH', icon: 'pi-percentage', hint: 'Suma bez DPH na sumu s DPH a naopak.' },
  { key: 'late', label: 'Úrok z omeškania', icon: 'pi-clock', hint: 'Zákonný úrok z neskoro zaplatenej faktúry alebo pôžičky.' },
  { key: 'loan', label: 'Úver a RPMN', icon: 'pi-credit-card', hint: 'Mesačná splátka, celkové náklady, RPMN a splátkový kalendár.' },
  { key: 'savings', label: 'Sporenie', icon: 'pi-wallet', hint: 'Koľko nasporíte pravidelnými vkladmi po zdanení úrokov.' },
  { key: 'wage', label: 'Čistá mzda', icon: 'pi-money-bill', hint: 'Z hrubej mzdy odvody, daň, bonus na deti, čistá mzda a cena práce.' },
  { key: 'fx', label: 'Kurzy mien', icon: 'pi-euro', hint: 'Prepočet referenčným kurzom ECB k dátumu.' },
]

export const debtorOptions = [
  { value: 'business', label: 'Medzi podnikateľmi' },
  { value: 'consumer', label: 'Občan (spotrebiteľ)' },
  { value: 'custom', label: 'Dohodnutá sadzba' },
]

export const contractOptions = [
  { value: 'tpp', label: 'Pracovný pomer' },
  { value: 'dopc', label: 'Dohoda o pracovnej činnosti' },
  { value: 'dovp', label: 'Dohoda o vykonaní práce' },
  { value: 'dobps', label: 'Dohoda – študent' },
]

export const pensionOptions = [
  { value: 'none', label: 'Bez dôchodku' },
  { value: 'old', label: 'Starobný dôchodca' },
  { value: 'disabled', label: 'Invalidný dôchodca (nad 70 %)' },
]

/** Predvolené hodnoty formulárov (today = Date, info z /api/calc). */
export function defaults(today, info = {}) {
  const year = today.getFullYear()
  const due = new Date(today)
  due.setMonth(due.getMonth() - 1)
  const years = info.wageYears ?? [year]
  return {
    vat: { amount: '100', rate: info.vatRates?.[0] ?? 23, gross: false },
    late: { amount: '1000', due, paid: new Date(today), debtor: 'business', rate: '' },
    loan: { amount: '10000', rate: '6', months: 60, fee: '', monthlyFee: '' },
    savings: { initial: '1000', monthly: '100', rate: '3', years: 5, tax: '19' },
    wage: { gross: '1500', year: years.includes(year) ? year : years[years.length - 1], month: today.getMonth() + 1, contract: 'tpp', regular: false, nczd: true, pension: 'none', children: [] },
    fx: { amount: '100', currency: 'CZK', date: new Date(today), toEur: false },
  }
}

const num = (s) => parseNumber(s)
const opt = (s) => num(s) ?? '0'

/** Telo požiadavky; null = formulár ešte nie je vyplnený (nič sa neposiela). */
export function toRequest(key, f) {
  switch (key) {
    case 'vat':
      return num(f.amount) && { amount: num(f.amount), rate: f.rate, gross: !!f.gross }
    case 'late':
      if (!num(f.amount) || !f.due || !f.paid || (f.debtor === 'custom' && !num(f.rate))) return null
      return { amount: num(f.amount), due: toISODate(f.due), paid: toISODate(f.paid), debtor: f.debtor, rate: f.debtor === 'custom' ? num(f.rate) : '0' }
    case 'loan':
      if (!num(f.amount) || !(f.months > 0)) return null
      return { amount: num(f.amount), rate: opt(f.rate), months: f.months, fee: opt(f.fee), monthlyFee: opt(f.monthlyFee) }
    case 'savings':
      if (!(f.years > 0) || (!Number(num(f.initial)) && !Number(num(f.monthly)))) return null
      return { initial: opt(f.initial), monthly: opt(f.monthly), rate: opt(f.rate), months: Math.round(f.years * 12), tax: opt(f.tax) }
    case 'wage':
      return num(f.gross) && { year: f.year, month: f.month, gross: num(f.gross), contract: f.contract, regular: !!f.regular, nczd: !!f.nczd, pension: f.pension === 'none' ? '' : f.pension, children: f.children.map(Number) }
    case 'fx':
      return num(f.amount) && f.currency && { amount: num(f.amount), currency: f.currency, date: toISODate(f.date), toEur: !!f.toEur }
  }
  return null
}

const row = (label, value, extra = {}) => ({ label, value, ...extra })
const pct = (v) => `${formatAmount(v)} %`

/** Riadky výsledku: { label, value, total?, sub? }. Posledný s total je konečná suma. */
export function resultRows(key, r, f = {}) {
  if (!r) return []
  switch (key) {
    case 'vat':
      return [row('Základ dane', formatMoney(r.base)), row(`DPH ${f.rate} %`, formatMoney(r.vat)), row('Suma s DPH', formatMoney(r.total), { total: true })]
    case 'late':
      return [
        row('Omeškanie', `${r.days} ${days(r.days)} od ${dateText(r.from)}`),
        row('Sadzba', Number(r.ecb) || Number(r.margin) ? `${pct(r.rate)} p. a. (ECB ${pct(r.ecb)} + ${formatAmount(r.margin)} p. b.)` : `${pct(r.rate)} p. a.`),
        row('Úrok z omeškania', formatMoney(r.interest)),
        ...(Number(r.fee) ? [row('Paušálna náhrada nákladov', formatMoney(r.fee), { sub: '§ 369c Obchodného zákonníka, za každú faktúru' })] : []),
        row('Spolu s istinou', formatMoney(r.total), { total: true }),
      ]
    case 'loan':
      return [
        row('Mesačná splátka', formatMoney(r.payment), { strong: true }),
        row('Úroky spolu', formatMoney(r.interest)),
        row('Poplatky spolu', formatMoney(r.fees)),
        row('RPMN', pct(r.apr), { sub: 'ročná percentuálna miera nákladov' }),
        row('Celkové náklady úveru', formatMoney(r.cost)),
        row('Spolu zaplatíte', formatMoney(r.total), { total: true }),
      ]
    case 'savings':
      return [
        row('Vklady spolu', formatMoney(r.deposits)),
        row('Úroky pred zdanením', formatMoney(r.interest)),
        row('Daň z úrokov', formatMoney(r.tax)),
        row('Čisté úroky', formatMoney(String(Number(r.interest) - Number(r.tax)))),
        row('Nasporíte', formatMoney(r.balance), { total: true }),
      ]
    case 'wage':
      return [
        row('Hrubá mzda', formatMoney(r.gross)),
        row('Sociálne poistenie', formatMoney(r.socialEmployee), { minus: true }),
        row('Zdravotné poistenie', formatMoney(r.healthEmployee), { minus: true }),
        row('Nezdaniteľná časť', formatMoney(r.nczd), { sub: 'znižuje základ dane' }),
        row('Preddavok na daň', formatMoney(r.tax), { minus: true }),
        ...(Number(r.taxBonus) ? [row('Daňový bonus na deti', formatMoney(r.taxBonus), { plus: true })] : []),
        row('Čistá mzda', formatMoney(r.net), { total: true }),
        row('Odvody zamestnávateľa', formatMoney(r.contribER), { after: true }),
        row('Cena práce', formatMoney(r.cost), { after: true, strong: true }),
      ]
    case 'fx':
      return [
        row('Kurz ECB', `1 € = ${formatAmount4(r.rate)} ${f.currency}`, { sub: `vyhlásený ${dateText(r.rateDate)}` }),
        row(f.toEur ? 'Suma v eurách' : `Suma v ${f.currency}`, f.toEur ? formatMoney(r.result) : formatMoney(r.result, f.currency), { total: true }),
      ]
  }
  return []
}

function formatAmount4(v) {
  return new Intl.NumberFormat('sk-SK', { minimumFractionDigits: 2, maximumFractionDigits: 4 }).format(Number(v))
}
function dateText(iso) {
  if (!iso) return ''
  const [y, m, d] = iso.split('-')
  return `${Number(d)}. ${Number(m)}. ${y}`
}
const days = (n) => (n === 1 ? 'deň' : n >= 2 && n <= 4 ? 'dni' : 'dní')

/** Hľadanie v splátkovom kalendári (číslo splátky, sumy). */
export function filterSchedule(rows, q) {
  const terms = searchTerms(q)
  if (!terms) return rows ?? []
  return (rows ?? []).filter((r) => matches(terms, [`${r.n}.`, String(r.n), formatAmount(r.payment), formatAmount(r.balance), formatAmount(r.interest)]))
}

/** Hľadanie v ročnom prehľade sporenia. */
export function filterSavingsYears(rows, q) {
  const terms = searchTerms(q)
  if (!terms) return rows ?? []
  return (rows ?? []).filter((r) => matches(terms, [`${r.year}. rok`, formatAmount(r.balance), formatAmount(r.deposits)]))
}
