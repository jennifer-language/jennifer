// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

//go:build !tinygo

// Standard-Go implementation of the `sql` library over database/sql. The blank
// imports register the two pure-Go drivers (MySQL as "mysql", Postgres via pgx's
// stdlib shim as "pgx"). Under TinyGo the sqllib_tiny.go stub is selected instead
// and the driver trees are never compiled.

package sqllib

import (
	"database/sql"
	"fmt"
	"strconv"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/devio"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// -------- registries --------

type rowState struct {
	// rmu guards a single cursor against concurrent use: database/sql's *sql.Rows
	// is not safe for concurrent Next/Scan, and cur is read by the accessors. A
	// cursor is inherently sequential, so sharing one across spawns is misuse;
	// this makes that misuse a serialized no-op rather than a Go data race.
	rmu     sync.Mutex
	id      int64
	rows    *sql.Rows
	cols    []string
	cur     []interface{} // the current row's scanned values; nil before next() / after exhaustion
	scanned bool
	done    bool // the cursor has been fully consumed (Next returned false)
}

var (
	mu    sync.Mutex
	conns = map[int64]*sql.DB{}
	rows  = map[int64]*rowState{}
	txs   = map[int64]*sql.Tx{}
	stmts = map[int64]*sql.Stmt{}
	nid   int64
)

// ResetForTest closes everything and clears the registries between tests.
func ResetForTest() {
	mu.Lock()
	defer mu.Unlock()
	for _, r := range rows {
		if r != nil && r.rows != nil {
			_ = r.rows.Close()
		}
	}
	for _, s := range stmts {
		if s != nil {
			_ = s.Close()
		}
	}
	for _, t := range txs {
		if t != nil {
			_ = t.Rollback()
		}
	}
	for _, d := range conns {
		if d != nil {
			_ = d.Close()
		}
	}
	conns, rows, txs, stmts = map[int64]*sql.DB{}, map[int64]*rowState{}, map[int64]*sql.Tx{}, map[int64]*sql.Stmt{}
	nid = 0
}

// Handle plumbing (build a `sql.Name{id}` value, pull the id back out, extract
// typed positional args) is the shared integer-registry boilerplate from
// `devio` - the same helpers the device libraries use.

func handle(name string, id int64) Value {
	return devio.Handle(LibraryName, name, id)
}

func handleID(fn string, v Value, name string) (int64, error) {
	return devio.HandleID(fn, v, LibraryName, name)
}

// -------- param binding (placeholders only, never string interpolation) --------

func toDriverArgs(fn string, params []Value) ([]interface{}, error) {
	// A single list argument is spread into the parameter sequence: since a
	// list cannot itself bind to a placeholder, `sql.exec(conn, sql, $list)` is
	// unambiguous and lets a caller pass a dynamic-length parameter list (which
	// the orm module needs) where the variadic call site has a fixed arity. The
	// plain variadic form `sql.exec(conn, sql, a, b, c)` is unaffected.
	if len(params) == 1 && params[0].Kind == interpreter.KindList {
		params = params[0].List
	}
	out := make([]interface{}, len(params))
	for i, p := range params {
		switch p.Kind {
		case interpreter.KindNull:
			out[i] = nil
		case interpreter.KindInt:
			out[i] = p.Int
		case interpreter.KindFloat:
			out[i] = p.Float
		case interpreter.KindString:
			out[i] = p.Str
		case interpreter.KindBool:
			out[i] = p.Bool
		case interpreter.KindBytes:
			out[i] = p.Bytes
		default:
			return nil, fmt.Errorf("%s: parameter %d has type %s, which cannot bind to a placeholder (use int/float/string/bool/bytes/null)", fn, i+1, p.Kind)
		}
	}
	return out, nil
}

// querier is satisfied by *sql.DB and *sql.Tx, so sql.query / sql.exec accept a
// Connection or a Tx uniformly.
type querier interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
	Exec(query string, args ...interface{}) (sql.Result, error)
}

