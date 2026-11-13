// node --test sync-relay/test — logika schránky s úložiskom v pamäti
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { LIMITS, expired, handle } from '../src/core.js'

function memStore() {
  const m = new Map()
  return {
    m,
    get: async (k) => (Array.isArray(k) ? new Map(k.filter((x) => m.has(x)).map((x) => [x, m.get(x)])) : m.get(k)),
    put: async (entries) => {
      for (const [k, v] of Object.entries(entries)) m.set(k, structuredClone(v))
    },
    delete: async (keys) => keys.forEach((k) => m.delete(k)),
    deleteAll: async () => m.clear(),
  }
}

const AUTH = 'a'.repeat(43)
const call = (store, method, path, { body, auth = AUTH, headers = {} } = {}, now) =>
  handle(new Request('https://x/v1/c/0123456789abcdef0123456789abcdef' + path, { method, body, headers: { Authorization: 'Bearer ' + auth, ...headers } }), path, store, now)

test('stav, kópia a fronta', async () => {
  const s = memStore()
  assert.equal((await call(s, 'GET', '/snapshot')).status, 404)
  let st = await (await call(s, 'GET', '/state')).json()
  assert.deepEqual(st, { snapshot: null, inbox: [] })

  assert.equal((await call(s, 'POST', '/want')).status, 204)
  assert.ok((await (await call(s, 'GET', '/state')).json()).want)

  // kópia väčšia ako jedna časť
  const big = new Uint8Array(LIMITS.chunk * 2 + 10).map((_, i) => i % 251)
  assert.equal((await call(s, 'PUT', '/snapshot', { body: big, headers: { 'X-Version': '7' } })).status, 204)
  const res = await call(s, 'GET', '/snapshot')
  assert.equal(res.headers.get('X-Version'), '7')
  assert.deepEqual(new Uint8Array(await res.arrayBuffer()), big)
  // menšia kópia zmaže nepotrebné časti
  assert.equal((await call(s, 'PUT', '/snapshot', { body: new Uint8Array([1, 2]), headers: { 'X-Version': '8' } })).status, 204)
  assert.ok(!s.m.has('snap:1') && !s.m.has('snap:2'))
  assert.deepEqual(new Uint8Array(await (await call(s, 'GET', '/snapshot')).arrayBuffer()), new Uint8Array([1, 2]))

  const ids = []
  for (const t of ['a', 'b', 'c']) ids.push((await (await call(s, 'POST', '/inbox', { body: t })).json()).id)
  assert.deepEqual(ids, ['000000000001', '000000000002', '000000000003'])
  st = await (await call(s, 'GET', '/state')).json()
  assert.equal(st.want, undefined) // kópia požiadavku vybavila
  assert.equal(st.snapshot.version, 8)
  assert.deepEqual(st.inbox.map((i) => i.id), ids)
  assert.equal(await (await call(s, 'GET', '/inbox/' + ids[1])).text(), 'b')
  for (let i = 0; i < 2; i++) assert.equal((await call(s, 'DELETE', '/inbox/' + ids[1])).status, 204)
  assert.equal((await call(s, 'GET', '/inbox/' + ids[1])).status, 404)
  assert.equal((await (await call(s, 'GET', '/state')).json()).inbox.length, 2)

  assert.equal((await call(s, 'DELETE', '')).status, 204)
  assert.equal(s.m.size, 0)
})

test('prístupový kľúč', async () => {
  const s = memStore()
  assert.equal((await call(s, 'GET', '/state', { auth: 'kratky' })).status, 401)
  assert.equal((await call(s, 'GET', '/state')).status, 200)
  assert.equal((await call(s, 'GET', '/state', { auth: 'b'.repeat(43) })).status, 401)
  assert.equal((await call(s, 'DELETE', '', { auth: 'b'.repeat(43) })).status, 401)
})

test('limity', async () => {
  const s = memStore()
  assert.equal((await call(s, 'PUT', '/snapshot', { body: 'x' })).status, 400) // bez verzie
  assert.equal((await call(s, 'POST', '/inbox', { body: new Uint8Array(LIMITS.item + 1) })).status, 413)
  for (let i = 0; i < LIMITS.inbox; i++) await call(s, 'POST', '/inbox', { body: 'x' })
  assert.equal((await call(s, 'POST', '/inbox', { body: 'x' })).status, 429)
  assert.equal((await call(s, 'GET', '/nieco')).status, 404)
})

test('nepoužívaná schránka sa zmaže', () => {
  const now = Date.UTC(2026, 10, 13)
  assert.equal(expired({ touched: now - 59 * 86400_000 }, now), false)
  assert.equal(expired({ touched: now - 61 * 86400_000 }, now), true)
  assert.equal(expired(undefined, now), true)
})
