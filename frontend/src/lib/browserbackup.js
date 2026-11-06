// Fiškus v prehliadači: texty o zálohe a uložení údajov (bez výpočtov dokladov).

const fmt = new Intl.DateTimeFormat('sk-SK', { dateStyle: 'long', timeStyle: 'short' })

/** „dnes“, „včera“, „pred 3 dňami“ … */
export function agoText(days) {
  if (days <= 0) return 'dnes'
  if (days === 1) return 'včera'
  return `pred ${days} dňami`
}

/** Veta o poslednej zálohe pre stav z /api/browser/backup. */
export function lastBackupText(st) {
  if (!st?.last) return 'Zatiaľ ste nestiahli žiadnu zálohu.'
  return `Posledná záloha: ${fmt.format(new Date(st.last))} (${agoText(st.days)}).`
}

/** Pruh s pripomienkou zálohy (prázdny = nezobrazovať). */
export function reminderText(st) {
  if (!st?.remind) return ''
  return st.last
    ? `Údaje sú iba v tomto prehliadači a poslednú zálohu ste stiahli ${agoText(st.days)}.`
    : 'Údaje sú iba v tomto prehliadači a zatiaľ nemáte žiadnu zálohu.'
}

/** Kontrola formulára zálohy: heslo aspoň 8 znakov (ako MinBackupPassword) a zhodné potvrdenie. */
export function passwordErrors(password, confirm, min = 8) {
  const e = {}
  if ([...password].length < min) e.password = `Heslo musí mať aspoň ${min} znakov`
  else if (confirm !== password) e.confirm = 'Heslá sa nezhodujú'
  return e
}