func resolveQuerier(fn string, v Value) (querier, error) {
	if v.Kind != interpreter.KindStruct || v.StructNS != LibraryName {
		return nil, fmt.Errorf("%s: first argument must be a sql.Connection or sql.Tx, got %s", fn, v.Kind)
	}
	mu.Lock()
	defer mu.Unlock()
	switch v.StructName {
	case "Connection":
		id, err := handleID(fn, v, "Connection")
		if err != nil {
			return nil, err
		}
		if d, ok := conns[id]; ok {
			return d, nil
		}
		return nil, fmt.Errorf("%s: sql.Connection id %d is not open", fn, id)
	case "Tx":
		id, err := handleID(fn, v, "Tx")
		if err != nil {
			return nil, err
		}
		if t, ok := txs[id]; ok {
			return t, nil
		}
		return nil, fmt.Errorf("%s: sql.Tx id %d is not open (committed or rolled back?)", fn, id)
	}
	return nil, fmt.Errorf("%s: first argument must be a sql.Connection or sql.Tx, got sql.%s", fn, v.StructName)
}

// -------- connection --------

// sql.open(driver, dsn) -> sql.Connection. driver is "mysql" / "mariadb" or
// "postgres" / "postgresql". Pings so a bad DSN / unreachable server fails here.
func openFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("sql.open expects 2 arguments (driver, dsn), got %d", len(args))
	}
	driver, err := devio.StringArg("sql.open", args, 0, "driver")
	if err != nil {
		return interpreter.Null(), err
	}
	dsn, err := devio.StringArg("sql.open", args, 1, "dsn")
	if err != nil {
		return interpreter.Null(), err
	}
	var goDriver string
	switch driver {
	case "mysql", "mariadb":
		goDriver = "mysql"
	case "postgres", "postgresql":
		goDriver = "pgx"
	default:
		return interpreter.Null(), fmt.Errorf(`sql.open: unknown driver %q; use "mysql" / "mariadb" or "postgres" / "postgresql"`, driver)
	}
	db, oerr := sql.Open(goDriver, dsn)
	if oerr != nil {
		return interpreter.Null(), fmt.Errorf("sql.open: %v", oerr)
	}
	if perr := db.Ping(); perr != nil {
		_ = db.Close()
		return interpreter.Null(), fmt.Errorf("sql.open: %v", perr)
	}
	mu.Lock()
	nid++
	id := nid
	conns[id] = db
	mu.Unlock()
	return handle("Connection", id), nil
}

// sql.close(conn) -> null.
func closeFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("sql.close expects 1 argument (sql.Connection), got %d", len(args))
	}
	id, err := handleID("sql.close", args[0], "Connection")
	if err != nil {
		return interpreter.Null(), err
	}
	mu.Lock()
	db, ok := conns[id]
	if !ok {
		mu.Unlock()
		return interpreter.Null(), fmt.Errorf("sql.close: sql.Connection id %d is not open (already closed?)", id)
	}
	delete(conns, id)
	mu.Unlock()
	if cerr := db.Close(); cerr != nil {
		return interpreter.Null(), fmt.Errorf("sql.close: %v", cerr)
	}
	return interpreter.Null(), nil
}

// -------- query / exec --------

func runQuery(fn string, q querier, args []Value) (Value, error) {
	sqlText, err := devio.StringArg(fn, args, 1, "sql")
	if err != nil {
		return interpreter.Null(), err
	}
	params, err := toDriverArgs(fn, args[2:])
	if err != nil {
		return interpreter.Null(), err
	}
	rs, qerr := q.Query(sqlText, params...)
	if qerr != nil {
		return interpreter.Null(), fmt.Errorf("%s: %v", fn, qerr)
	}
	cols, cerr := rs.Columns()
	if cerr != nil {
		_ = rs.Close()
		return interpreter.Null(), fmt.Errorf("%s: %v", fn, cerr)
	}
	mu.Lock()
	nid++
	id := nid
	rows[id] = &rowState{id: id, rows: rs, cols: cols}
	mu.Unlock()
	return handle("Rows", id), nil
}

