// Tlačové zostavy zo zoznamov: rovnaké stĺpce a riadky ako export (po filtri
// a hľadaní), s nastavením pre každú stránku — výber a poradie stĺpcov,
// orientácia strany, zoskupenie a súčty. Výstup do tlače/PDF alebo Excelu.
//
// Súčty sú súčty už vypočítaných súm zo zoznamu (v centoch, bez zaokrúhľovania
// cez float) — frontend tu nič neprepočítava, iba sčíta stĺpec.

import { csvText } from './export.js'

const KEY = 'fiskus.report.'

/** Kľúč stĺpca v nastavení (hlavička je v rámci zoznamu jedinečná). */
const colKey = (c) => c.header

/** Predvolené nastavenie zostavy pre zoznam. */
export function defaultSettings(name, columns) {
  const keys = columns.map(colKey)
  return {
    title: name,
    columns: defaultColumns(columns),
    orientation: keys.length > 6 ? 'landscape' : 'portrait',
    groupBy: '',
    totals: true,
    format: 'pdf',
  }
}

/**
 * Predvolené stĺpce zostavy: pri dlhom exporte (napr. 28 stĺpcov faktúry)
 * prvé tri (číslo, druh, názov), najviac dva dátumy a štyri sumy, spolu
 * najviac 9 v pôvodnom poradí. Ostatné si používateľ zapne sám.
 */
export function defaultColumns(columns) {
  if (columns.length <= 9) return columns.map(colKey)
  const pick = new Set(columns.slice(0, 3).map(colKey))
  for (const c of columns.filter((c) => c.type === 'date').slice(0, 2)) pick.add(colKey(c))
  for (const c of columns.filter((c) => c.type === 'money').slice(0, 4)) pick.add(colKey(c))
  return columns.map(colKey).filter((k) => pick.has(k)).slice(0, 9)
}

/** Uložené nastavenie → platné (neznáme stĺpce preč, chýbajúce polia predvolené). */
export function cleanSettings(saved, name, columns) {
  const d = defaultSettings(name, columns)
  if (!saved || typeof saved !== 'object') return d
  const keys = new Set(columns.map(colKey))
  const cols = Array.isArray(saved.columns) ? saved.columns.filter((k) => keys.has(k)) : []
  return {
    title: typeof saved.title === 'string' && saved.title.trim() ? saved.title.slice(0, 120) : d.title,
    columns: cols.length ? [...new Set(cols)] : d.columns,
    orientation: saved.orientation === 'landscape' || saved.orientation === 'portrait' ? saved.orientation : d.orientation,
    groupBy: keys.has(saved.groupBy) ? saved.groupBy : '',
    totals: saved.totals !== false,
    format: saved.format === 'xlsx' ? 'xlsx' : 'pdf',
  }
}

export function loadSettings(name, columns) {
  let saved = null
  try {
    saved = JSON.parse(localStorage.getItem(KEY + name) ?? 'null')
  } catch {
    saved = null // prehliadač bez úložiska alebo poškodený záznam
  }
  return cleanSettings(saved, name, columns)
}

export function saveSettings(name, settings) {
  try {
    localStorage.setItem(KEY + name, JSON.stringify(settings))
  } catch {
    // nevadí — nastavenie je iba pohodlie
  }
}

/** Suma v centoch (reťazec z API, aj s čiarkou a medzerami); neplatná = null. */
function cents(v) {
  if (v === null || v === undefined || v === '') return null
  const s = String(v).replace(/\s/g, '').replace(',', '.')
  if (!/^-?\d+(\.\d+)?$/.test(s)) return null
  const neg = s.startsWith('-')
  const [i, f = ''] = s.replace('-', '').split('.')
  const c = Number(i) * 100 + Number((f + '00').slice(0, 2))
  return neg ? -c : c
}
const fromCents = (c) => (c < 0 ? '-' : '') + (Math.abs(c) / 100).toFixed(2)

function sums(cols, rows) {
  const out = {}
  for (const c of cols) {
    if (c.type !== 'money') continue
    let s = 0
    let any = false
    for (const r of rows) {
      const v = cents(c.value(r))
      if (v !== null) {
        s += v
        any = true
      }
    }
    out[c.header] = any ? fromCents(s) : ''
  }
  return out
}

