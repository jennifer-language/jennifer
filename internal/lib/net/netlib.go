// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package netlib implements Jennifer's `net` library:
// blocking TCP and UDP sockets plus two DNS lookup helpers. The
// design mirrors `fs`: blocking on purpose, with non-blocking
// use composed through `spawn` rather than a duplicated
// `*Async` surface.
//
// The library uses a build-tag split. The default `jennifer`
// binary (standard-Go build) ships the real implementations in
// netlib_std.go; `jennifer-tiny` (TinyGo build) ships stubs in
// netlib_tinygo.go that return a friendly runtime error pointing
// the user back at `jennifer`. Same shape as `os.run` /
// `os.spawn` under TinyGo.
//
// Handles use the integer-registry pattern from hash and
// fs: `net.Conn{id as int}`, `net.Listener{id as int}`,
// `net.UDPSocket{id as int}` on the Jennifer side; live Go state
// lives in per-registry maps guarded by mutexes.
package netlib

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the namespace prefix and `use` name.
const LibraryName = "net"

// Value type alias keeps signatures short.
type Value = interpreter.Value

// Install registers the net surface with the interpreter.
// The actual implementations of connect / listen / accept / read /
// write / eof / address / listenUDP / sendTo / recvFrom / lookup /
// reverseLookup are provided by the build-tag-selected file
// (netlib_std.go or netlib_tinygo.go); this file registers them
// and installs the shared verbs (close, struct definitions).
func Install(in *interpreter.Interpreter) {
	// Structs.
	in.RegisterNamespacedStruct(LibraryName, "Conn", []parser.StructField{
		{Name: "id", Type: parser.PrimitiveType(parser.TypeInt)},
	})
	in.RegisterNamespacedStruct(LibraryName, "Listener", []parser.StructField{
		{Name: "id", Type: parser.PrimitiveType(parser.TypeInt)},
	})
	in.RegisterNamespacedStruct(LibraryName, "UDPSocket", []parser.StructField{
		{Name: "id", Type: parser.PrimitiveType(parser.TypeInt)},
	})
	in.RegisterNamespacedStruct(LibraryName, "Datagram", []parser.StructField{
		{Name: "data", Type: parser.PrimitiveType(parser.TypeBytes)},
		{Name: "peer", Type: parser.PrimitiveType(parser.TypeString)},
	})
	// TLS handshake options; the zero value verifies against the system
	// roots. `skipVerify: true` accepts any certificate (insecure); a
	// non-empty `caCert` (PEM) trusts a specific / self-signed certificate.
	in.RegisterNamespacedStruct(LibraryName, "TLSOptions", []parser.StructField{
		{Name: "skipVerify", Type: parser.PrimitiveType(parser.TypeBool)},
		{Name: "caCert", Type: parser.PrimitiveType(parser.TypeBytes)},
	})

	// TCP.
	in.RegisterNamespaced(LibraryName, "connect", connectFn)
	in.RegisterNamespaced(LibraryName, "connectTLS", connectTLSFn)
	in.RegisterNamespaced(LibraryName, "startTLS", startTLSFn)
	in.RegisterNamespaced(LibraryName, "listen", listenFn)
	in.RegisterNamespaced(LibraryName, "accept", acceptFn)
	in.RegisterNamespaced(LibraryName, "readBytes", readBytesFn)
	in.RegisterNamespaced(LibraryName, "readAll", readAllFn)
	in.RegisterNamespaced(LibraryName, "readN", readNFn)
	in.RegisterNamespaced(LibraryName, "writeBytes", writeBytesFn)
	in.RegisterNamespaced(LibraryName, "setDeadline", setDeadlineFn)
	in.RegisterNamespaced(LibraryName, "eof", eofFn)
	in.RegisterNamespaced(LibraryName, "address", addressFn)

	// UDP.
	in.RegisterNamespaced(LibraryName, "listenUDP", listenUDPFn)
	in.RegisterNamespaced(LibraryName, "sendTo", sendToFn)
	in.RegisterNamespaced(LibraryName, "recvFrom", recvFromFn)

	// DNS.
	in.RegisterNamespaced(LibraryName, "lookup", lookupFn)
	in.RegisterNamespaced(LibraryName, "reverseLookup", reverseLookupFn)

	// Polymorphic close - dispatches on the argument's struct tag.
	// Lives here (not in the build-tag files) because dispatch is
	// pure Jennifer-value logic; the actual close syscall is in the
	// build-tag file.
	in.RegisterNamespaced(LibraryName, "close", closeFn)
}

