// Pripraví globalThis.fiskusSQL v Node.js pre testy Go zostavené pre
// prehliadač (GOOS=js GOARCH=wasm): databázy sú v pamäti, pomenované
// podľa cesty a zachované aj po zatvorení (test môže databázu znova otvoriť).
//
//   NODE_OPTIONS=--import=<cesta k tomuto súboru> go test -tags sqlite …
import sqlite3InitModule from '@sqlite.org/sqlite-wasm'
import { createBridge } from '../src/browser/sqlbridge.js'

const sqlite3 = await sqlite3InitModule({ print: () => {}, printErr: () => {} })
const kept = new Map()

function openDb(name) {
  let db = kept.get(name)
  if (!db) {
    db = new sqlite3.oo1.DB(':memory:')
    kept.set(name, db)
  }
  return {
    exec: (...a) => db.exec(...a),
    prepare: (...a) => db.prepare(...a),
    changes: (...a) => db.changes(...a),
    get pointer() {
      return db.pointer
    },
    close() {},
  }
}

globalThis.fiskusSQL = createBridge(sqlite3, openDb)
