// Fiškus v prehliadači — service worker.
//
// 1. Požiadavky na /api/… nejdú na internet: pošle ich karte, ktorá má
//    spustený Fiškus (web worker s programom a databázou), a vráti jej odpoveď.
// 2. Aplikácia funguje aj bez internetu: index.html a súbory z /assets/
//    (s odtlačkom v mene) sa uložia do cache.
const CACHE = 'fiskus-app'
let ownerId = null

self.addEventListener('install', () => self.skipWaiting())
self.addEventListener('activate', (e) => e.waitUntil(self.clients.claim()))

self.addEventListener('message', (e) => {
  if (e.data?.type === 'fiskus-owner') ownerId = e.source.id
})

// karta s bežiacim Fiškusom: odosielateľ, posledná známa, alebo sa opýta všetkých
async function owner(clientId) {
  for (const id of [clientId, ownerId]) {
    if (!id) continue
    const c = await self.clients.get(id)
    if (c && (await answers(c))) return c
  }
  for (const c of await self.clients.matchAll({ type: 'window' })) {
    if (await answers(c)) {
      ownerId = c.id
      return c
    }
  }
  return null
}

function answers(client) {
  return new Promise((resolve) => {
    const ch = new MessageChannel()
    const t = setTimeout(() => resolve(false), 1000)
    ch.port1.onmessage = () => {
      clearTimeout(t)
      resolve(true)
    }
    client.postMessage({ type: 'fiskus-owner?' }, [ch.port2])
  })
}

async function api(event) {
  const req = event.request
  const client = await owner(event.clientId)
  if (!client) {
    return Response.json({ error: 'Fiškus nie je otvorený. Otvorte ho a skúste znova.' }, { status: 503 })
  }
  const body = req.method === 'GET' || req.method === 'HEAD' ? null : await req.arrayBuffer()
  const url = new URL(req.url)
  const ch = new MessageChannel()
  const res = await new Promise((resolve) => {
    ch.port1.onmessage = (m) => resolve(m.data)
    const msg = { type: 'fiskus-api', req: { method: req.method, url: url.pathname + url.search, headers: [...req.headers], body } }
    client.postMessage(msg, body ? [ch.port2, body] : [ch.port2])
  })
  const bodyless = req.method === 'HEAD' || res.status === 204 || res.status === 304
  return new Response(bodyless ? null : res.body, { status: res.status, headers: res.headers })
}

async function cached(req, key = req) {
  const cache = await caches.open(CACHE)
  const hit = await cache.match(key)
  if (hit) return clean(hit)
  const res = await clean(await fetch(req))
  if (res.ok) cache.put(key, res.clone())
  return res
}

// Safari (WebKit) odmietne pri otváraní stránky odpoveď service workera,
// ktorá prešla presmerovaním („Response served by service worker has
// redirections“) — Cloudflare Pages presmeruje /index.html na /. Odpoveď
// sa preto vždy prebalí do novej (bez príznaku presmerovania).
async function clean(res) {
  if (!res.redirected) return res
  return new Response(await res.blob(), { status: res.status, statusText: res.statusText, headers: res.headers })
}

const PAGE = '/' // index.html (Cloudflare Pages ho podáva na /)

async function page(req) {
  // aplikácia má jednu stránku: každá adresa (aj /faktury/12) = index.html
  const cache = await caches.open(CACHE)
  try {
    const res = await clean(await fetch(PAGE, { cache: 'no-cache' }))
    if (res.ok) {
      cache.put(PAGE, res.clone())
      return res
    }
  } catch {
    /* bez internetu */
  }
  const hit = (await cache.match(PAGE)) ?? (await cache.match('/index.html')) // aj zo starších verzií
  return hit ? clean(hit) : fetch(req)
}

self.addEventListener('fetch', (event) => {
  const req = event.request
  const url = new URL(req.url)
  if (url.origin !== self.location.origin) return
  if (url.pathname.startsWith('/api/')) {
    event.respondWith(api(event))
  } else if (req.mode === 'navigate') {
    event.respondWith(page(req))
  } else if (req.method === 'GET' && url.pathname.startsWith('/assets/')) {
    event.respondWith(cached(req))
  }
})