// makeConn / makeListener / makeUDPSocket construct the Jennifer
// struct values wrapping a registry id. Defined here so both build
// tags can produce them.
func makeConn(id int64) Value {
	return interpreter.NamespacedStructVal(LibraryName, "Conn", []interpreter.StructField{
		{Name: "id", Value: interpreter.IntVal(id)},
	})
}

func makeListener(id int64) Value {
	return interpreter.NamespacedStructVal(LibraryName, "Listener", []interpreter.StructField{
		{Name: "id", Value: interpreter.IntVal(id)},
	})
}

func makeUDPSocket(id int64) Value {
	return interpreter.NamespacedStructVal(LibraryName, "UDPSocket", []interpreter.StructField{
		{Name: "id", Value: interpreter.IntVal(id)},
	})
}

func makeDatagram(data []byte, peer string) Value {
	return interpreter.NamespacedStructVal(LibraryName, "Datagram", []interpreter.StructField{
		{Name: "data", Value: interpreter.BytesVal(data)},
		{Name: "peer", Value: interpreter.StringVal(peer)},
	})
}

// extractID pulls the integer id out of a struct value of the given
// namespaced-struct name. Returns a boundary error mentioning what
// the caller passed in so mis-typed arguments surface cleanly.
func extractID(fnName, wantStruct string, v Value) (int64, error) {
	if v.Kind != interpreter.KindStruct || v.StructNS != LibraryName || v.StructName != wantStruct {
		return 0, fmt.Errorf("%s: argument must be a net.%s, got %s", fnName, wantStruct, v.Kind)
	}
	for _, f := range v.Fields {
		if f.Name == "id" {
			if f.Value.Kind != interpreter.KindInt {
				return 0, fmt.Errorf("%s: net.%s.id is not int (got %s)", fnName, wantStruct, f.Value.Kind)
			}
			return f.Value.Int, nil
		}
	}
	return 0, fmt.Errorf("%s: net.%s has no id field", fnName, wantStruct)
}

// takeStringArg pulls out a positional string argument or errors
// with the role name for context.
func takeStringArg(fnName string, args []Value, idx int, role string) (string, error) {
	if args[idx].Kind != interpreter.KindString {
		return "", fmt.Errorf("%s: %s must be string, got %s", fnName, role, args[idx].Kind)
	}
	return args[idx].Str, nil
}

// takeBytesArg pulls out a positional bytes argument.
func takeBytesArg(fnName string, args []Value, idx int, role string) ([]byte, error) {
	if args[idx].Kind != interpreter.KindBytes {
		return nil, fmt.Errorf("%s: %s must be bytes, got %s", fnName, role, args[idx].Kind)
	}
	return args[idx].Bytes, nil
}

// takeIntArg pulls out a positional non-negative int argument.
func takeIntArg(fnName string, args []Value, idx int, role string) (int64, error) {
	if args[idx].Kind != interpreter.KindInt {
		return 0, fmt.Errorf("%s: %s must be int, got %s", fnName, role, args[idx].Kind)
	}
	if args[idx].Int < 0 {
		return 0, fmt.Errorf("%s: %s must be non-negative, got %d", fnName, role, args[idx].Int)
	}
	return args[idx].Int, nil
}

// stringSlice wraps a Go []string as a Jennifer `list of string`.
// Used by DNS lookup returns.
func stringSlice(ss []string) Value {
	out := make([]Value, len(ss))
	for i, s := range ss {
		out[i] = interpreter.StringVal(s)
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeString), out)
}

// closeFn dispatches on the argument's struct tag. Three struct
// kinds (Conn, Listener, UDPSocket) share this verb - the caller
// doesn't have to remember three different closer names.
func closeFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("net.close expects 1 argument (net.Conn, net.Listener, or net.UDPSocket), got %d", len(args))
	}
	v := args[0]
	if v.Kind != interpreter.KindStruct || v.StructNS != LibraryName {
		return interpreter.Null(), fmt.Errorf("net.close: argument must be a net.Conn, net.Listener, or net.UDPSocket; got %s", v.Kind)
	}
	switch v.StructName {
	case "Conn":
		id, err := extractID("net.close", "Conn", v)
		if err != nil {
			return interpreter.Null(), err
		}
		return interpreter.Null(), closeConn(id)
	case "Listener":
		id, err := extractID("net.close", "Listener", v)
		if err != nil {
			return interpreter.Null(), err
		}
		return interpreter.Null(), closeListener(id)
	case "UDPSocket":
		id, err := extractID("net.close", "UDPSocket", v)
		if err != nil {
			return interpreter.Null(), err
		}
		return interpreter.Null(), closeUDP(id)
	default:
		return interpreter.Null(), fmt.Errorf("net.close: argument must be a net.Conn, net.Listener, or net.UDPSocket; got net.%s", v.StructName)
	}
}