/**
 * Model zostavy: vybrané stĺpce v poradí nastavenia, skupiny (podľa textu
 * stĺpca groupBy, v poradí prvého výskytu) s medzisúčtami a celkový súčet
 * peňažných stĺpcov.
 */
export function reportModel(columns, rows, settings) {
  const byKey = new Map(columns.map((c) => [colKey(c), c]))
  const cols = settings.columns.map((k) => byKey.get(k)).filter(Boolean)
  const group = settings.groupBy ? byKey.get(settings.groupBy) : null
  const groups = []
  if (group) {
    const index = new Map()
    for (const r of rows) {
      const label = printText(group, r) || '(bez hodnoty)'
      if (!index.has(label)) {
        index.set(label, { label, rows: [] })
        groups.push(index.get(label))
      }
      index.get(label).rows.push(r)
    }
    for (const g of groups) g.totals = settings.totals ? sums(cols, g.rows) : null
  } else {
    groups.push({ label: '', rows, totals: null })
  }
  const hasMoney = cols.some((c) => c.type === 'money')
  return {
    title: settings.title,
    orientation: settings.orientation,
    cols,
    groups,
    count: rows.length,
    totals: settings.totals && hasMoney ? sums(cols, rows) : null,
  }
}

const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (ch) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[ch])

/** Text bunky do tlače: sumy s medzerami v tisícoch, dátum d. m. rrrr. */
export function printText(col, row) {
  const t = csvText(col, row).replace(/^'/, '') // ochrana pred vzorcom je iba pre CSV
  if (col.type === 'money' && t) return groupThousands(t)
  if (col.type === 'date' && t) return t.replaceAll('.', '. ')
  return t
}
function groupThousands(t) {
  const [i, f] = t.split(',')
  const neg = i.startsWith('-')
  const g = i.replace('-', '').replace(/\B(?=(\d{3})+(?!\d))/g, ' ')
  return (neg ? '−' : '') + g + (f !== undefined ? ',' + f : '')
}
const moneyText = (v) => (v === '' || v == null ? '' : groupThousands(String(v).replace('.', ',')))

/** HTML zostavy na tlač (iba obsah, štýly sú v reportCSS). */
export function reportHTML(model, meta = {}) {
  const num = (c) => (c.type === 'money' || c.type === 'number' ? ' class="n"' : '')
  const head = `<tr>${model.cols.map((c) => `<th${num(c)}>${esc(c.header)}</th>`).join('')}</tr>`
  const totalRow = (t, label) =>
    `<tr class="tot">${model.cols
      .map((c, i) => `<td${num(c)}>${i === 0 && !(c.header in t && t[c.header] !== '') ? esc(label) : c.type === 'money' ? esc(moneyText(t[c.header])) : ''}</td>`)
      .join('')}</tr>`
  let body = ''
  for (const g of model.groups) {
    if (g.label) body += `<tr class="grp"><td colspan="${model.cols.length}">${esc(g.label)} <span>(${g.rows.length})</span></td></tr>`
    body += g.rows.map((r) => `<tr>${model.cols.map((c) => `<td${num(c)}>${esc(printText(c, r))}</td>`).join('')}</tr>`).join('')
    if (g.label && g.totals) body += totalRow(g.totals, 'Spolu')
  }
  const foot = model.totals ? `<tfoot>${totalRow(model.totals, 'Spolu')}</tfoot>` : ''
  return `<header class="rp-head">
  <div><h1>${esc(model.title)}</h1>${meta.subtitle ? `<p>${esc(meta.subtitle)}</p>` : ''}</div>
  <div class="rp-meta">${esc(meta.company ?? '')}<br>${esc(meta.printed ?? '')} · ${model.count} ${model.count === 1 ? 'riadok' : model.count < 5 && model.count > 0 ? 'riadky' : 'riadkov'}</div>
</header>
<table class="rp"><thead>${head}</thead><tbody>${body}</tbody>${foot}</table>`
}

// logo Fiškusa s názvom do päty strany — jeden obrázok, aby boli značka
// a názov vždy na jednej osi (text vedľa obrázka v okraji strany prehliadač
// zarovná na účiaru a logo by trčalo hore)
const LOGO =
  'data:image/svg+xml,' +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 50 12" width="50" height="12">' +
      '<g transform="scale(.375)"><rect width="32" height="32" rx="7" fill="#1f3a5c"/><path d="M10 8h13v3.4h-9v3.6h8v3.3h-8V24h-4z" fill="#fff"/><rect x="18" y="20.5" width="5" height="3.5" fill="#d9a441"/></g>' +
      '<text x="15.5" y="6" dominant-baseline="central" font-family="&quot;Source Sans 3&quot;, &quot;Segoe UI&quot;, Arial, sans-serif" font-size="10" font-weight="600" fill="#1f3a5c">Fiškus</text></svg>',
  )

