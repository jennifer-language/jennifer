// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package httpdlib implements Jennifer's `httpd` library: an HTTP/1.1 server
// engine wrapping Go's `net/http`. It is the server counterpart to the `net`
// client primitives and the `http` client module - a server multiplexes many
// concurrent requests and wants the battle-tested Go engine (keep-alive,
// chunked transfer, TLS / HTTP-2, timeouts, graceful shutdown), where parsing
// each request in the tree-walker would be the wrong place for the hot path.
//
// Request handling is a **pull loop**. Jennifer has no first-class functions,
// so a handler cannot be a callback handed to Go. Instead `net/http` accepts
// and parses concurrently on its own goroutines and hands the interpreter one
// request at a time: `httpd.accept($srv)` blocks for the next request and
// `httpd.respond($req, status, body)` answers it. The two concurrency worlds
// stay separate - Go owns the I/O concurrency, and a `.j` program opts into
// per-request parallelism with its own `spawn` (several `spawn`ed workers can
// each call `httpd.accept` on the same server handle to form a worker pool).
//
// Like `net`, the library uses a build-tag split: the real engine lives in
// httpdlib_std.go (default `jennifer`), and httpdlib_tinygo.go stubs every
// entry point with a friendly error under `jennifer-tiny` (no netdev in
// TinyGo's runtime). Handles use the integer-registry pattern:
// `httpd.Server{id as int}` and `httpd.Request{id as int}` on the Jennifer
// side, live Go state in mutex-guarded registries.
package httpdlib

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the namespace prefix and `use` name.
const LibraryName = "httpd"

// Value type alias keeps signatures short.
type Value = interpreter.Value

// Install registers the httpd surface. The listen / accept / respond / ...
// implementations come from the build-tag-selected file (httpdlib_std.go or
// httpdlib_tinygo.go); this file registers them and the shared struct types.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespacedStruct(LibraryName, "Server", []parser.StructField{
		{Name: "id", Type: parser.PrimitiveType(parser.TypeInt)},
	})
	in.RegisterNamespacedStruct(LibraryName, "Request", []parser.StructField{
		{Name: "id", Type: parser.PrimitiveType(parser.TypeInt)},
	})

	// Lifecycle.
	in.RegisterNamespaced(LibraryName, "listen", listenFn)
	in.RegisterNamespaced(LibraryName, "listenTLS", listenTLSFn)
	in.RegisterNamespaced(LibraryName, "address", addressFn)
	in.RegisterNamespaced(LibraryName, "shutdown", shutdownFn)

	// Pull loop.
	in.RegisterNamespaced(LibraryName, "accept", acceptFn)

	// Request accessors.
	in.RegisterNamespaced(LibraryName, "method", methodFn)
	in.RegisterNamespaced(LibraryName, "path", pathFn)
	in.RegisterNamespaced(LibraryName, "query", queryFn)
	in.RegisterNamespaced(LibraryName, "header", headerFn)
	in.RegisterNamespaced(LibraryName, "body", bodyFn)
	in.RegisterNamespaced(LibraryName, "remoteAddr", remoteAddrFn)

	// Response.
	in.RegisterNamespaced(LibraryName, "setHeader", setHeaderFn)
	in.RegisterNamespaced(LibraryName, "respond", respondFn)
	in.RegisterNamespaced(LibraryName, "serveFile", serveFileFn)
	in.RegisterNamespaced(LibraryName, "serveDir", serveDirFn)
}

// makeServer / makeRequest build the Jennifer struct values wrapping a registry
// id. Defined here so both build tags produce them identically.
func makeServer(id int64) Value {
	return interpreter.NamespacedStructVal(LibraryName, "Server", []interpreter.StructField{
		{Name: "id", Value: interpreter.IntVal(id)},
	})
}

func makeRequest(id int64) Value {
	return interpreter.NamespacedStructVal(LibraryName, "Request", []interpreter.StructField{
		{Name: "id", Value: interpreter.IntVal(id)},
	})
}

// extractID pulls the integer id out of a namespaced-struct value, with a
// boundary error naming what the caller passed.
func extractID(fnName, wantStruct string, v Value) (int64, error) {
	if v.Kind != interpreter.KindStruct || v.StructNS != LibraryName || v.StructName != wantStruct {
		return 0, fmt.Errorf("%s: argument must be a httpd.%s, got %s", fnName, wantStruct, v.Kind)
	}
	for _, f := range v.Fields {
		if f.Name == "id" {
			if f.Value.Kind != interpreter.KindInt {
				return 0, fmt.Errorf("%s: httpd.%s.id is not int (got %s)", fnName, wantStruct, f.Value.Kind)
			}
			return f.Value.Int, nil
		}
	}
	return 0, fmt.Errorf("%s: httpd.%s has no id field", fnName, wantStruct)
}

// takeStringArg pulls a positional string argument or errors with the role.
func takeStringArg(fnName string, args []Value, idx int, role string) (string, error) {
	if args[idx].Kind != interpreter.KindString {
		return "", fmt.Errorf("%s: %s must be string, got %s", fnName, role, args[idx].Kind)
	}
	return args[idx].Str, nil
}

// takeIntArg pulls a positional int argument.
func takeIntArg(fnName string, args []Value, idx int, role string) (int64, error) {
	if args[idx].Kind != interpreter.KindInt {
		return 0, fmt.Errorf("%s: %s must be int, got %s", fnName, role, args[idx].Kind)
	}
	return args[idx].Int, nil
}

// takeBytesArg pulls a positional bytes argument.
func takeBytesArg(fnName string, args []Value, idx int, role string) ([]byte, error) {
	if args[idx].Kind != interpreter.KindBytes {
		return nil, fmt.Errorf("%s: %s must be bytes, got %s", fnName, role, args[idx].Kind)
	}
	return args[idx].Bytes, nil
}

// takeBodyArg accepts a response body as either a string or bytes.
func takeBodyArg(fnName string, v Value) ([]byte, error) {
	switch v.Kind {
	case interpreter.KindString:
		return []byte(v.Str), nil
	case interpreter.KindBytes:
		// Copy at the boundary: the socket write happens later, on the
		// handler goroutine, after respond has returned - handing it the
		// caller's backing would let a post-respond `$buf[i] = ...`
		// mutation race the write.
		out := make([]byte, len(v.Bytes))
		copy(out, v.Bytes)
		return out, nil
	default:
		return nil, fmt.Errorf("%s: body must be string or bytes, got %s", fnName, v.Kind)
	}
}
