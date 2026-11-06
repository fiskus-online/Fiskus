// Fiškus v prehliadači — web worker: SQLite (WebAssembly, úložisko OPFS)
// a Fiškus v Go (WebAssembly). Odpovedá na požiadavky API, ktoré mu
// posiela stránka (boot.js) zo service workera.
import sqlite3InitModule from '@sqlite.org/sqlite-wasm'
import './wasm_exec.js' // z inštalácie Go (make browser), definuje globalThis.Go
import fiskusWasm from './fiskus.wasm?url'
import { createBridge } from './sqlbridge.js'

/* global __WASM_SIZE__ */
const wasmSize = typeof __WASM_SIZE__ === 'number' ? __WASM_SIZE__ : 0
const DB = '/fiskus.db'
const RESTORE = '/obnova.db'

let markReady
const ready = new Promise((resolve) => (markReady = resolve))

async function start() {
  const sqlite3 = await sqlite3InitModule({ print: () => {}, printErr: () => {} })
  const pool = await sqlite3.installOpfsSAHPoolVfs({ name: 'fiskus', initialCapacity: 12 })

  // obnova zo zálohy pripravená pred opätovným načítaním (cmd/fiskus-web)
  if (pool.getFileNames().includes(RESTORE)) {
    await pool.importDb(DB, pool.exportFile(RESTORE))
    pool.unlink(RESTORE)
  }

  const bridge = createBridge(sqlite3, (name) => new pool.OpfsSAHPoolDb(name))
  bridge.readFile = (name) => pool.exportFile(name)
  bridge.writeFile = (name, bytes) => pool.importDb(name, bytes)
  bridge.removeFile = (name) => {
    if (pool.getFileNames().includes(name)) pool.unlink(name)
  }
  globalThis.fiskusSQL = bridge
  globalThis.fiskusReady = () => {
    markReady()
    postMessage({ type: 'ready' })
  }
  globalThis.fiskusFailed = (message) => postMessage({ type: 'failed', message })

  // program sa sťahuje raz, potom je v cache prehliadača (súbor má v mene odtlačok)
  const res = await fetch(fiskusWasm)
  if (!res.ok) throw new Error(`Program sa nepodarilo stiahnuť (${res.status}).`)
  const reader = res.body.getReader()
  const chunks = []
  let loaded = 0
  let last = 0
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    chunks.push(value)
    loaded += value.length
    if (loaded - last > 512 * 1024) {
      last = loaded
      postMessage({ type: 'progress', loaded, total: wasmSize })
    }
  }
  postMessage({ type: 'progress', loaded, total: loaded })
  const bytes = new Uint8Array(loaded)
  let off = 0
  for (const c of chunks) {
    bytes.set(c, off)
    off += c.length
  }
  const go = new globalThis.Go()
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject)
  go.run(instance).then(() => postMessage({ type: 'failed', message: 'Fiškus sa neočakávane ukončil. Načítajte stránku znova.' }))
}

const errorResponse = (message) => ({
  status: 503,
  headers: [['Content-Type', 'application/json']],
  body: new TextEncoder().encode(JSON.stringify({ error: message })).buffer,
})

onmessage = async (e) => {
  if (e.data?.type !== 'api') return
  const port = e.ports[0]
  await ready
  try {
    const r = await globalThis.fiskusServe(e.data.req)
    port.postMessage(r, [r.body])
  } catch (err) {
    const r = errorResponse(`Fiškus neodpovedá: ${err.message}`)
    port.postMessage(r, [r.body])
  }
}

start().catch((err) => {
  const msg = /Access Handle|NoModificationAllowed/i.test(err.message)
    ? 'Databázu má otvorenú iná karta alebo okno s Fiškusom. Zatvorte ho a skúste znova.'
    : err.message
  postMessage({ type: 'failed', message: msg })
})
