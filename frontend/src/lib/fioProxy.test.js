// Funkcia functions/fio/[[path]].js (Cloudflare Pages): prepošle iba
// požiadavku na pohyby, token z hlavičky, nič iné.
import { afterEach, describe, expect, it, vi } from 'vitest'
import { onRequest } from '../../functions/fio/[[path]].js'

const TOKEN = 'AbCdEf0123456789AbCdEf0123456789AbCdEf0123456789AbCdEf0123456789'
const call = (path, { method = 'GET', headers = { 'X-Fio-Token': TOKEN } } = {}) =>
  onRequest({ request: new Request(`https://app.fiskus.online/fio/${path.join('/')}`, { method, headers }), params: { path } })

describe('Fio cez funkciu aplikácie', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('prepošle pohyby za obdobie s tokenom z hlavičky', async () => {
    const fetch = vi.fn(async () => new Response('{"accountStatement":{}}', { status: 200, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetch)
    const r = await call(['periods', '2026-08-01', '2026-10-29', 'transactions.json'])
    expect(r.status).toBe(200)
    expect(r.headers.get('Cache-Control')).toBe('no-store')
    expect(await r.text()).toContain('accountStatement')
    expect(fetch.mock.calls[0][0]).toBe(`https://fioapi.fio.cz/v1/rest/periods/${TOKEN}/2026-08-01/2026-10-29/transactions.json`)
  })

  it('odpoveď banky (napr. limit 30 s) vráti s rovnakým kódom', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('', { status: 409 })))
    expect((await call(['periods', '2026-08-01', '2026-10-29', 'transactions.json'])).status).toBe(409)
  })

  it.each([
    ['iný druh požiadavky', ['last', 'transactions.json'], {}, 400],
    ['zlý dátum', ['periods', '2026-8-1', '2026-10-29', 'transactions.json'], {}, 400],
    ['bez tokenu', ['periods', '2026-08-01', '2026-10-29', 'transactions.json'], { headers: {} }, 400],
    ['token s inými znakmi', ['periods', '2026-08-01', '2026-10-29', 'transactions.json'], { headers: { 'X-Fio-Token': '../x?y=' + TOKEN } }, 400],
    ['POST', ['periods', '2026-08-01', '2026-10-29', 'transactions.json'], { method: 'POST' }, 405],
    ['cudzí web', ['periods', '2026-08-01', '2026-10-29', 'transactions.json'], { headers: { 'X-Fio-Token': TOKEN, Origin: 'https://zly.example' } }, 403],
  ])('odmietne: %s', async (_, path, opts, status) => {
    const fetch = vi.fn()
    vi.stubGlobal('fetch', fetch)
    expect((await call(path, opts)).status).toBe(status)
    expect(fetch).not.toHaveBeenCalled()
  })
})
