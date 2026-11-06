//go:build sqlite && js && wasm

package sqlite

// Ovládač „sqlite“ pre database/sql vo verzii pre prehliadač: volá SQLite
// skompilované do WebAssembly (@sqlite.org/sqlite-wasm) cez objekt
// globalThis.fiskusSQL, ktorý pripraví frontend (src/browser/sqlbridge.js).
// Volania sú synchrónne — Go aj SQLite bežia v tom istom web workeri.

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"strconv"
	"strings"
	"syscall/js"
	"time"
)

func init() { sql.Register("sqlite", jsDriver{}) }

// typy stĺpcov SQLite (sqlite3_column_type)
const (
	colInteger = 1
	colFloat   = 2
	colText    = 3
	colBlob    = 4
	colNull    = 5
)

type jsDriver struct{}

// Open: dsn je „file:<meno>?_pragma=…“ ako pri modernc. Pragmy sa vykonajú
// po otvorení; journal_mode sa vynechá (úložisko prehliadača WAL nepozná).
func (jsDriver) Open(dsn string) (driver.Conn, error) {
	bridge := js.Global().Get("fiskusSQL")
	if bridge.IsUndefined() {
		return nil, errors.New("SQLite v prehliadači nie je pripravené (chýba fiskusSQL)")
	}
	name, query, _ := strings.Cut(strings.TrimPrefix(dsn, "file:"), "?")
	q, _ := url.ParseQuery(query)
	var id js.Value
	if err := catch(func() { id = bridge.Call("open", name, q.Get("mode") == "ro") }); err != nil {
		return nil, err
	}
	c := &jsConn{bridge: bridge, id: id}
	for _, p := range q["_pragma"] {
		if strings.HasPrefix(p, "journal_mode") {
			continue
		}
		k, v, ok := strings.Cut(strings.TrimSuffix(p, ")"), "(")
		stmt := "PRAGMA " + k
		if ok {
			stmt += " = " + v
		}
		if _, err := c.ExecContext(context.Background(), stmt, nil); err != nil {
			c.Close()
			return nil, err
		}
	}
	return c, nil
}

// catch prevedie výnimku z JavaScriptu (napr. SQLite3Error) na chybu Go.
func catch(f func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if je, ok := r.(js.Error); ok {
				err = errors.New(strings.TrimPrefix(je.Value.Get("message").String(), "SQLITE_ERROR: "))
				return
			}
			err = fmt.Errorf("sqlite: %v", r)
		}
	}()
	f()
	return nil
}

type jsConn struct {
	bridge js.Value
	id     js.Value
}

func (c *jsConn) Prepare(query string) (driver.Stmt, error) { return &jsStmt{c: c, query: query}, nil }

func (c *jsConn) Close() error {
	return catch(func() { c.bridge.Call("close", c.id) })
}

func (c *jsConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *jsConn) BeginTx(ctx context.Context, _ driver.TxOptions) (driver.Tx, error) {
	if _, err := c.ExecContext(ctx, "BEGIN", nil); err != nil {
		return nil, err
	}
	return jsTx{c}, nil
}

func (c *jsConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	var r js.Value
	if err := catch(func() { r = c.bridge.Call("exec", c.id, query, jsArgs(args)) }); err != nil {
		return nil, err
	}
	return jsResult{changes: int64(r.Index(0).Float()), lastID: int64(r.Index(1).Float())}, nil
}

func (c *jsConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	var r js.Value
	if err := catch(func() { r = c.bridge.Call("query", c.id, query, jsArgs(args)) }); err != nil {
		return nil, err
	}
	cols := r.Get("columns")
	names := make([]string, cols.Length())
	for i := range names {
		names[i] = cols.Index(i).String()
	}
	return &jsRows{cols: names, cells: r.Get("cells"), n: r.Get("rows").Int()}, nil
}

