// Automatická aktualizácia aplikácie: texty a rozhodnutia pre obrazovku
// priebehu a Nastavenia → Aktualizácie. Stav počíta server (internal/update).

/** Prebieha aktualizácia (program sa onedlho reštartuje)? */
export function updating(st) {
  return ['downloading', 'installing', 'restarting'].includes(st?.state)
}

/** Čaká sa ešte na výsledok kontroly po spustení? */
export function pending(st) {
  return !st || st.state === '' || st.state === 'checking'
}

/** Percentá sťahovania (0–100). */
export function percent(st) {
  return Math.round(Math.min(1, Math.max(0, st?.progress || 0)) * 100)
}

/** Nadpis a popis priebehu. */
export function progressText(st) {
  const v = st?.latest ? ` ${st.latest}` : ''
  switch (st?.state) {
    case 'downloading':
      return { title: `Sťahujem Fiškus${v}… ${percent(st)} %`, note: 'Potom sa Fiškus sám zavrie, nainštaluje novú verziu a znova otvorí.' }
    case 'installing':
      return { title: `Inštalujem Fiškus${v}…`, note: 'Fiškus sa o chvíľu sám znova otvorí.' }
    case 'restarting':
      return { title: `Spúšťam Fiškus${v}…`, note: 'Starý Fiškus sa ukončuje a nový sa spúšťa. Okno sa samo znova načíta.' }
    default:
      return { title: '', note: '' }
  }
}

/** Riadok stavu v Nastaveniach. */
export function statusLine(st) {
  if (!st) return ''
  switch (st.state) {
    case 'checking':
      return 'Zisťujem, či je nová verzia…'
    case 'current':
      return `Fiškus ${st.current} je aktuálny.`
    case 'available':
      return `Je k dispozícii Fiškus ${st.latest} (máte ${st.current}).`
    case 'error':
      return `Aktualizácia zlyhala: ${st.error}`
    case '':
      return `Fiškus ${st.current}. Nová verzia sa ešte nezisťovala.`
    default:
      return progressText(st).title
  }
}

/**
 * Nový program už beží? Po reštarte sa porovná verzia z /api/capabilities
 * s verziou pred aktualizáciou.
 */
export function restarted(before, caps) {
  const v = String(caps?.version ?? '').replace(/^v/, '')
  return !!v && v !== String(before ?? '').replace(/^v/, '')
}
