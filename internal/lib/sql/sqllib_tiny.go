// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build tinygo

// TinyGo stub for the `sql` library. The database/sql drivers (go-sql-driver /
// pgx) are heavyweight dependency trees that TinyGo does not compile, so
// `jennifer-tiny` never imports them: every verb returns a friendly error
// pointing at the default `jennifer` binary. (On stock `jennifer-tiny` the
// engines are unreachable anyway - no network stack.)

package sqllib

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func unavailable(fn string) (Value, error) {
	return interpreter.Null(), fmt.Errorf("%s: relational databases are not available on this build; use the default `jennifer` binary on a network-capable host", fn)
}

// ResetForTest is a no-op where no state exists.
func ResetForTest() {}

func openFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error)    { return unavailable("sql.open") }
func closeFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error)   { return unavailable("sql.close") }
func queryFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error)   { return unavailable("sql.query") }
func execFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error)    { return unavailable("sql.exec") }
func nextFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error)    { return unavailable("sql.next") }
func columnsFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) { return unavailable("sql.columns") }
func asIntFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error)   { return unavailable("sql.asInt") }
func asFloatFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) { return unavailable("sql.asFloat") }
func asStringFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("sql.asString")
}
func asBoolFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error)  { return unavailable("sql.asBool") }
func asBytesFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) { return unavailable("sql.asBytes") }
func isNullFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error)  { return unavailable("sql.isNull") }
func closeRowsFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("sql.closeRows")
}
func beginFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error)  { return unavailable("sql.begin") }
func commitFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) { return unavailable("sql.commit") }
func rollbackFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("sql.rollback")
}
func prepareFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) { return unavailable("sql.prepare") }
func queryStmtFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("sql.queryStmt")
}
func execStmtFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("sql.execStmt")
}
func closeStmtFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("sql.closeStmt")
}
