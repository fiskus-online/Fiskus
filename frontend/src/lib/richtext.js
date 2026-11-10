// Formátovaný text dokladov (text nad položkami a pod nimi): rovnaké
// pravidlá ako na serveri (backend/internal/richtext). Povolené sú odseky,
// zalomenie riadku, tučné, kurzíva, podčiarknuté, odrážky a číslovaný
// zoznam; všetko ostatné (písma, farby, tabuľky z Wordu, skripty) sa pri
// vložení zahodí. Spoločné vzorky: backend/internal/richtext/testdata/cases.json.

const reTag = /^<\s*(\/?)\s*([a-zA-Z][a-zA-Z0-9:_-]*)([^>]*?)(\/?)\s*>/
const reStyle = /style\s*=\s*("([^"]*)"|'([^']*)')/i
const reSpace = /[\t\n\f\r  ]+/g
const reNumbered = /^(\d+|[a-zA-Z]|[ivxlcdmIVXLCDM]+)[.)]$/

const skipped = new Set(['script', 'style', 'head', 'title', 'xml', 'template', 'noscript', 'iframe', 'object', 'svg', 'math'])
const blocky = new Set(['p', 'div', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'blockquote', 'pre', 'section', 'article', 'header', 'footer', 'table',
  'tr', 'dl', 'dt', 'dd', 'figure', 'address', 'hr'])

/** Je text formátovaný (HTML), alebo obyčajný? */
export function isRich(s) {
  return String(s ?? '').trim().startsWith('<')
}

let decoder = null
function unescape(s) {
  if (!s.includes('&')) return s
  if (typeof document !== 'undefined') {
    decoder ??= document.createElement('textarea')
    decoder.innerHTML = s
    return decoder.value
  }
  return s.replace(/&(lt|gt|amp|quot|apos|nbsp|#\d+|#x[0-9a-f]+);/gi, (m, e) => {
    const named = { lt: '<', gt: '>', amp: '&', quot: '"', apos: "'", nbsp: ' ' }[e.toLowerCase()]
    if (named) return named
    return String.fromCodePoint(e[1] === 'x' || e[1] === 'X' ? parseInt(e.slice(2), 16) : parseInt(e.slice(1), 10))
  })
}

/** Únik ako Go html.EscapeString. */
export function escapeHTML(s) {
  return String(s).replace(/[&<>'"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&#34;' })[c])
}

/** Odseky a položky z formátovaného textu. */
export function parse(input) {
  let s = input
  const blocks = []
  let cur = null
  let stack = []
  const lists = []
  let marker = ''
  const top = () => (stack.length ? stack[stack.length - 1].fmt : {})
  const flush = () => {
    if (cur) blocks.push(cur)
    cur = null
  }
  const block = () => {
    if (!cur) {
      cur = { list: '', index: 0, runs: [] }
      if (marker) {
        cur.list = reNumbered.test(marker) ? 'ol' : 'ul'
        marker = ''
      }
    }
    return cur
  }
  const text = (t) => {
    const f = top()
    if (f.ignore) {
      marker += unescape(t).replace(reSpace, '').trim()
      return
    }
    t = unescape(t).replace(reSpace, ' ')
    if (!t) return
    block().runs.push({ text: t, bold: !!f.bold, italic: !!f.italic, underline: !!f.underline })
  }
  const open = (name, attrs, self) => {
    if (name === 'br') {
      block().runs.push({ text: '\n' })
      return
    }
    if (name === 'ul' || name === 'ol') {
      flush()
      lists.push(name)
      return
    }
    if (name === 'li') {
      flush()
      cur = { list: lists.length ? lists[lists.length - 1] : 'ul', index: 0, runs: [] }
    } else if (blocky.has(name)) {
      flush()
    } else if ((name === 'td' || name === 'th') && cur) {
      cur.runs.push({ text: ' ' })
    }
    if (self) return
    const f = { ...top() }
    if (name === 'b' || name === 'strong') f.bold = true
    if (['i', 'em', 'cite', 'dfn', 'var'].includes(name)) f.italic = true
    if (name === 'u' || name === 'ins') f.underline = true
    if (/^h[1-6]$/.test(name) || name === 'th') f.bold = true
    const m = reStyle.exec(attrs)
    if (m) {
      const st = ((m[2] ?? '') + (m[3] ?? '')).toLowerCase().replaceAll(' ', '')
      if (st.includes('mso-list:ignore')) f.ignore = true
      if (['font-weight:bold', 'font-weight:600', 'font-weight:700', 'font-weight:800', 'font-weight:900'].some((x) => st.includes(x))) f.bold = true
      if (st.includes('font-weight:normal') || st.includes('font-weight:400')) f.bold = false
      if (st.includes('font-style:italic')) f.italic = true
      if (st.includes('text-decoration:underline') || st.includes('text-decoration-line:underline')) f.underline = true
    }
    stack.push({ name, fmt: f })
  }
  const close = (name) => {
    if (name === 'ul' || name === 'ol') {
      flush()
      lists.pop()
      return
    }
    if (name === 'li' || blocky.has(name)) flush()
    for (let i = stack.length - 1; i >= 0; i--) {
      if (stack[i].name === name) {
        stack = stack.slice(0, i)
        break
      }
    }
  }

  while (s.length) {
    const i = s.indexOf('<')
    if (i < 0) {
      text(s)
      break
    }
    if (i > 0) {
      text(s.slice(0, i))
      s = s.slice(i)
    }
    if (s.startsWith('<!--')) {
      const end = s.indexOf('-->')
      if (end < 0) break
      s = s.slice(end + 3)
      continue
    }
    if (s.startsWith('<!') || s.startsWith('<?')) {
      const end = s.indexOf('>')
      if (end < 0) break
      s = s.slice(end + 1)
      continue
    }
    const m = reTag.exec(s)
    if (!m) {
      text('<')
      s = s.slice(1)
      continue
    }
    s = s.slice(m[0].length)
    const closing = m[1] === '/'
    const name = m[2].toLowerCase()
    const self = m[4] === '/'
    if (skipped.has(name) && !closing && !self) {
      const end = s.toLowerCase().indexOf('</' + name)
      if (end < 0) break
      s = s.slice(end)
      const j = s.indexOf('>')
      if (j >= 0) s = s.slice(j + 1)
      continue
    }
    if (closing) close(name)
    else open(name, m[3], self)
  }
  flush()
  return finish(blocks)
}

