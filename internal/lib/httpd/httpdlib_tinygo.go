// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build tinygo

// TinyGo stub for the `httpd` library. `jennifer-tiny` ships no network stack
// (no netdev driver in TinyGo's runtime), so every entry point returns a
// friendly Jennifer-level error pointing the user at the default `jennifer`
// binary - the same shape as the `net` and `os.run` stubs.

package httpdlib

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// unavailable is the shared error every stub returns.
func unavailable(fnName string) (Value, error) {
	return interpreter.Null(), fmt.Errorf("%s: `jennifer-tiny` (TinyGo build) does not include a network stack; use the default `jennifer` binary to run an HTTP server", fnName)
}

// ResetForTest is a no-op under TinyGo since no state exists.
func ResetForTest() {}

func listenFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) { return unavailable("httpd.listen") }
func listenTLSFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("httpd.listenTLS")
}
func addressFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("httpd.address")
}
func shutdownFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("httpd.shutdown")
}
func acceptFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) { return unavailable("httpd.accept") }
func methodFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) { return unavailable("httpd.method") }
func pathFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error)   { return unavailable("httpd.path") }
func queryFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error)  { return unavailable("httpd.query") }
func headerFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) { return unavailable("httpd.header") }
func bodyFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error)   { return unavailable("httpd.body") }
func remoteAddrFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("httpd.remoteAddr")
}
func setHeaderFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("httpd.setHeader")
}
func respondFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("httpd.respond")
}
func serveFileFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("httpd.serveFile")
}
func serveDirFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("httpd.serveDir")
}