func runExec(fn string, q querier, args []Value) (Value, error) {
	sqlText, err := devio.StringArg(fn, args, 1, "sql")
	if err != nil {
		return interpreter.Null(), err
	}
	params, err := toDriverArgs(fn, args[2:])
	if err != nil {
		return interpreter.Null(), err
	}
	res, eerr := q.Exec(sqlText, params...)
	if eerr != nil {
		return interpreter.Null(), fmt.Errorf("%s: %v", fn, eerr)
	}
	return resultValue(res), nil
}

// resultValue builds a sql.Result{affected, lastId}. lastInsertId is not
// supported by every driver (Postgres); a driver error there yields -1 rather
// than failing the whole exec.
func resultValue(res sql.Result) Value {
	affected := int64(-1)
	if a, err := res.RowsAffected(); err == nil {
		affected = a
	}
	lastID := int64(-1)
	if l, err := res.LastInsertId(); err == nil {
		lastID = l
	}
	return interpreter.NamespacedStructVal(LibraryName, "Result", []interpreter.StructField{
		{Name: "affected", Value: interpreter.IntVal(affected)},
		{Name: "lastId", Value: interpreter.IntVal(lastID)},
	})
}

// sql.query(target, sql, params...) -> sql.Rows. target is a Connection or Tx.
func queryFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) < 2 {
		return interpreter.Null(), fmt.Errorf("sql.query expects at least (target, sql), got %d args", len(args))
	}
	q, err := resolveQuerier("sql.query", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	return runQuery("sql.query", q, args)
}

// sql.exec(target, sql, params...) -> sql.Result. target is a Connection or Tx.
func execFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) < 2 {
		return interpreter.Null(), fmt.Errorf("sql.exec expects at least (target, sql), got %d args", len(args))
	}
	q, err := resolveQuerier("sql.exec", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	return runExec("sql.exec", q, args)
}

// -------- row cursor --------

func resolveRows(fn string, v Value) (*rowState, error) {
	id, err := handleID(fn, v, "Rows")
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	r, ok := rows[id]
	if !ok {
		return nil, fmt.Errorf("%s: sql.Rows id %d is not open (exhausted or closed)", fn, id)
	}
	return r, nil
}

// releaseRow drops a spent cursor from the registry. Deleting a missing key is
// a no-op, so a concurrent closeRows racing an exhaustion is harmless.
func releaseRow(id int64) {
	mu.Lock()
	delete(rows, id)
	mu.Unlock()
}

// sql.next(rows) -> bool. Advances to the next row and scans it; returns false at
// the end (closing the cursor and releasing the handle - a spent cursor does not
// linger in the registry).
func nextFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("sql.next expects 1 argument (sql.Rows), got %d", len(args))
	}
	r, err := resolveRows("sql.next", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	r.rmu.Lock()
	defer r.rmu.Unlock()
	if r.done {
		return interpreter.BoolVal(false), nil
	}
	if !r.rows.Next() {
		rerr := r.rows.Err()
		// Exhausted: close the cursor, clear the current row so a stray accessor
		// errors instead of returning the last row's stale data, and release the
		// registry entry so query loops do not accumulate spent cursors.
		r.done = true
		r.cur = nil
		r.scanned = false
		_ = r.rows.Close()
		releaseRow(r.id)
		if rerr != nil {
			return interpreter.Null(), fmt.Errorf("sql.next: %v", rerr)
		}
		return interpreter.BoolVal(false), nil
	}
	dest := make([]interface{}, len(r.cols))
	holders := make([]interface{}, len(r.cols))
	for i := range dest {
		holders[i] = &dest[i]
	}
	if serr := r.rows.Scan(holders...); serr != nil {
		// A failed scan must not leave the previous row readable.
		r.cur = nil
		r.scanned = false
		return interpreter.Null(), fmt.Errorf("sql.next: %v", serr)
	}
	r.cur = dest
	r.scanned = true
	return interpreter.BoolVal(true), nil
}

