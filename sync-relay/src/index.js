// Schránka synchronizácie Fiškusa (Cloudflare Worker + Durable Object).
// Každé spárovanie (kanál) je jeden Durable Object s vlastným úložiskom.
// Nasadenie: sync-relay/README.md.
import { DurableObject } from 'cloudflare:workers'
import { LIMITS, expired, handle } from './core.js'

const CORS = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Methods': 'GET, PUT, POST, DELETE',
  'Access-Control-Allow-Headers': 'Authorization, Content-Type, X-Version',
  'Access-Control-Expose-Headers': 'X-Version',
  'Access-Control-Max-Age': '86400',
}

function cors(res) {
  const r = new Response(res.body, res)
  for (const [k, v] of Object.entries(CORS)) r.headers.set(k, v)
  r.headers.set('Cache-Control', 'no-store')
  return r
}

export default {
  async fetch(request, env) {
    if (request.method === 'OPTIONS') return new Response(null, { status: 204, headers: CORS })
    const url = new URL(request.url)
    if (url.pathname === '/') return cors(new Response('Schránka synchronizácie Fiškusa (https://fiskus.online).\n', { headers: { 'Content-Type': 'text/plain; charset=utf-8' } }))
    const m = url.pathname.match(/^\/v1\/c\/([0-9a-f]{32})(\/.*)?$/)
    if (!m) return cors(Response.json({ error: 'neznáma adresa' }, { status: 404 }))
    const stub = env.CHANNELS.get(env.CHANNELS.idFromName(m[1]))
    return cors(await stub.fetch(request))
  },
}

export class Channel extends DurableObject {
  async fetch(request) {
    const path = new URL(request.url).pathname.replace(/^\/v1\/c\/[0-9a-f]{32}/, '')
    const s = this.ctx.storage
    const store = {
      get: (k) => s.get(k),
      put: (entries) => s.put(entries),
      delete: (keys) => s.delete(keys),
      deleteAll: async () => {
        await s.deleteAlarm()
        await s.deleteAll()
      },
    }
    const res = await handle(request, path, store)
    if (!(request.method === 'DELETE' && path === '') && !(await s.getAlarm())) {
      await s.setAlarm(Date.now() + LIMITS.ttlDays * 86400_000)
    }
    return res
  }

  // nepoužívaná schránka sa po ttlDays zmaže
  async alarm() {
    const meta = await this.ctx.storage.get('meta')
    if (expired(meta)) {
      await this.ctx.storage.deleteAll()
      return
    }
    await this.ctx.storage.setAlarm(meta.touched + LIMITS.ttlDays * 86400_000)
  }
}
