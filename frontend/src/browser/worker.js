// Fiškus v prehliadači — web worker: SQLite (WebAssembly, úložisko OPFS)
// a Fiškus v Go (WebAssembly). Odpovedá na požiadavky API, ktoré mu
// posiela stránka (boot.js) zo service workera.
import sqlite3InitModule from '@sqlite.org/sqlite-wasm'
import './wasm_exec.js' // z inštalácie Go (make browser), definuje globalThis.Go
import fiskusWasm from './fiskus.wasm.gz?url'
import { createBridge } from './sqlbridge.js'
import { memoryStorage, openIdbFiles } from './memfiles.js'

/* global __WASM_SIZE__ */
const wasmSize = typeof __WASM_SIZE__ === 'number' ? __WASM_SIZE__ : 0
const DB = '/fiskus.db'
const RESTORE = '/obnova.db'

let markReady
const ready = new Promise((resolve) => (markReady = resolve))

// databázu má otvorenú iná karta — to nie je chyba úložiska, náhrada nepomôže
const LOCKED = /Access Handle|NoModificationAllowed/i

let storage = null

/**
 * Úložisko databázy: OPFS, a keď ho prehliadač nedovolí (Safari a ostatné
 * prehliadače na iPhone: „WebKit encountered an internal error“), databáza
 * v pamäti ukladaná do IndexedDB, v krajnom prípade iba v pamäti.
 */
async function openStorage(sqlite3) {
  try {
    const pool = await sqlite3.installOpfsSAHPoolVfs({ name: 'fiskus', initialCapacity: 12 })
    // obnova zo zálohy pripravená pred opätovným načítaním (cmd/fiskus-web)
    if (pool.getFileNames().includes(RESTORE)) {
      await pool.importDb(DB, pool.exportFile(RESTORE))
      pool.unlink(RESTORE)
    }
    return {
      mode: 'opfs',
      open: (name) => new pool.OpfsSAHPoolDb(name),
      readFile: (name) => pool.exportFile(name),
      writeFile: (name, bytes) => pool.importDb(name, bytes),
      removeFile(name) {
        if (pool.getFileNames().includes(name)) pool.unlink(name)
      },
      changed() {},
    }
  } catch (err) {
    if (LOCKED.test(err.message)) throw err
    console.warn('Úložisko OPFS nejde, databáza bude v IndexedDB:', err)
  }
  let idb = null
  try {
    idb = await openIdbFiles()
  } catch (err) {
    console.warn('Ani IndexedDB nejde, databáza bude iba v pamäti:', err)
  }
  return memoryStorage(sqlite3, idb, { db: DB, restore: RESTORE })
}

async function start() {
  const sqlite3 = await sqlite3InitModule({ print: () => {}, printErr: () => {} })
  storage = await openStorage(sqlite3)

  const bridge = createBridge(sqlite3, (name, readonly) => storage.open(name, readonly))
  bridge.readFile = (name) => storage.readFile(name)
  bridge.writeFile = (name, bytes) => storage.writeFile(name, bytes)
  bridge.removeFile = (name) => storage.removeFile(name)
  globalThis.fiskusSQL = bridge
  globalThis.fiskusReady = () => {
    markReady()
    storage.changed() // nová databáza (po migráciách) sa hneď uloží
    postMessage({ type: 'ready', storage: storage.mode })
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
  const bytes = await unpack(new Blob(chunks))
  const go = new globalThis.Go()
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject)
  go.run(instance).then(() => postMessage({ type: 'failed', message: 'Fiškus sa neočakávane ukončil. Načítajte stránku znova.' }))
}

// Program je uložený skomprimovaný (gzip). Ak ho hosting poslal už
// rozbalený (Content-Encoding), použije sa tak, ako prišiel.
async function unpack(blob) {
  const head = new Uint8Array(await blob.slice(0, 2).arrayBuffer())
  if (head[0] !== 0x1f || head[1] !== 0x8b) return new Uint8Array(await blob.arrayBuffer())
  return new Uint8Array(await new Response(blob.stream().pipeThrough(new DecompressionStream('gzip'))).arrayBuffer())
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
    if (e.data.req.method !== 'GET' && e.data.req.method !== 'HEAD') storage.changed()
    port.postMessage(r, [r.body])
  } catch (err) {
    const r = errorResponse(`Fiškus neodpovedá: ${err.message}`)
    port.postMessage(r, [r.body])
  }
}

start().catch((err) => {
  const msg = LOCKED.test(err.message)
    ? 'Databázu má otvorenú iná karta alebo okno s Fiškusom. Zatvorte ho a skúste znova.'
    : err.message
  postMessage({ type: 'failed', message: msg })
})
