// Tlač zákazky podľa druhu: zákazkový list (autoservis, všeobecná),
// montážny protokol, servisný list. Iba zostavenie textov, sumy sú zo servera.
import { formatDate } from './format.js'

/** Názov tlačiva podľa druhu zákazky. */
export function printTitle(kind) {
  return { install: 'Montážny protokol', service: 'Servisný list' }[kind] ?? 'Zákazkový list'
}

const km = (n) => (n ? `${Number(n).toLocaleString('sk-SK')} km` : '')

/** Údaje druhu ako riadky „popis – hodnota“ (prázdne sa vynechajú, wide = na celú šírku). */
export function detailRows(j) {
  const rows = []
  const add = (label, value, wide = false) => value && rows.push({ label, value: String(value), wide })
  if (j?.kind === 'auto' && j.vehicle) {
    const v = j.vehicle
    add('EČV', v.plate)
    add('VIN', v.vin)
    add('Vozidlo', [v.make, v.model].filter(Boolean).join(' '))
    add('Rok výroby', v.year || '')
    add('Stav tachometra pri príjme', km(v.mileage))
    add('Palivo pri príjme', v.fuel)
    add('Závada / požiadavka zákazníka', v.fault, true)
    add('Poškodenia zistené pri príjme', v.damage, true)
  } else if (j?.kind === 'install' && j.site) {
    const s = j.site
    add('Miesto montáže', s.address, true)
    add('Montážnici', s.workers)
    add('Kontakt na mieste', s.contact)
    add('Rozsah prác', s.scope, true)
  } else if (j?.kind === 'service' && j.device) {
    const d = j.device
    add('Zariadenie', d.type)
    add('Výrobca a typ', [d.make, d.model].filter(Boolean).join(' '))
    add('Výrobné číslo', d.serial)
    add('Záruka', d.warranty ? `áno${d.purchased ? `, kúpené ${formatDate(d.purchased)}` : ''}` : 'nie, platená oprava')
    add('Popis závady', d.fault, true)
    add('Prevzaté príslušenstvo', d.accessories, true)
  }
  return rows
}

/** Text vyhlásenia zákazníka a podpisy podľa druhu. */
export function printSigns(kind) {
  switch (kind) {
    case 'auto':
      return {
        statement: 'Zákazník súhlasí s vykonaním uvedených prác. Práce nad rámec dohody vykonáme až po jeho súhlase.',
        signs: ['Vozidlo odovzdal (zákazník)', 'Vozidlo prevzal (servis)', 'Vozidlo po oprave prevzal (zákazník)'],
      }
    case 'install':
      return {
        statement: 'Zákazník potvrdzuje prevzatie diela v rozsahu uvedenom v protokole. Výhrady:',
        remarks: true,
        signs: ['Za zhotoviteľa', 'Dielo prevzal (zákazník)'],
      }
    case 'service':
      return {
        statement: 'Zákazník súhlasí s diagnostikou a opravou zariadenia. Opravu nad rámec dohody vykonáme až po jeho súhlase.',
        signs: ['Zariadenie odovzdal (zákazník)', 'Zariadenie prevzal (servis)', 'Zariadenie po oprave prevzal (zákazník)'],
      }
  }
  return { statement: '', signs: ['Za zhotoviteľa', 'Zákazník'] }
}

/** Počet prázdnych riadkov na ručné doplnenie prác (pri príjme ešte nič nie je vo výkaze). */
export const blankRows = (j) => ((j?.work?.length ?? 0) + (j?.material?.length ?? 0) ? 0 : 6)