// CheckNamedValue prijme hodnoty, ktoré vie jsArgs previesť.
func (c *jsConn) CheckNamedValue(nv *driver.NamedValue) error {
	switch v := nv.Value.(type) {
	case int:
		nv.Value = int64(v)
	case int32:
		nv.Value = int64(v)
	case uint32:
		nv.Value = int64(v)
	case uint64:
		if v > math.MaxInt64 {
			return fmt.Errorf("sqlite: číslo %d je príliš veľké", v)
		}
		nv.Value = int64(v)
	case float32:
		nv.Value = float64(v)
	default:
		var err error
		nv.Value, err = driver.DefaultParameterConverter.ConvertValue(v)
		return err
	}
	return nil
}

// jsArgs prevedie parametre dotazu na pole pre fiskusSQL.
func jsArgs(args []driver.NamedValue) js.Value {
	arr := js.Global().Get("Array").New(len(args))
	for i, a := range args {
		var v any
		switch x := a.Value.(type) {
		case nil:
			v = nil
		case int64:
			if x > 1<<53 || x < -(1<<53) {
				v = js.Global().Call("BigInt", strconv.FormatInt(x, 10))
			} else {
				v = float64(x)
			}
		case float64:
			v = x
		case bool:
			if x {
				v = 1
			} else {
				v = 0
			}
		case string:
			v = x
		case []byte:
			b := js.Global().Get("Uint8Array").New(len(x))
			js.CopyBytesToJS(b, x)
			v = b
		case time.Time:
			v = x.Format("2006-01-02 15:04:05.999999999-07:00")
		default:
			v = fmt.Sprint(x)
		}
		arr.SetIndex(i, v)
	}
	return arr
}

type jsTx struct{ c *jsConn }

func (t jsTx) Commit() error {
	_, err := t.c.ExecContext(context.Background(), "COMMIT", nil)
	return err
}

func (t jsTx) Rollback() error {
	_, err := t.c.ExecContext(context.Background(), "ROLLBACK", nil)
	return err
}

type jsResult struct{ changes, lastID int64 }

func (r jsResult) LastInsertId() (int64, error) { return r.lastID, nil }
func (r jsResult) RowsAffected() (int64, error) { return r.changes, nil }

type jsStmt struct {
	c     *jsConn
	query string
}

func (s *jsStmt) Close() error  { return nil }
func (s *jsStmt) NumInput() int { return -1 }

func (s *jsStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.c.ExecContext(context.Background(), s.query, named(args))
}

func (s *jsStmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.c.QueryContext(context.Background(), s.query, named(args))
}

func (s *jsStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return s.c.ExecContext(ctx, s.query, args)
}

func (s *jsStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	return s.c.QueryContext(ctx, s.query, args)
}

func named(args []driver.Value) []driver.NamedValue {
	out := make([]driver.NamedValue, len(args))
	for i, a := range args {
		out[i] = driver.NamedValue{Ordinal: i + 1, Value: a}
	}
	return out
}

// jsRows číta výsledok dotazu: cells je ploché pole [typ, hodnota, typ, hodnota, …]
// po riadkoch.
type jsRows struct {
	cols  []string
	cells js.Value
	n     int
	row   int
}

func (r *jsRows) Columns() []string { return r.cols }
func (r *jsRows) Close() error      { return nil }

func (r *jsRows) Next(dest []driver.Value) error {
	if r.row >= r.n {
		return io.EOF
	}
	base := r.row * len(r.cols) * 2
	for i := range dest {
		t := r.cells.Index(base + 2*i).Int()
		v := r.cells.Index(base + 2*i + 1)
		switch t {
		case colInteger:
			if v.Type() == js.TypeString { // mimo rozsahu čísel JavaScriptu
				n, err := strconv.ParseInt(v.String(), 10, 64)
				if err != nil {
					return err
				}
				dest[i] = n
			} else {
				dest[i] = int64(v.Float())
			}
		case colFloat:
			dest[i] = v.Float()
		case colText:
			dest[i] = v.String()
		case colBlob:
			b := make([]byte, v.Length())
			js.CopyBytesToGo(b, v)
			dest[i] = b
		default:
			dest[i] = nil
		}
	}
	r.row++
	return nil
}
