// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

package sqllib

import (
	"database/sql"
	"database/sql/driver"
	"io"
	"strings"
	"sync"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// -------- a canned in-memory database/sql driver (test only) --------

type mockDriver struct{}

func (mockDriver) Open(string) (driver.Conn, error) { return &mockConn{}, nil }

type mockConn struct{}

func (c *mockConn) Prepare(q string) (driver.Stmt, error) { return &mockStmt{}, nil }
func (c *mockConn) Close() error                          { return nil }
func (c *mockConn) Begin() (driver.Tx, error)             { return mockTx{}, nil }

type mockTx struct{}

func (mockTx) Commit() error   { return nil }
func (mockTx) Rollback() error { return nil }

type mockStmt struct{}

func (s *mockStmt) Close() error                                 { return nil }
func (s *mockStmt) NumInput() int                                { return -1 } // skip arg-count check
func (s *mockStmt) Exec(_ []driver.Value) (driver.Result, error) { return mockResult{}, nil }
func (s *mockStmt) Query(_ []driver.Value) (driver.Rows, error)  { return &mockRows{}, nil }

type mockResult struct{}

func (mockResult) LastInsertId() (int64, error) { return 42, nil }
func (mockResult) RowsAffected() (int64, error) { return 1, nil }

type mockRows struct{ i int }

func (r *mockRows) Columns() []string { return []string{"id", "name", "score", "active", "note"} }
func (r *mockRows) Close() error      { return nil }
func (r *mockRows) Next(dest []driver.Value) error {
	data := [][]driver.Value{
		{int64(1), "alice", float64(9.5), true, nil},
		{int64(2), "bob", float64(3.0), false, []byte("hi")},
	}
	if r.i >= len(data) {
		return io.EOF
	}
	copy(dest, data[r.i])
	r.i++
	return nil
}

var registerOnce sync.Once

func mockConnValue(t *testing.T) Value {
	t.Helper()
	registerOnce.Do(func() { sql.Register("mockdb", mockDriver{}) })
	ResetForTest()
	db, err := sql.Open("mockdb", "")
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	nid++
	id := nid
	conns[id] = db
	mu.Unlock()
	return handle("Connection", id)
}

func ctx() interpreter.BuiltinCtx { return interpreter.BuiltinCtx{} }

func sv(s string) Value { return interpreter.StringVal(s) }

// -------- tests --------

func TestQueryCursorAndAccessors(t *testing.T) {
	conn := mockConnValue(t)
	rowsV, err := queryFn(ctx(), []Value{conn, sv("SELECT * FROM t")})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	// columns
	cols, err := columnsFn(ctx(), []Value{rowsV})
	if err != nil || cols.Kind != interpreter.KindList || len(cols.List) != 5 {
		t.Fatalf("columns: %v (%+v)", err, cols)
	}
	// row 1
	ok, err := nextFn(ctx(), []Value{rowsV})
	if err != nil || !ok.Bool {
		t.Fatalf("next 1: %v ok=%v", err, ok)
	}
	assertInt(t, rowsV, sv("id"), 1)
	assertStr(t, rowsV, sv("name"), "alice")
	assertFloat(t, rowsV, sv("score"), 9.5)
	assertBool(t, rowsV, sv("active"), true)
	assertFloat(t, rowsV, sv("active"), 1.0) // bool coerces like asInt does
	// NULL column
	nv, err := isNullFn(ctx(), []Value{rowsV, sv("note")})
	if err != nil || !nv.Bool {
		t.Errorf("isNull note (row1): %v %v", err, nv)
	}
	// asInt on NULL is an error, not a silent zero
	if _, err := asIntFn(ctx(), []Value{rowsV, sv("note")}); err == nil {
		t.Error("asInt on NULL should error")
	}
	// column by index works too
	assertInt(t, rowsV, interpreter.IntVal(0), 1)

	// row 2
	ok, _ = nextFn(ctx(), []Value{rowsV})
	if !ok.Bool {
		t.Fatal("next 2 should be true")
	}
	assertStr(t, rowsV, sv("name"), "bob")
	bv, err := asBytesFn(ctx(), []Value{rowsV, sv("note")})
	if err != nil || string(bv.Bytes) != "hi" {
		t.Errorf("asBytes note (row2): %v %q", err, bv.Bytes)
	}
	// end
	ok, _ = nextFn(ctx(), []Value{rowsV})
	if ok.Bool {
		t.Error("next 3 should be false (end)")
	}
	// exhaustion released the handle: the registry must not accumulate spent
	// cursors, and a further next on the released handle is a positioned error.
	mu.Lock()
	nRows := len(rows)
	mu.Unlock()
	if nRows != 0 {
		t.Errorf("rows registry should be empty after exhaustion, has %d entries", nRows)
	}
	if _, err := nextFn(ctx(), []Value{rowsV}); err == nil || !strings.Contains(err.Error(), "not open") {
		t.Errorf("next after release should error 'not open', got %v", err)
	}
}

func TestExecResult(t *testing.T) {
	conn := mockConnValue(t)
	res, err := execFn(ctx(), []Value{conn, sv("INSERT INTO t VALUES (?)"), interpreter.IntVal(7)})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if res.Kind != interpreter.KindStruct || res.StructName != "Result" {
		t.Fatalf("exec result shape: %+v", res)
	}
	var affected, lastId int64 = -99, -99
	for _, f := range res.Fields {
		switch f.Name {
		case "affected":
			affected = f.Value.Int
		case "lastId":
			lastId = f.Value.Int
		}
	}
	if affected != 1 || lastId != 42 {
		t.Errorf("affected=%d lastId=%d, want 1/42", affected, lastId)
	}
}

func TestTransactionAndPrepared(t *testing.T) {
	conn := mockConnValue(t)
	tx, err := beginFn(ctx(), []Value{conn})
	if err != nil || tx.StructName != "Tx" {
		t.Fatalf("begin: %v (%+v)", err, tx)
	}
	// exec on a Tx (querier dispatch)
	if _, err := execFn(ctx(), []Value{tx, sv("UPDATE t SET x=?"), interpreter.IntVal(1)}); err != nil {
		t.Fatalf("exec on tx: %v", err)
	}
	if _, err := commitFn(ctx(), []Value{tx}); err != nil {
		t.Fatalf("commit: %v", err)
	}
	// a committed tx is no longer usable
	if _, err := execFn(ctx(), []Value{tx, sv("x")}); err == nil {
		t.Error("exec on a committed tx should error")
	}
	// prepared statement
	st, err := prepareFn(ctx(), []Value{conn, sv("SELECT * FROM t WHERE id=?")})
	if err != nil || st.StructName != "Statement" {
		t.Fatalf("prepare: %v (%+v)", err, st)
	}
	rowsV, err := queryStmtFn(ctx(), []Value{st, interpreter.IntVal(1)})
	if err != nil {
		t.Fatalf("queryStmt: %v", err)
	}
	if _, err := nextFn(ctx(), []Value{rowsV}); err != nil {
		t.Fatalf("next on stmt rows: %v", err)
	}
	if _, err := closeStmtFn(ctx(), []Value{st}); err != nil {
		t.Fatalf("closeStmt: %v", err)
	}
}

func TestErrorsAndBinding(t *testing.T) {
	conn := mockConnValue(t)
	// unknown driver ("pgx" is deliberately not an alias: the documented names
	// are the only accepted ones)
	for _, d := range []string{"oracle", "pgx"} {
		if _, err := openFn(ctx(), []Value{sv(d), sv("dsn")}); err == nil || !strings.Contains(err.Error(), "unknown driver") {
			t.Errorf("expected unknown-driver error for %q, got %v", d, err)
		}
	}
	// wrong handle kind
	if _, err := queryFn(ctx(), []Value{interpreter.IntVal(5), sv("SELECT 1")}); err == nil {
		t.Error("query with a non-handle target should error")
	}
	// A lone list argument is spread into the parameter sequence (what the orm
	// module relies on): a list of scalars binds element by element.
	list := interpreter.ListVal(parser.PrimitiveType(parser.TypeInt),
		[]Value{interpreter.IntVal(1), interpreter.IntVal(2)})
	if _, err := execFn(ctx(), []Value{conn, sv("INSERT ? ?"), list}); err != nil {
		t.Errorf("a lone list param should spread and bind, got %v", err)
	}
	// A list mixed with other params does NOT spread (only a lone list does), so
	// the list is an unbindable parameter and errors.
	if _, err := execFn(ctx(), []Value{conn, sv("INSERT ? ?"), interpreter.IntVal(9), list}); err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Errorf("a list among other params should not bind, got %v", err)
	}
	// A lone list whose element is itself non-scalar errors after the spread.
	nested := interpreter.ListVal(parser.PrimitiveType(parser.TypeInt), []Value{list})
	if _, err := execFn(ctx(), []Value{conn, sv("INSERT ?"), nested}); err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Errorf("a spread list of lists should error, got %v", err)
	}
	// every scalar kind binds fine (int/float/string/bool/bytes/null)
	if _, err := execFn(ctx(), []Value{conn, sv("INSERT ?"),
		interpreter.IntVal(1), interpreter.FloatVal(2.0), sv("s"), interpreter.BoolVal(true),
		interpreter.BytesVal([]byte{1}), interpreter.Null()}); err != nil {
		t.Errorf("scalar params should bind: %v", err)
	}
	// accessor before next()
	rowsV, _ := queryFn(ctx(), []Value{conn, sv("SELECT 1")})
	if _, err := asIntFn(ctx(), []Value{rowsV, sv("id")}); err == nil {
		t.Error("accessor before next() should error")
	}
}

