// Cloudflare Pages Function pre Fiškus v prehliadači (app.fiskus.online):
// Fio banka nedovolí volať svoje API z webovej stránky, preto požiadavku
// na pohyby prepošle táto funkcia. Prijíma iba
//   GET /fio/periods/{od}/{do}/transactions.json  s hlavičkou X-Fio-Token
// Token (iba na čítanie pohybov) sa neukladá a nezapisuje; odpoveď banky
// ide späť bez zmeny a bez ukladania do cache.

const DATE = /^\d{4}-\d{2}-\d{2}$/
const TOKEN = /^[A-Za-z0-9]{20,128}$/

export async function onRequest({ request, params }) {
  if (request.method !== 'GET') return text('Povolené je iba GET', 405)
  // iba z tej istej stránky (Fiškus v prehliadači), nie z cudzích webov
  const origin = request.headers.get('Origin')
  if (origin && origin !== new URL(request.url).origin) return text('Nepovolený pôvod', 403)
  const p = params.path ?? []
  if (p.length !== 4 || p[0] !== 'periods' || !DATE.test(p[1]) || !DATE.test(p[2]) || p[3] !== 'transactions.json') {
    return text('Neplatná požiadavka', 400)
  }
  const token = request.headers.get('X-Fio-Token') ?? ''
  if (!TOKEN.test(token)) return text('Chýba token Fio API', 400)
  let res
  try {
    res = await fetch(`https://fioapi.fio.cz/v1/rest/periods/${token}/${p[1]}/${p[2]}/transactions.json`, {
      headers: { Accept: 'application/json' },
      cf: { cacheTtl: 0, cacheEverything: false },
    })
  } catch {
    return text('Fio banka neodpovedá', 502)
  }
  return new Response(res.body, {
    status: res.status,
    headers: {
      'Content-Type': res.headers.get('Content-Type') ?? 'application/json',
      'Cache-Control': 'no-store',
      'Referrer-Policy': 'no-referrer',
    },
  })
}

function text(msg, status) {
  return new Response(msg, { status, headers: { 'Content-Type': 'text/plain; charset=utf-8', 'Cache-Control': 'no-store' } })
}
