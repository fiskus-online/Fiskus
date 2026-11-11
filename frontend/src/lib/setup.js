// Úvodné nastavenie firmy (sprievodca) a druh účtovníctva.

export const bookkeepingOptions = [
  {
    value: 'simple',
    label: 'Jednoduché účtovníctvo',
    who: 'Živnostník, ktorý uplatňuje skutočné výdavky alebo vedie jednoduché účtovníctvo',
    what: 'Peňažný denník, výkazy o príjmoch a výdavkoch a o majetku a záväzkoch',
  },
  {
    value: 'double',
    label: 'Podvojné účtovníctvo',
    who: 's.r.o., a.s. a ďalšie právnické osoby, živnostník nad hranicou obratu',
    what: 'Účtový rozvrh, účtovný denník a hlavná kniha, súvaha a výkaz ziskov a strát',
  },
]

/** Popis druhu účtovníctva ('' = nezvolené). */
export function bookkeepingLabel(v) {
  return bookkeepingOptions.find((o) => o.value === v)?.label ?? 'Nezvolené (zobrazuje sa oboje)'
}

/** Patrí položka menu k zvolenému účtovníctvu? Nezvolené = všetko okrem
 * domácnosti. Domácnosť má iba svoje položky a spoločné (home: true). */
export function bookAllows(item, bookkeeping) {
  if (bookkeeping === 'household') return item.book === 'household' || !!item.home
  if (item.book === 'household') return false
  return !item.book || !bookkeeping || item.book === bookkeeping
}

/** Úvodná stránka podľa druhu firmy. */
export const homePath = (company) => (company?.bookkeeping === 'household' ? '/domacnost' : '/prehlad')

/** Položky DPH (vat: true) iba pre platiteľa DPH. */
export function vatAllows(item, company) {
  return !item.vat || !!company?.vatPayer
}

export const setupSteps = ['Firma', 'DPH', 'Účtovníctvo', 'Začiatok']

/** Chyby kroku sprievodcu (prázdny objekt = dá sa pokračovať). */
export function stepErrors(step, f) {
  const e = {}
  if (step === 0 && !String(f.name ?? '').trim()) e.name = 'Zadajte názov firmy alebo meno živnostníka'
  if (step === 1 && f.vatPayer && !/^SK\d{10}$/.test(String(f.icdph ?? '').replace(/\s/g, '').toUpperCase())) e.icdph = 'IČ DPH je SK a 10 číslic'
  if (step === 2 && !f.bookkeeping) e.bookkeeping = 'Vyberte druh účtovníctva'
  return e
}

/** Kam ísť po dokončení sprievodcu. */
export function afterSetup(start) {
  return start === 'migrate' ? '/prenos' : '/prehlad'
}

/** Text o prenose dokladov z iného programu (null = ešte nebol). */
export function migrationNote(company) {
  if (!company?.migratedAt) return null
  const d = new Date(company.migratedAt)
  const when = Number.isNaN(d.getTime()) ? company.migratedAt : `${d.getDate()}. ${d.getMonth() + 1}. ${d.getFullYear()}`
  return `Doklady sa už preniesli z programu ${company.migratedFrom} (${when}). Ďalší prenos dokladov nie je možný — adresár a cenník sa prenášajú kedykoľvek.`
}

/** Patrí obrazovka k druhu firmy? Domácnosť má svoje obrazovky, nastavenia
 * a audit; firma všetko okrem domácnosti. */
export function pathFits(path, bookkeeping, isSettings = () => false) {
  const home = path === '/domacnost' || path.startsWith('/domacnost/')
  if (bookkeeping !== 'household') return !home
  return home || path === '/audit' || path === '/kalendar' || path === '/nastavenia' || isSettings(path)
}
