// Kontrola značky vydania: tvar v1.2.3 (malé v, čísla bez núl na začiatku)
// a vyššia než všetky doterajšie vydania. Nainštalované verzie porovnávajú
// vydania podľa čísel, takže v0.1.2 po v0.10.1 by sa nikdy neponúkla.
//
// Použitie: gh release list ... | node tools/vydanie/znacka.mjs v0.10.2

const shape = /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/

/** Čísla verzie zo značky (v aj V, aj staré so začiatočnou nulou), inak null. */
export function parse(tag) {
  const m = /^[vV]?(\d+)\.(\d+)\.(\d+)$/.exec(String(tag).trim())
  return m ? m.slice(1).map(Number) : null
}

function cmp(a, b) {
  for (let i = 0; i < 3; i++) if (a[i] !== b[i]) return a[i] - b[i]
  return 0
}

/** Chyba pre značku tag pri doterajších značkách existing, alebo '' ak je v poriadku.
 * Rovnaká značka sa preskočí, aby šlo vydanie spustiť znova. */
export function check(tag, existing = []) {
  if (!shape.test(tag)) return `Značka vydania musí mať tvar v1.2.3 (malé v, čísla bez núl na začiatku), zadané: ${tag}`
  const v = parse(tag)
  let top = null
  for (const t of existing) {
    if (t === tag) continue // opakované spustenie toho istého vydania
    const p = parse(t)
    if (p && (!top || cmp(p, top.v) > 0)) top = { t, v: p }
  }
  if (top && cmp(v, top.v) <= 0) return `Značka ${tag} nie je vyššia než posledné vydanie ${top.t}. Nainštalované verzie by sa neaktualizovali.`
  return ''
}

if (import.meta.url === `file://${process.argv[1]}`) {
  const tag = process.argv[2] || ''
  let input = ''
  process.stdin.setEncoding('utf8')
  for await (const chunk of process.stdin) input += chunk
  const existing = input.split(/\s+/).filter(Boolean)
  const err = check(tag, existing)
  if (err) {
    console.log(`::error::${err}`)
    process.exit(1)
  }
  console.log(`Značka ${tag} je v poriadku.`)
}
