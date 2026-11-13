// @vitest-environment node
import { afterEach, beforeAll, describe, expect, it } from 'vitest'
import sqlite3InitModule from '@sqlite.org/sqlite-wasm'
import { createBridge } from './sqlbridge.js'
import { createMemoryFiles, memoryStorage } from './memfiles.js'

let sqlite3
beforeAll(async () => {
  sqlite3 = await sqlite3InitModule({ print() {}, printErr() {} })
})

// pamäťové databázy sú zdieľané podľa mena v celom procese — po teste preč
let open = []
const track = (x) => (open.push(x), x)
afterEach(() => {
  for (const x of open) x.close()
  open = []
})

// IndexedDB ako mapa (rovnaké rozhranie ako openIdbFiles)
const fakeIdb = (init = {}) => {
  const m = new Map(Object.entries(init))
  return { m, get: async (k) => m.get(k), put: async (k, v) => void m.set(k, v), del: async (k) => void m.delete(k) }
}
const count = (bridge, id) => bridge.query(id, 'SELECT count(*) FROM t').cells[1]

describe('databáza v pamäti', () => {
  it('spojenia s rovnakým menom zdieľajú údaje, iné meno je iná databáza', () => {
    const files = track(createMemoryFiles(sqlite3))
    const bridge = createBridge(sqlite3, files.open)
    const a = bridge.open('/a.db', false)
    bridge.exec(a, 'CREATE TABLE t(x); INSERT INTO t VALUES (1), (2)')
    const b = bridge.open('/a.db', true)
    expect(count(bridge, b)).toBe(2)
    bridge.close(a)
    bridge.close(b)
    const c = bridge.open('/a.db', false)
    expect(count(bridge, c)).toBe(2) // kotva drží údaje
    const d = bridge.open('/b.db', false)
    expect(() => bridge.query(d, 'SELECT * FROM t')).toThrow(/no such table/)
    bridge.close(c)
    bridge.close(d)
  })

  it('export a import súboru', () => {
    const files = track(createMemoryFiles(sqlite3))
    const bridge = createBridge(sqlite3, files.open)
    bridge.exec(bridge.open('/a.db', false), 'CREATE TABLE t(x); INSERT INTO t VALUES (1), (2), (3)')
    files.importDb('/c.db', files.exportFile('/a.db'))
    expect(count(bridge, bridge.open('/c.db', true))).toBe(3)
    files.unlink('/c.db')
    expect(files.has('/c.db')).toBe(false)
  })
})

describe('úložisko v pamäti s IndexedDB', () => {
  const opts = { db: '/fiskus.db', restore: '/obnova.db', delay: 0 }

  it.each([
    ['s IndexedDB', true, 'indexeddb'],
    ['bez IndexedDB', false, 'memory'],
  ])('%s', async (_, withIdb, mode) => {
    const st = track(await memoryStorage(sqlite3, withIdb ? fakeIdb() : null, opts))
    expect(st.mode).toBe(mode)
  })

  it('po zmene uloží databázu a pri ďalšom štarte ju načíta', async () => {
    const idb = fakeIdb()
    const st = track(await memoryStorage(sqlite3, idb, opts))
    const bridge = createBridge(sqlite3, st.open)
    bridge.exec(bridge.open('/fiskus.db', false), 'CREATE TABLE t(x); INSERT INTO t VALUES (7)')
    await st.save()
    expect(idb.m.get('/fiskus.db')).toBeInstanceOf(Uint8Array)

    // „nová stránka“: v pamäti nič, údaje z IndexedDB
    const files = track(createMemoryFiles(sqlite3))
    files.importDb('/iny.db', idb.m.get('/fiskus.db'))
    const b2 = createBridge(sqlite3, files.open)
    expect(count(b2, b2.open('/iny.db', true))).toBe(1)
  })

  it('obnova zo zálohy: pripravená databáza nahradí aktuálnu pri ďalšom štarte', async () => {
    const files = track(createMemoryFiles(sqlite3))
    const bridge = createBridge(sqlite3, files.open)
    bridge.exec(bridge.open('/zdroj.db', false), 'CREATE TABLE t(x); INSERT INTO t VALUES (1), (2), (3), (4)')
    const backup = files.exportFile('/zdroj.db')

    const idb = fakeIdb()
    const st = track(await memoryStorage(sqlite3, idb, opts))
    await st.writeFile('/obnova.db', backup)
    expect(idb.m.has('/obnova.db')).toBe(true)

    st.close()
    const next = track(await memoryStorage(sqlite3, idb, opts))
    expect(idb.m.has('/obnova.db')).toBe(false)
    expect(idb.m.get('/fiskus.db')).toBe(backup)
    const b2 = createBridge(sqlite3, next.open)
    expect(count(b2, b2.open('/fiskus.db', true))).toBe(4)
  })

  it('záloha (VACUUM INTO) vráti obsah hlavnej databázy', async () => {
    const st = track(await memoryStorage(sqlite3, null, { ...opts, db: '/hlavna.db' }))
    const bridge = createBridge(sqlite3, st.open)
    const id = bridge.open('/hlavna.db', false)
    bridge.exec(id, 'CREATE TABLE t(x); INSERT INTO t VALUES (1)')
    st.removeFile('/kopia.db')
    bridge.exec(id, "VACUUM INTO '/kopia.db'")
    const files = track(createMemoryFiles(sqlite3))
    files.importDb('/x.db', st.readFile('/kopia.db'))
    const b2 = createBridge(sqlite3, files.open)
    expect(count(b2, b2.open('/x.db', true))).toBe(1)
  })
})

describe('ďalšie databázy v IndexedDB', () => {
  it('stav telefónu sa uloží a načíta spolu s hlavnou databázou', async () => {
    const idb = fakeIdb()
    const opts = { db: '/f9.db', restore: '/r9.db', keep: ['/s9.db'], delay: 0 }
    const st = track(await memoryStorage(sqlite3, idb, opts))
    const bridge = createBridge(sqlite3, st.open)
    bridge.exec(bridge.open('/s9.db', false), "CREATE TABLE kv(k, v); INSERT INTO kv VALUES ('phone', 'x')")
    await st.save()
    expect(idb.m.get('/s9.db')).toBeInstanceOf(Uint8Array)
    st.close()
    const next = track(await memoryStorage(sqlite3, idb, opts))
    const b2 = createBridge(sqlite3, next.open)
    expect(b2.query(b2.open('/s9.db', true), 'SELECT v FROM kv').cells[1]).toBe('x')
  })
})