/** Štýly tlače: hustá tabuľka ako v účtovnej knihe, logo a číslovanie strán v päte. */
export function reportCSS(orientation) {
  return `@page { size: A4 ${orientation === 'landscape' ? 'landscape' : 'portrait'}; margin: 12mm 10mm 14mm;
  @bottom-right { content: "Strana " counter(page) " z " counter(pages); font: 8pt 'Source Sans 3', sans-serif; color: #6b7480; }
  @bottom-left { content: url("${LOGO}"); } }
#report-print { font-family: 'Source Sans 3', sans-serif; color: #1c232c; font-size: 8.5pt; font-variant-numeric: tabular-nums; }
#report-print .rp-head { display: flex; justify-content: space-between; align-items: flex-end; border-bottom: 2px solid #1f3a5c; padding-bottom: 4px; margin-bottom: 6px; }
#report-print h1 { font-size: 14pt; margin: 0; color: #1f3a5c; }
#report-print .rp-head p { margin: 2px 0 0; color: #4e5866; }
#report-print .rp-meta { text-align: right; color: #4e5866; font-size: 8pt; }
#report-print table.rp { width: 100%; border-collapse: collapse; }
#report-print thead { display: table-header-group; }
#report-print tfoot { display: table-row-group; }
#report-print th { text-align: left; font-weight: 600; border-bottom: 1px solid #1f3a5c; padding: 3px 4px; color: #1f3a5c; }
#report-print td { padding: 2px 4px; border-bottom: 1px solid #e3e7ec; vertical-align: top; }
#report-print tr { break-inside: avoid; }
#report-print tbody tr:nth-child(even) td { background: #f6f8fa; }
#report-print .n { text-align: right; white-space: nowrap; }
#report-print tr.grp td { background: #eef2f6; font-weight: 600; color: #1f3a5c; padding-top: 5px; border-bottom: 1px solid #c9d2dc; }
#report-print tr.grp span { font-weight: 400; color: #6b7480; }
#report-print tr.tot td { font-weight: 600; background: none; border-bottom: 1px solid #c9d2dc; }
#report-print tfoot tr.tot td { border-top: 1px solid #1f3a5c; border-bottom: 3px double #1f3a5c; }`
}

/** Údaje hárku Excelu: názov, podnadpis, hlavička, riadky (so skupinami) a súčty. */
export function reportSheet(model, meta, xlsxCell) {
  const bold = (value) => ({ value, type: String, fontWeight: 'bold' })
  const data = [
    [{ value: model.title, type: String, fontWeight: 'bold', fontSize: 14 }],
    [{ value: [meta.company, meta.subtitle, meta.printed].filter(Boolean).join(' · '), type: String }],
    [],
    model.cols.map((c) => bold(c.header)),
  ]
  const totalRow = (t, label) =>
    model.cols.map((c, i) => {
      if (c.type === 'money' && t[c.header] !== '') return { value: Number(t[c.header]), type: Number, format: '#,##0.00', fontWeight: 'bold' }
      return i === 0 ? bold(label) : null
    })
  for (const g of model.groups) {
    if (g.label) data.push([bold(`${g.label} (${g.rows.length})`)])
    for (const r of g.rows) data.push(model.cols.map((c) => xlsxCell(c, r)))
    if (g.label && g.totals) data.push(totalRow(g.totals, 'Spolu'))
  }
  if (model.totals) data.push(totalRow(model.totals, 'Spolu'))
  return data
}
