// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package sqllib

import (
	"database/sql"
	"database/sql/driver"
	"io"
	"math"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// typedDriver returns the Go types go-sql-driver/mysql produces beyond the
// database/sql/driver.Value set: uint64 (BIGINT UNSIGNED, text protocol) and
// float32 (FLOAT), plus floats that do not convert to an int.
type typedDriver struct{}

func (typedDriver) Open(string) (driver.Conn, error) { return typedConn{}, nil }

type typedConn struct{}

func (typedConn) Prepare(string) (driver.Stmt, error) { return typedStmt{}, nil }
func (typedConn) Close() error                        { return nil }
func (typedConn) Begin() (driver.Tx, error)           { return mockTx{}, nil }

type typedStmt struct{}

func (typedStmt) Close() error                               { return nil }
func (typedStmt) NumInput() int                              { return -1 }
func (typedStmt) Exec([]driver.Value) (driver.Result, error) { return mockResult{}, nil }
func (typedStmt) Query([]driver.Value) (driver.Rows, error)  { return &typedRows{}, nil }

type typedRows struct{ done bool }

func (*typedRows) Columns() []string {
	return []string{"id", "big", "f32", "frac", "huge", "nan"}
}
func (*typedRows) Close() error { return nil }
func (r *typedRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	copy(dest, []driver.Value{uint64(5), uint64(math.MaxUint64), float32(1.5), float64(3.7), float64(1e300), math.NaN()})
	return nil
}

var registerTypedOnce sync.Once

func typedRowsValue(t *testing.T) Value {
	t.Helper()
	registerTypedOnce.Do(func() { sql.Register("mocktyped", typedDriver{}) })
	ResetForTest()
	db, err := sql.Open("mocktyped", "")
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	nid++
	id := nid
	conns[id] = db
	mu.Unlock()
	rowsV, err := queryFn(ctx(), []Value{handle("Connection", id), sv("SELECT")})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if ok, err := nextFn(ctx(), []Value{rowsV}); err != nil || !ok.Bool {
		t.Fatalf("next: %v %v", ok, err)
	}
	return rowsV
}

// TestAccessorsDriverTypes: uint64 and float32 columns convert; a float that is
// not a whole int64 is an error instead of a truncated or wrapped int.
func TestAccessorsDriverTypes(t *testing.T) {
	rowsV := typedRowsValue(t)
	assertInt(t, rowsV, sv("id"), 5)
	assertFloat(t, rowsV, sv("id"), 5)
	assertBool(t, rowsV, sv("id"), true)
	assertStr(t, rowsV, sv("big"), "18446744073709551615")
	assertFloat(t, rowsV, sv("f32"), 1.5)
	assertStr(t, rowsV, sv("f32"), "1.5")
	for _, c := range []struct{ col, want string }{
		{"big", "out of int range"},
		{"f32", "not an integer"},
		{"frac", "not an integer"},
		{"huge", "not an integer"},
		{"nan", "not an integer"},
	} {
		v, err := asIntFn(ctx(), []Value{rowsV, sv(c.col)})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("asInt(%s) = %v, err %v; want an error containing %q", c.col, v, err, c.want)
		}
	}
}

// TestBeginPrepareBoundedByQueryTimeout: with the only pool connection pinned by
// an open cursor, begin and prepare fail after the query timeout instead of
// waiting forever for a connection.
func TestBeginPrepareBoundedByQueryTimeout(t *testing.T) {
	conn := mockConnValue(t)
	mu.Lock()
	conns[conn.Fields[0].Value.Int].SetMaxOpenConns(1)
	mu.Unlock()
	// The cursor is opened without a deadline (which would close it and free
	// the connection), then the timeout is lowered for begin / prepare.
	if _, err := setQueryTimeoutFn(ctx(), []Value{interpreter.IntVal(0)}); err != nil {
		t.Fatal(err)
	}
	if _, err := queryFn(ctx(), []Value{conn, sv("SELECT * FROM t")}); err != nil {
		t.Fatalf("query: %v", err)
	}
	if _, err := setQueryTimeoutFn(ctx(), []Value{interpreter.IntVal(50)}); err != nil {
		t.Fatal(err)
	}
	for name, call := range map[string]func() error{
		"begin": func() error {
			_, err := beginFn(ctx(), []Value{conn})
			return err
		},
		"prepare": func() error {
			_, err := prepareFn(ctx(), []Value{conn, sv("SELECT 1")})
			return err
		},
	} {
		errc := make(chan error, 1)
		go func() { errc <- call() }()
		select {
		case err := <-errc:
			if err == nil {
				t.Errorf("%s: expected a timeout error with the pool exhausted", name)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: still waiting for a pool connection after 5 s", name)
		}
	}
	ResetForTest()
}

// TestBeginReleasesConnection: commit and rollback return the connection begin
// acquired, so a one-connection pool serves the next begin.
func TestBeginReleasesConnection(t *testing.T) {
	conn := mockConnValue(t)
	mu.Lock()
	conns[conn.Fields[0].Value.Int].SetMaxOpenConns(1)
	mu.Unlock()
	if _, err := setQueryTimeoutFn(ctx(), []Value{interpreter.IntVal(1000)}); err != nil {
		t.Fatal(err)
	}
	for i, end := range []func(interpreter.BuiltinCtx, []Value) (Value, error){commitFn, rollbackFn, commitFn} {
		tx, err := beginFn(ctx(), []Value{conn})
		if err != nil {
			t.Fatalf("begin %d: %v", i, err)
		}
		if _, err := end(ctx(), []Value{tx}); err != nil {
			t.Fatalf("end %d: %v", i, err)
		}
	}
	ResetForTest()
}

// TestOpenPingBoundedByQueryTimeout: a server that accepts TCP but never sends
// the MySQL greeting makes sql.open fail after the query timeout.
func TestOpenPingBoundedByQueryTimeout(t *testing.T) {
	ResetForTest()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("listen: %v", err)
	}
	defer ln.Close()
	var held []net.Conn
	var hmu sync.Mutex
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			hmu.Lock()
			held = append(held, c)
			hmu.Unlock()
		}
	}()
	defer func() {
		hmu.Lock()
		for _, c := range held {
			_ = c.Close()
		}
		hmu.Unlock()
	}()
	if _, err := setQueryTimeoutFn(ctx(), []Value{interpreter.IntVal(200)}); err != nil {
		t.Fatal(err)
	}
	errc := make(chan error, 1)
	go func() {
		_, err := openFn(ctx(), []Value{sv("mysql"), sv("u:p@tcp(" + ln.Addr().String() + ")/db")})
		errc <- err
	}()
	select {
	case err := <-errc:
		if err == nil {
			t.Error("open against a silent server: expected an error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("open still pinging a silent server after 5 s")
	}
	ResetForTest()
}
