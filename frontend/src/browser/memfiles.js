// Náhradné úložisko databázy, keď prehliadač nedovolí úložisko súborov OPFS
// (napr. Safari a ostatné prehliadače na iPhone hlásia „WebKit encountered
// an internal error“). Databázy sú v pamäti (VFS memdb, zdieľané medzi
// spojeniami podľa mena) a ich obsah sa po zmenách ukladá do IndexedDB.
// Ak nejde ani IndexedDB, údaje vydržia iba do zatvorenia stránky.

/**
 * Súbory databáz v pamäti. Každé meno drží jedno „kotviace“ spojenie —
 * pamäťová databáza memdb zanikne, keď sa zatvorí jej posledné spojenie.
 */
export function createMemoryFiles(sqlite3) {
  const { oo1, capi, wasm } = sqlite3
  const anchors = new Map()
  const uri = (name) => `file:${name}?vfs=memdb`
  const anchor = (name) => {
    if (!anchors.has(name)) anchors.set(name, new oo1.DB(uri(name), 'c'))
    return anchors.get(name)
  }
  const files = {
    has: (name) => anchors.has(name),
    open(name, readonly) {
      anchor(name)
      return new oo1.DB(uri(name), readonly ? 'r' : 'c')
    },
    exportFile: (name) => capi.sqlite3_js_db_export(anchor(name)),
    /** Nahradí obsah databázy bajtmi súboru SQLite. */
    importDb(name, bytes) {
      // súbor sa načíta do dočasného spojenia a záloha (backup API) ho
      // prepíše do zdieľanej databázy — deserialize by spojenie od zdieľanej
      // databázy odpojil
      const tmp = new oo1.DB(':memory:')
      try {
        const p = wasm.allocFromTypedArray(bytes)
        const rc = capi.sqlite3_deserialize(tmp, 'main', p, bytes.length, bytes.length, capi.SQLITE_DESERIALIZE_FREEONCLOSE | capi.SQLITE_DESERIALIZE_RESIZEABLE)
        if (rc) throw new Error(`súbor databázy sa nenačítal (${capi.sqlite3_js_rc_str(rc)})`)
        const bk = capi.sqlite3_backup_init(anchor(name), 'main', tmp, 'main')
        if (!bk) throw new Error(`databáza ${name} sa nedá prepísať`)
        const step = capi.sqlite3_backup_step(bk, -1)
        capi.sqlite3_backup_finish(bk)
        if (step !== capi.SQLITE_DONE) throw new Error(`databáza ${name} sa neprepísala (${capi.sqlite3_js_rc_str(step)})`)
      } finally {
        tmp.close()
      }
    },
    unlink(name) {
      anchors.get(name)?.close()
      anchors.delete(name)
    },
    /** Zatvorí všetky databázy (testy; stránka ich drží až do zatvorenia). */
    close() {
      for (const name of [...anchors.keys()]) files.unlink(name)
    },
  }
  return files
}

/** Malé úložisko súborov v IndexedDB (kľúč = meno súboru, hodnota = bajty). */
export async function openIdbFiles(idb = globalThis.indexedDB, dbName = 'fiskus-databaza') {
  if (!idb) throw new Error('IndexedDB nie je k dispozícii')
  const req = (r) =>
    new Promise((resolve, reject) => {
      r.onsuccess = () => resolve(r.result)
      r.onerror = () => reject(r.error)
    })
  const open = idb.open(dbName, 1)
  open.onupgradeneeded = () => open.result.createObjectStore('files')
  const db = await req(open)
  const store = (mode) => db.transaction('files', mode).objectStore('files')
  const files = {
    get: (name) => req(store('readonly').get(name)),
    put: (name, bytes) => req(store('readwrite').put(bytes, name)),
    del: (name) => req(store('readwrite').delete(name)),
  }
  await files.get('/skuska') // prehliadač, ktorý IndexedDB síce má, ale nedovolí, zlyhá tu
  return files
}

/**
 * Úložisko v pamäti s ukladaním do IndexedDB (idb môže byť null — potom
 * iba v pamäti). Rozhranie je rovnaké ako pri OPFS vo workeri.
 */
export async function memoryStorage(sqlite3, idb, { db, restore, delay = 400 }) {
  const files = createMemoryFiles(sqlite3)
  if (idb) {
    const staged = await idb.get(restore)
    const data = staged ?? (await idb.get(db))
    if (data) files.importDb(db, data)
    if (staged) {
      await idb.put(db, staged)
      await idb.del(restore)
    }
  }
  let timer = null
  let saving = Promise.resolve()
  const save = () => {
    if (!idb || !files.has(db)) return saving
    saving = saving.then(() => idb.put(db, files.exportFile(db))).catch(() => {}) // ďalší pokus pri ďalšej zmene
    return saving
  }
  return {
    mode: idb ? 'indexeddb' : 'memory',
    open: (name, readonly) => files.open(name, readonly),
    // kópia na zálohu (VACUUM INTO) v pamäti po skončení príkazu zanikne,
    // preto sa pri neznámom mene vráti hlavná databáza
    readFile: (name) => files.exportFile(files.has(name) ? name : db),
    async writeFile(name, bytes) {
      files.importDb(name, bytes)
      if (idb && name === restore) await idb.put(restore, bytes)
    },
    removeFile(name) {
      files.unlink(name)
      if (idb && name === restore) idb.del(restore).catch(() => {})
    },
    /** Po zmene údajov: uloží databázu do IndexedDB (s malým oneskorením). */
    changed() {
      clearTimeout(timer)
      timer = setTimeout(save, delay)
    },
    save,
    close: () => files.close(),
  }
}
