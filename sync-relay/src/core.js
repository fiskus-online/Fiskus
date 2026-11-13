// Schránka synchronizácie telefónu s Fiškusom v počítači — logika jedného
// kanála (jedno spárovanie). Drží iba zašifrované dáta: kópiu databázy
// z počítača a frontu záznamov z telefónu. Kľúč na rozšifrovanie nepozná.
//
// Rovnaké rozhranie má schránka v pamäti v Go (backend/internal/relay/memserver.go),
// limity sú rovnaké ako v backend/internal/relay/client.go.
//
// store: { get(key), put(entries), delete(keys), deleteAll() } — v Cloudflare
// úložisko Durable Object, v testoch mapa v pamäti.

export const LIMITS = {
  snapshot: 32 << 20, // kópia databázy
  item: 512 << 10, // jeden záznam z telefónu
  inbox: 500, // záznamov čakajúcich na počítač
  chunk: 1 << 20, // kópia sa ukladá po častiach
  ttlDays: 60, // nepoužívaná schránka sa zmaže
}

const json = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
const err = (status, error) => json({ error }, status)
const hex = (buf) => [...new Uint8Array(buf)].map((b) => b.toString(16).padStart(2, '0')).join('')

async function readBody(req, limit) {
  const len = Number(req.headers.get('Content-Length') || 0)
  if (len > limit) return null
  const b = new Uint8Array(await req.arrayBuffer())
  return b.length > limit ? null : b
}

/** Porovnanie reťazcov v konštantnom čase. */
function same(a, b) {
  if (a.length !== b.length) return false
  let d = 0
  for (let i = 0; i < a.length; i++) d |= a.charCodeAt(i) ^ b.charCodeAt(i)
  return d === 0
}

/**
 * Vybaví požiadavku na kanál. path je časť adresy za /v1/c/{kanál}
 * („/state“, „/snapshot“, „/inbox“, „/inbox/{id}“, „/want“ alebo „“).
 */
export async function handle(req, path, store, now = Date.now()) {
  const auth = (req.headers.get('Authorization') || '').replace(/^Bearer /, '')
  if (auth.length < 32) return err(401, 'chýba prístupový kľúč')
  const sum = hex(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(auth)))
  let meta = await store.get('meta')
  if (!meta) {
    if (req.method === 'DELETE' && path === '') return new Response(null, { status: 204 })
    // prvý prístup schránku založí a zapamätá si odtlačok kľúča
    meta = { auth: sum, snap: null, next: 0, items: {}, want: '' }
  } else if (!same(meta.auth, sum)) {
    return err(401, 'nesprávny prístupový kľúč')
  }
  const at = new Date(now).toISOString().replace(/\.\d{3}Z$/, 'Z')
  meta.touched = now
  const save = (extra = {}) => store.put({ meta, ...extra })

  if (req.method === 'GET' && path === '/state') {
    await save()
    const inbox = Object.entries(meta.items)
      .map(([id, i]) => ({ id, size: i.size, at: i.at }))
      .sort((a, b) => (a.id < b.id ? -1 : 1))
    const snap = meta.snap && { version: meta.snap.version, size: meta.snap.size, at: meta.snap.at }
    return json({ snapshot: snap, inbox, want: meta.want || undefined })
  }

  if (req.method === 'PUT' && path === '/snapshot') {
    const version = Number(req.headers.get('X-Version'))
    if (!Number.isSafeInteger(version) || version <= 0) return err(400, 'chýba verzia kópie')
    const b = await readBody(req, LIMITS.snapshot)
    if (!b) return err(413, 'kópia údajov je priveľká')
    const old = meta.snap?.chunks ?? 0
    const parts = {}
    const chunks = Math.max(1, Math.ceil(b.length / LIMITS.chunk))
    for (let i = 0; i < chunks; i++) parts[`snap:${i}`] = b.slice(i * LIMITS.chunk, (i + 1) * LIMITS.chunk)
    meta.snap = { version, size: b.length, at, chunks }
    meta.want = ''
    // najviac 128 kľúčov naraz — po častiach, meta až na koniec
    const keys = Object.keys(parts)
    for (let i = 0; i < keys.length; i += 64) await store.put(Object.fromEntries(keys.slice(i, i + 64).map((k) => [k, parts[k]])))
    await save()
    const stale = []
    for (let i = chunks; i < old; i++) stale.push(`snap:${i}`)
    if (stale.length) await store.delete(stale)
    return new Response(null, { status: 204 })
  }

  if (req.method === 'GET' && path === '/snapshot') {
    if (!meta.snap) return err(404, 'kópia ešte nie je')
    await save()
    const keys = Array.from({ length: meta.snap.chunks }, (_, i) => `snap:${i}`)
    const got = await store.get(keys)
    const out = new Uint8Array(meta.snap.size)
    let off = 0
    for (const k of keys) {
      const c = got.get(k)
      if (!c) return err(500, 'kópia je neúplná — počítač ju pošle znova')
      out.set(c, off)
      off += c.length
    }
    return new Response(out, { headers: { 'Content-Type': 'application/octet-stream', 'X-Version': String(meta.snap.version) } })
  }

  if (req.method === 'POST' && path === '/inbox') {
    if (Object.keys(meta.items).length >= LIMITS.inbox) return err(429, 'fronta je plná — zapnite Fiškus v počítači')
    const b = await readBody(req, LIMITS.item)
    if (!b) return err(413, 'záznam je priveľký')
    meta.next += 1
    const id = String(meta.next).padStart(12, '0')
    meta.items[id] = { size: b.length, at }
    await save({ [`item:${id}`]: b })
    return json({ id })
  }

  if (req.method === 'POST' && path === '/want') {
    meta.want = at
    await save()
    return new Response(null, { status: 204 })
  }

  if (req.method === 'DELETE' && path === '') {
    await store.deleteAll()
    return new Response(null, { status: 204 })
  }

  const m = path.match(/^\/inbox\/(\d{12})$/)
  if (m) {
    const id = m[1]
    if (req.method === 'DELETE') {
      if (meta.items[id]) {
        delete meta.items[id]
        await store.delete([`item:${id}`])
        await save()
      }
      return new Response(null, { status: 204 })
    }
    if (req.method === 'GET') {
      if (!meta.items[id]) return err(404, 'záznam nie je')
      await save()
      return new Response(await store.get(`item:${id}`), { headers: { 'Content-Type': 'application/octet-stream' } })
    }
    return err(405, 'nepovolená metóda')
  }
  return err(404, 'neznáma adresa')
}

/** Má sa nepoužívaná schránka zmazať? */
export function expired(meta, now = Date.now()) {
  return !meta || now - (meta.touched ?? 0) > LIMITS.ttlDays * 86400_000
}
