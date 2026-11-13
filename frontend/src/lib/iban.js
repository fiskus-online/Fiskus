// IBAN: kontrola (modulo 97 ako party.ValidIBAN na serveri), zápis po
// štvoriciach a banka podľa kódu banky (SK a CZ: znaky 5 – 8).

/** IBAN bez medzier, veľkými písmenami. */
export const normIban = (v) => String(v ?? '').replace(/\s/g, '').toUpperCase()

/** IBAN po štvoriciach: SK31 1200 0000 1987 4263 7541. */
export const ibanGroups = (v) => normIban(v).replace(/(.{4})(?=.)/g, '$1 ')

/** Správny IBAN: tvar, dĺžka (SK 24 znakov) a kontrolný súčet modulo 97. */
export function validIban(v) {
  const s = normIban(v)
  if (s.length < 15 || s.length > 34 || !/^[A-Z]{2}\d{2}[A-Z0-9]+$/.test(s)) return false
  if (s.startsWith('SK') && s.length !== 24) return false
  if (s.startsWith('CZ') && s.length !== 24) return false
  let rem = 0
  for (const ch of s.slice(4) + s.slice(0, 4)) {
    const v = ch >= 'A' ? String(ch.charCodeAt(0) - 55) : ch
    for (const d of v) rem = (rem * 10 + Number(d)) % 97
  }
  return rem === 1
}

// kódy bánk podľa číselníkov NBS a ČNB
const banks = {
  SK: {
    '0200': 'Všeobecná úverová banka (VÚB)',
    '0720': 'Národná banka Slovenska',
    '0900': 'Slovenská sporiteľňa',
    1100: 'Tatra banka',
    1111: 'UniCredit Bank',
    3000: 'Slovenská záručná a rozvojová banka',
    3100: 'Prima banka',
    5200: 'ČSOB (pôvodne OTP Banka)',
    5600: 'Prima banka',
    6500: '365.bank',
    7300: 'ING Bank',
    7500: 'Československá obchodná banka (ČSOB)',
    7930: 'Wüstenrot stavebná sporiteľňa',
    8050: 'Commerzbank',
    8100: 'Komerční banka',
    8120: 'Privatbanka',
    8130: 'Citibank Europe',
    8170: 'ČSOB stavebná sporiteľňa',
    8180: 'Štátna pokladnica',
    8320: 'J&T Banka',
    8330: 'Fio banka',
    8360: 'mBank',
    8420: 'BKS Bank',
    9952: 'TrustPay',
  },
  CZ: {
    '0100': 'Komerční banka',
    '0300': 'ČSOB',
    '0600': 'MONETA Money Bank',
    '0710': 'Česká národní banka',
    '0800': 'Česká spořitelna',
    2010: 'Fio banka',
    2250: 'Banka CREDITAS',
    2700: 'UniCredit Bank',
    3030: 'Air Bank',
    3500: 'ING Bank',
    5500: 'Raiffeisenbank',
    5800: 'J&T Banka',
    6210: 'mBank',
  },
}

/** Banka podľa IBAN (SK, CZ); pri inej krajine alebo neznámom kóde ''. */
export function bankOf(v) {
  const s = normIban(v)
  return banks[s.slice(0, 2)]?.[s.slice(4, 8)] ?? ''
}

/**
 * Stav IBAN pri písaní: { ok, bank, error }. Neúplný IBAN ešte nie je chyba
 * (error sa ukáže až pri plnej dĺžke alebo zlom tvare).
 */
export function ibanStatus(v) {
  const s = normIban(v)
  if (!s) return { ok: false, bank: '', error: '' }
  if (validIban(s)) return { ok: true, bank: bankOf(s), error: '' }
  const full = (s.startsWith('SK') || s.startsWith('CZ') ? 24 : 15)
  if (!/^[A-Z]{0,2}$|^[A-Z]{2}\d{0,2}$|^[A-Z]{2}\d{2}[A-Z0-9]*$/.test(s)) return { ok: false, bank: '', error: 'IBAN začína kódom krajiny a dvoma číslicami, napr. SK31…' }
  if (s.length < full) return { ok: false, bank: '', error: '' }
  return { ok: false, bank: '', error: 'IBAN nie je správny — nesedí kontrolný súčet alebo dĺžka' }
}
