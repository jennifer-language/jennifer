// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

//go:build tinygo

// TinyGo stub for the `net` library. TinyGo 0.41 compiles
// most of the standard-Go `net` surface but requires a netdev
// driver at runtime (which the `jennifer-tiny` binary doesn't
// register), and lacks `net.ListenPacket` entirely for UDP. Rather
// than surface cryptic runtime errors from deep inside Go's
// runtime, every entry point returns a friendly Jennifer-level
// message pointing the user at the default `jennifer` binary
// (standard-Go build).
//
// This mirrors the `os.run` pattern.

package netlib

import (
	"fmt"

	"github.com/mplx/jennifer-lang/internal/interpreter"
)

// unavailable is the shared error every stub returns. Named so the
// message stays uniform across the surface and future edits happen
// in one place.
func unavailable(fnName string) (Value, error) {
	return interpreter.Null(), fmt.Errorf("%s: `jennifer-tiny` (TinyGo build) does not include a network stack; use the default `jennifer` binary for network I/O", fnName)
}

// ResetForTest is a no-op under TinyGo since no state exists.
func ResetForTest() {}

// TCP.

func connectFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("net.connect")
}
func connectTLSFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("net.connectTLS")
}
func startTLSFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("net.startTLS")
}
func listenFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("net.listen")
}
func acceptFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("net.accept")
}
func readBytesFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("net.readBytes")
}
func writeBytesFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("net.writeBytes")
}
func eofFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("net.eof")
}
func addressFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("net.address")
}

// UDP.

func listenUDPFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("net.listenUDP")
}
func sendToFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("net.sendTo")
}
func recvFromFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("net.recvFrom")
}

// DNS.

func lookupFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("net.lookup")
}
func reverseLookupFn(_ interpreter.BuiltinCtx, _ []Value) (Value, error) {
	return unavailable("net.reverseLookup")
}

// Close paths (called from the polymorphic net.close dispatcher).

func closeConn(_ int64) error {
	return fmt.Errorf("net.close: `jennifer-tiny` (TinyGo build) does not include a network stack; use the default `jennifer` binary for network I/O")
}
func closeListener(_ int64) error {
	return fmt.Errorf("net.close: `jennifer-tiny` (TinyGo build) does not include a network stack; use the default `jennifer` binary for network I/O")
}
func closeUDP(_ int64) error {
	return fmt.Errorf("net.close: `jennifer-tiny` (TinyGo build) does not include a network stack; use the default `jennifer` binary for network I/O")
}