func assertInt(t *testing.T, rows, col Value, want int64) {
	t.Helper()
	v, err := asIntFn(ctx(), []Value{rows, col})
	if err != nil || v.Int != want {
		t.Errorf("asInt(%v): got %v err=%v, want %d", col, v, err, want)
	}
}
func assertStr(t *testing.T, rows, col Value, want string) {
	t.Helper()
	v, err := asStringFn(ctx(), []Value{rows, col})
	if err != nil || v.Str != want {
		t.Errorf("asString(%v): got %q err=%v, want %q", col, v.Str, err, want)
	}
}
func assertFloat(t *testing.T, rows, col Value, want float64) {
	t.Helper()
	v, err := asFloatFn(ctx(), []Value{rows, col})
	if err != nil || v.Float != want {
		t.Errorf("asFloat(%v): got %v err=%v, want %v", col, v.Float, err, want)
	}
}
func assertBool(t *testing.T, rows, col Value, want bool) {
	t.Helper()
	v, err := asBoolFn(ctx(), []Value{rows, col})
	if err != nil || v.Bool != want {
		t.Errorf("asBool(%v): got %v err=%v, want %v", col, v.Bool, err, want)
	}
}

// After the cursor is exhausted (next returned false), an accessor must error -
// not silently return the last row's stale data.
func TestCursorExhaustedAccessorErrors(t *testing.T) {
	conn := mockConnValue(t)
	rowsV, _ := queryFn(ctx(), []Value{conn, sv("SELECT * FROM t")})
	for {
		ok, _ := nextFn(ctx(), []Value{rowsV})
		if !ok.Bool {
			break
		}
	}
	if _, err := asIntFn(ctx(), []Value{rowsV, sv("id")}); err == nil || !strings.Contains(err.Error(), "exhausted") {
		t.Errorf("accessor after exhaustion should error 'exhausted', got %v", err)
	}
	// closeRows after exhaustion is a no-op, not a double-close error (so the
	// `defer sql.closeRows($rows)` idiom is safe after a fully-consumed loop) -
	// and so is a second closeRows.
	if _, err := closeRowsFn(ctx(), []Value{rowsV}); err != nil {
		t.Errorf("closeRows after exhaustion should be a no-op, got %v", err)
	}
	if _, err := closeRowsFn(ctx(), []Value{rowsV}); err != nil {
		t.Errorf("double closeRows should be a no-op, got %v", err)
	}
}

// A cursor shared across goroutines (misuse) must not be a Go data race: the
// per-cursor mutex serializes concurrent next/accessor calls. Run under -race.
func TestCursorConcurrentIsRaceFree(t *testing.T) {
	conn := mockConnValue(t)
	rowsV, _ := queryFn(ctx(), []Value{conn, sv("SELECT * FROM t")})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if ok, _ := nextFn(ctx(), []Value{rowsV}); ok.Bool {
					_, _ = asIntFn(ctx(), []Value{rowsV, sv("id")})
				}
			}
		}()
	}
	wg.Wait()
}
