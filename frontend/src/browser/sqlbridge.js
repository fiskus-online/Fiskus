// Most medzi ovládačom Go (backend/internal/store/sqlite/driver_js.go)
// a SQLite vo WebAssembly. Go volá tieto funkcie synchrónne.
//
// openDb(name, readonly) vráti otvorenú databázu oo1 (v prehliadači
// z úložiska OPFS, v testoch v pamäti).

export function createBridge(sqlite3, openDb) {
  const { capi } = sqlite3
  const dbs = new Map()
  let next = 1

  const db = (id) => {
    const d = dbs.get(id)
    if (!d) throw new Error('databáza je zatvorená')
    return d
  }

  return {
    open(name, readonly) {
      const id = next++
      dbs.set(id, openDb(name, readonly))
      return id
    },

    close(id) {
      dbs.get(id)?.close()
      dbs.delete(id)
    },

    /** Vykoná príkaz (bez parametrov aj viac príkazov naraz) → [zmenené riadky, posledné id]. */
    exec(id, sql, args) {
      const d = db(id)
      if (!args?.length) {
        d.exec(sql)
      } else {
        const stmt = d.prepare(sql)
        try {
          stmt.bind(args)
          while (stmt.step()) {
            /* príkaz s RETURNING cez Exec — riadky sa zahodia */
          }
        } finally {
          stmt.finalize()
        }
      }
      return [d.changes(), Number(capi.sqlite3_last_insert_rowid(d.pointer))]
    },

    /**
     * Dotaz → { columns, rows, cells }, kde cells je ploché pole
     * [typ, hodnota, …] po riadkoch (typ podľa sqlite3_column_type).
     * Celé čísla mimo rozsahu JavaScriptu sa vrátia ako reťazec.
     */
    query(id, sql, args) {
      const stmt = db(id).prepare(sql)
      try {
        if (args?.length) stmt.bind(args)
        const columns = stmt.getColumnNames()
        const n = columns.length
        const cells = []
        let rows = 0
        while (stmt.step()) {
          for (let i = 0; i < n; i++) {
            const t = capi.sqlite3_column_type(stmt.pointer, i)
            let v = null
            if (t === capi.SQLITE_INTEGER) {
              v = stmt.get(i, capi.SQLITE_INTEGER)
              if (typeof v === 'bigint') v = Number.isSafeInteger(Number(v)) ? Number(v) : v.toString()
            } else if (t === capi.SQLITE_FLOAT) {
              v = stmt.get(i, capi.SQLITE_FLOAT)
            } else if (t === capi.SQLITE_TEXT) {
              v = stmt.get(i, capi.SQLITE_TEXT)
            } else if (t === capi.SQLITE_BLOB) {
              v = stmt.get(i, capi.SQLITE_BLOB) ?? new Uint8Array(0)
            }
            cells.push(t, v)
          }
          rows++
        }
        return { columns, rows, cells }
      } finally {
        stmt.finalize()
      }
    },
  }
}