function same(a, b) {
  return a.bold === b.bold && a.italic === b.italic && a.underline === b.underline
}

function finish(input) {
  const out = []
  let n = 0
  for (const b of input) {
    let runs = []
    for (const r0 of b.runs) {
      const r = { ...r0 }
      if (r.text === '\n') {
        runs.push(r)
        continue
      }
      const last = runs[runs.length - 1]
      if (!last || last.text === '\n' || last.text.endsWith(' ')) r.text = r.text.replace(/^ +/, '')
      if (!r.text) continue
      if (last && last.text !== '\n' && same(last, { bold: !!r.bold, italic: !!r.italic, underline: !!r.underline })) {
        last.text += r.text
        continue
      }
      runs.push({ text: r.text, bold: !!r.bold, italic: !!r.italic, underline: !!r.underline })
    }
    while (runs.length) {
      const k = runs.length - 1
      if (runs[k].text === '\n') {
        runs.pop()
        continue
      }
      runs[k].text = runs[k].text.replace(/ +$/, '')
      if (!runs[k].text) {
        runs.pop()
        continue
      }
      break
    }
    for (let k = 0; k + 1 < runs.length; k++) {
      if (runs[k + 1].text === '\n') runs[k].text = runs[k].text.replace(/ +$/, '')
    }
    runs = runs.filter((r) => r.text !== '')
    if (!runs.length) continue
    const blk = { list: b.list, index: 0, runs }
    if (b.list === 'ol') {
      n = out.length && out[out.length - 1].list === 'ol' ? n + 1 : 1
      blk.index = n
    }
    out.push(blk)
  }
  return out
}

/** Odseky v povolenom tvare HTML. */
export function render(blocks) {
  let out = ''
  let list = ''
  for (const bl of blocks) {
    if (bl.list !== list) {
      if (list) out += `</${list}>`
      if (bl.list) out += `<${bl.list}>`
      list = bl.list
    }
    const tag = bl.list ? 'li' : 'p'
    out += `<${tag}>`
    for (const r of bl.runs) {
      if (r.text === '\n') {
        out += '<br>'
        continue
      }
      let t = escapeHTML(r.text)
      if (r.underline) t = `<u>${t}</u>`
      if (r.italic) t = `<em>${t}</em>`
      if (r.bold) t = `<strong>${t}</strong>`
      out += t
    }
    out += `</${tag}>`
  }
  if (list) out += `</${list}>`
  return out
}

/** Vyčistí text na povolený tvar; obyčajný text (bez „<“) iba oreže. */
export function sanitize(s) {
  s = String(s ?? '').trim()
  if (!s.includes('<')) return s
  return render(parse(s))
}

/** Obyčajný text (odrážky „• “, číslovanie „1. “). */
export function plain(s) {
  if (!isRich(s)) return String(s ?? '').trim()
  return parse(s)
    .map((b) => (b.list === 'ul' ? '• ' : b.list === 'ol' ? `${b.index}. ` : '') + b.runs.map((r) => r.text).join(''))
    .join('\n')
}

/** HTML na zobrazenie (tlač, editor): obyčajný text s koncami riadkov ako odseky. */
export function toHTML(s) {
  if (isRich(s)) return sanitize(s)
  const t = String(s ?? '').trim()
  if (!t) return ''
  return t
    .split(/\r?\n/)
    .map((l) => `<p>${l ? escapeHTML(l) : '<br>'}</p>`)
    .join('')
}

/** Text z editora: prázdny editor = prázdny text. */
export function fromEditor(html) {
  return sanitize(html)
}