// sql.columns(rows) -> list of string.
func columnsFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("sql.columns expects 1 argument (sql.Rows), got %d", len(args))
	}
	r, err := resolveRows("sql.columns", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	out := make([]Value, len(r.cols))
	for i, c := range r.cols {
		out[i] = interpreter.StringVal(c)
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeString), out), nil
}

// column returns the current row's raw value for a column named or indexed by
// col. The caller holds r.rmu.
func (r *rowState) column(fn string, col Value) (interface{}, error) {
	if r.done {
		return nil, fmt.Errorf("%s: the cursor is exhausted (sql.next returned false)", fn)
	}
	if !r.scanned || r.cur == nil {
		return nil, fmt.Errorf("%s: no current row - call sql.next first", fn)
	}
	switch col.Kind {
	case interpreter.KindInt:
		i := int(col.Int)
		if i < 0 || i >= len(r.cur) {
			return nil, fmt.Errorf("%s: column index %d out of range (0..%d)", fn, i, len(r.cur)-1)
		}
		return r.cur[i], nil
	case interpreter.KindString:
		for i, c := range r.cols {
			if c == col.Str {
				return r.cur[i], nil
			}
		}
		return nil, fmt.Errorf("%s: no column named %q", fn, col.Str)
	}
	return nil, fmt.Errorf("%s: column must be a name (string) or a 0-based index (int)", fn)
}

func accessor(fn string, args []Value) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("%s expects 2 arguments (sql.Rows, column), got %d", fn, len(args))
	}
	r, err := resolveRows(fn, args[0])
	if err != nil {
		return nil, err
	}
	r.rmu.Lock()
	defer r.rmu.Unlock()
	return r.column(fn, args[1])
}

func asIntFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	v, err := accessor("sql.asInt", args)
	if err != nil {
		return interpreter.Null(), err
	}
	switch x := v.(type) {
	case int64:
		return interpreter.IntVal(x), nil
	case float64:
		return interpreter.IntVal(int64(x)), nil
	case bool:
		if x {
			return interpreter.IntVal(1), nil
		}
		return interpreter.IntVal(0), nil
	case []byte:
		return parseInt("sql.asInt", string(x))
	case string:
		return parseInt("sql.asInt", x)
	case nil:
		return interpreter.Null(), fmt.Errorf("sql.asInt: column is NULL (check sql.isNull first)")
	}
	return interpreter.Null(), fmt.Errorf("sql.asInt: column is not an integer (%T)", v)
}

func parseInt(fn, s string) (Value, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("%s: %q is not an integer", fn, s)
	}
	return interpreter.IntVal(n), nil
}

func asFloatFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	v, err := accessor("sql.asFloat", args)
	if err != nil {
		return interpreter.Null(), err
	}
	switch x := v.(type) {
	case float64:
		return interpreter.FloatVal(x), nil
	case int64:
		return interpreter.FloatVal(float64(x)), nil
	case bool:
		if x {
			return interpreter.FloatVal(1), nil
		}
		return interpreter.FloatVal(0), nil
	case []byte:
		return parseFloat(string(x))
	case string:
		return parseFloat(x)
	case nil:
		return interpreter.Null(), fmt.Errorf("sql.asFloat: column is NULL (check sql.isNull first)")
	}
	return interpreter.Null(), fmt.Errorf("sql.asFloat: column is not a number (%T)", v)
}

func parseFloat(s string) (Value, error) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("sql.asFloat: %q is not a number", s)
	}
	return interpreter.FloatVal(f), nil
}

// sql.asString always succeeds for a non-NULL column (like convert.toString).
func asStringFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	v, err := accessor("sql.asString", args)
	if err != nil {
		return interpreter.Null(), err
	}
	switch x := v.(type) {
	case string:
		return interpreter.StringVal(x), nil
	case []byte:
		return interpreter.StringVal(string(x)), nil
	case int64:
		return interpreter.StringVal(strconv.FormatInt(x, 10)), nil
	case float64:
		return interpreter.StringVal(strconv.FormatFloat(x, 'g', -1, 64)), nil
	case bool:
		return interpreter.StringVal(strconv.FormatBool(x)), nil
	case time.Time:
		return interpreter.StringVal(x.Format(time.RFC3339)), nil
	case nil:
		return interpreter.Null(), fmt.Errorf("sql.asString: column is NULL (check sql.isNull first)")
	}
	return interpreter.StringVal(fmt.Sprintf("%v", v)), nil
}

func asBoolFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	v, err := accessor("sql.asBool", args)
	if err != nil {
		return interpreter.Null(), err
	}
	switch x := v.(type) {
	case bool:
		return interpreter.BoolVal(x), nil
	case int64:
		return interpreter.BoolVal(x != 0), nil
	case nil:
		return interpreter.Null(), fmt.Errorf("sql.asBool: column is NULL (check sql.isNull first)")
	}
	return interpreter.Null(), fmt.Errorf("sql.asBool: column is not a bool (%T)", v)
}

func asBytesFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	v, err := accessor("sql.asBytes", args)
	if err != nil {
		return interpreter.Null(), err
	}
	switch x := v.(type) {
	case []byte:
		b := make([]byte, len(x))
		copy(b, x)
		return interpreter.BytesVal(b), nil
	case string:
		return interpreter.BytesVal([]byte(x)), nil
	case nil:
		return interpreter.Null(), fmt.Errorf("sql.asBytes: column is NULL (check sql.isNull first)")
	}
	return interpreter.Null(), fmt.Errorf("sql.asBytes: column is not bytes (%T)", v)
}

// sql.isNull(rows, column) -> bool.
func isNullFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	v, err := accessor("sql.isNull", args)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BoolVal(v == nil), nil
}

// sql.closeRows(rows) -> null. Closes a cursor early (before exhaustion).
// Always a no-op on a cursor that is already gone - exhaustion auto-releases
// the handle, so `defer sql.closeRows($rows)` must stay safe either way.
func closeRowsFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("sql.closeRows expects 1 argument (sql.Rows), got %d", len(args))
	}
	id, err := handleID("sql.closeRows", args[0], "Rows")
	if err != nil {
		return interpreter.Null(), err
	}
	mu.Lock()
	r, ok := rows[id]
	if !ok {
		mu.Unlock()
		return interpreter.Null(), nil // exhausted or already closed
	}
	delete(rows, id)
	mu.Unlock()
	// Take the cursor lock so we do not close *sql.Rows out from under an
	// in-flight sql.next on another goroutine.
	r.rmu.Lock()
	defer r.rmu.Unlock()
	if r.done {
		return interpreter.Null(), nil // already closed on exhaustion
	}
	if cerr := r.rows.Close(); cerr != nil {
		return interpreter.Null(), fmt.Errorf("sql.closeRows: %v", cerr)
	}
	return interpreter.Null(), nil
}

// -------- transactions --------

// sql.begin(conn) -> sql.Tx.
func beginFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("sql.begin expects 1 argument (sql.Connection), got %d", len(args))
	}
	id, err := handleID("sql.begin", args[0], "Connection")
	if err != nil {
		return interpreter.Null(), err
	}
	mu.Lock()
	db, ok := conns[id]
	mu.Unlock()
	if !ok {
		return interpreter.Null(), fmt.Errorf("sql.begin: sql.Connection id %d is not open", id)
	}
	tx, berr := db.Begin()
	if berr != nil {
		return interpreter.Null(), fmt.Errorf("sql.begin: %v", berr)
	}
	mu.Lock()
	nid++
	tid := nid
	txs[tid] = tx
	mu.Unlock()
	return handle("Tx", tid), nil
}

func endTx(fn string, args []Value, commit bool) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("%s expects 1 argument (sql.Tx), got %d", fn, len(args))
	}
	id, err := handleID(fn, args[0], "Tx")
	if err != nil {
		return interpreter.Null(), err
	}
	mu.Lock()
	tx, ok := txs[id]
	if !ok {
		mu.Unlock()
		return interpreter.Null(), fmt.Errorf("%s: sql.Tx id %d is not open (already committed or rolled back?)", fn, id)
	}
	delete(txs, id)
	mu.Unlock()
	var terr error
	if commit {
		terr = tx.Commit()
	} else {
		terr = tx.Rollback()
	}
	if terr != nil {
		return interpreter.Null(), fmt.Errorf("%s: %v", fn, terr)
	}
	return interpreter.Null(), nil
}

func commitFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	return endTx("sql.commit", args, true)
}
func rollbackFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	return endTx("sql.rollback", args, false)
}

// -------- prepared statements --------

// sql.prepare(conn, sql) -> sql.Statement.
func prepareFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("sql.prepare expects 2 arguments (sql.Connection, sql), got %d", len(args))
	}
	id, err := handleID("sql.prepare", args[0], "Connection")
	if err != nil {
		return interpreter.Null(), err
	}
	sqlText, err := devio.StringArg("sql.prepare", args, 1, "sql")
	if err != nil {
		return interpreter.Null(), err
	}
	mu.Lock()
	db, ok := conns[id]
	mu.Unlock()
	if !ok {
		return interpreter.Null(), fmt.Errorf("sql.prepare: sql.Connection id %d is not open", id)
	}
	st, perr := db.Prepare(sqlText)
	if perr != nil {
		return interpreter.Null(), fmt.Errorf("sql.prepare: %v", perr)
	}
	mu.Lock()
	nid++
	sid := nid
	stmts[sid] = st
	mu.Unlock()
	return handle("Statement", sid), nil
}

func resolveStmt(fn string, v Value) (*sql.Stmt, error) {
	id, err := handleID(fn, v, "Statement")
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	st, ok := stmts[id]
	if !ok {
		return nil, fmt.Errorf("%s: sql.Statement id %d is not open (already closed?)", fn, id)
	}
	return st, nil
}

// sql.queryStmt(stmt, params...) -> sql.Rows.
func queryStmtFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) < 1 {
		return interpreter.Null(), fmt.Errorf("sql.queryStmt expects at least (sql.Statement), got %d args", len(args))
	}
	st, err := resolveStmt("sql.queryStmt", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	params, err := toDriverArgs("sql.queryStmt", args[1:])
	if err != nil {
		return interpreter.Null(), err
	}
	rs, qerr := st.Query(params...)
	if qerr != nil {
		return interpreter.Null(), fmt.Errorf("sql.queryStmt: %v", qerr)
	}
	cols, cerr := rs.Columns()
	if cerr != nil {
		_ = rs.Close()
		return interpreter.Null(), fmt.Errorf("sql.queryStmt: %v", cerr)
	}
	mu.Lock()
	nid++
	rid := nid
	rows[rid] = &rowState{id: rid, rows: rs, cols: cols}
	mu.Unlock()
	return handle("Rows", rid), nil
}

// sql.execStmt(stmt, params...) -> sql.Result.
func execStmtFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) < 1 {
		return interpreter.Null(), fmt.Errorf("sql.execStmt expects at least (sql.Statement), got %d args", len(args))
	}
	st, err := resolveStmt("sql.execStmt", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	params, err := toDriverArgs("sql.execStmt", args[1:])
	if err != nil {
		return interpreter.Null(), err
	}
	res, eerr := st.Exec(params...)
	if eerr != nil {
		return interpreter.Null(), fmt.Errorf("sql.execStmt: %v", eerr)
	}
	return resultValue(res), nil
}

// sql.closeStmt(stmt) -> null.
func closeStmtFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("sql.closeStmt expects 1 argument (sql.Statement), got %d", len(args))
	}
	id, err := handleID("sql.closeStmt", args[0], "Statement")
	if err != nil {
		return interpreter.Null(), err
	}
	mu.Lock()
	st, ok := stmts[id]
	if !ok {
		mu.Unlock()
		return interpreter.Null(), fmt.Errorf("sql.closeStmt: sql.Statement id %d is not open (already closed?)", id)
	}
	delete(stmts, id)
	mu.Unlock()
	if cerr := st.Close(); cerr != nil {
		return interpreter.Null(), fmt.Errorf("sql.closeStmt: %v", cerr)
	}
	return interpreter.Null(), nil
}
