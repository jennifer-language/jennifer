// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package binarylib implements Jennifer's `binary` library: bulk operations on
// `bytes` values - the byte-data counterpart to what `strings` is for text and
// `lists` is for lists. Its reason to exist is throughput: a `.j` program that
// concatenates, slices, searches, or splits byte buffers a byte at a time pays
// the tree-walker's per-operation cost on every byte, so these push the inner
// loop into Go and run once at native speed (`bytes.Index` / `bytes.Split` /
// copy). The name is `binary` because `bytes` is a reserved type keyword and so
// cannot be a library namespace.
//
// Every operation is non-mutating and value-semantic: the inputs are never
// aliased or written; each result is a freshly-allocated `bytes` (or `list of
// bytes` / `int` / `bool`).
package binarylib

import (
	"bytes"
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the Jennifer name programs `use` to enable these functions.
const LibraryName = "binary"

// Value type-alias keeps signatures short.
type Value = interpreter.Value

// Install registers the binary library functions on an interpreter. Namespaced:
// every name lives behind the `binary.` prefix.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespaced(LibraryName, "concat", concatFn)
	in.RegisterNamespaced(LibraryName, "slice", sliceFn)
	in.RegisterNamespaced(LibraryName, "indexOf", indexOfFn)
	in.RegisterNamespaced(LibraryName, "contains", containsFn)
	in.RegisterNamespaced(LibraryName, "split", splitFn)
	in.RegisterNamespaced(LibraryName, "startsWith", startsWithFn)
	in.RegisterNamespaced(LibraryName, "endsWith", endsWithFn)
}

// takeBytes reads a positional bytes argument.
func takeBytes(fn string, args []Value, idx int, role string) ([]byte, error) {
	if args[idx].Kind != interpreter.KindBytes {
		return nil, fmt.Errorf("%s: %s must be bytes, got %s", fn, role, args[idx].Kind)
	}
	return args[idx].Bytes, nil
}

// takeInt reads a positional int argument.
func takeInt(fn string, args []Value, idx int, role string) (int64, error) {
	if args[idx].Kind != interpreter.KindInt {
		return 0, fmt.Errorf("%s: %s must be int, got %s", fn, role, args[idx].Kind)
	}
	return args[idx].Int, nil
}

// concatFn joins two byte sequences into a fresh bytes: binary.concat(a, b). For
// building a buffer in a loop prefer net.readAll / net.readN (which grow one Go
// slice); concat is O(len(a)+len(b)) per call, so a concat-in-a-loop is O(n^2).
func concatFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("binary.concat expects 2 arguments (a, b), got %d", len(args))
	}
	a, err := takeBytes("binary.concat", args, 0, "first argument")
	if err != nil {
		return interpreter.Null(), err
	}
	b, err := takeBytes("binary.concat", args, 1, "second argument")
	if err != nil {
		return interpreter.Null(), err
	}
	out := make([]byte, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	return interpreter.BytesVal(out), nil
}

// sliceFn returns the half-open range [start, end) as a fresh bytes:
// binary.slice(b, start[, end]). end defaults to len(b). The range must lie
// within [0, len(b)] with start <= end, else a positioned error (strict, like
// the language's index checks).
func sliceFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 && len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("binary.slice expects 2 or 3 arguments (b, start[, end]), got %d", len(args))
	}
	b, err := takeBytes("binary.slice", args, 0, "first argument")
	if err != nil {
		return interpreter.Null(), err
	}
	start, err := takeInt("binary.slice", args, 1, "start")
	if err != nil {
		return interpreter.Null(), err
	}
	end := int64(len(b))
	if len(args) == 3 {
		end, err = takeInt("binary.slice", args, 2, "end")
		if err != nil {
			return interpreter.Null(), err
		}
	}
	if start < 0 || end > int64(len(b)) || start > end {
		return interpreter.Null(), fmt.Errorf("binary.slice: range [%d, %d) out of bounds for %d bytes", start, end, len(b))
	}
	out := make([]byte, end-start)
	copy(out, b[start:end])
	return interpreter.BytesVal(out), nil
}

// indexOfFn returns the index of the first occurrence of needle in haystack, or
// -1 if absent: binary.indexOf(haystack, needle). Named to match
// strings.indexOf (same argument order, same -1-on-absent contract). An empty
// needle matches at 0. Runs at native speed (bytes.Index), so scanning a large
// buffer for a delimiter (a MIME boundary, a CRLF) does not pay a per-byte
// interpreted loop.
func indexOfFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("binary.indexOf expects 2 arguments (haystack, needle), got %d", len(args))
	}
	hay, err := takeBytes("binary.indexOf", args, 0, "haystack")
	if err != nil {
		return interpreter.Null(), err
	}
	needle, err := takeBytes("binary.indexOf", args, 1, "needle")
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.IntVal(int64(bytes.Index(hay, needle))), nil
}

// containsFn reports whether needle occurs in haystack: binary.contains(haystack,
// needle). The boolean sibling of indexOf, mirroring strings.contains.
func containsFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("binary.contains expects 2 arguments (haystack, needle), got %d", len(args))
	}
	hay, err := takeBytes("binary.contains", args, 0, "haystack")
	if err != nil {
		return interpreter.Null(), err
	}
	needle, err := takeBytes("binary.contains", args, 1, "needle")
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BoolVal(bytes.Contains(hay, needle)), nil
}

// splitFn splits b on every occurrence of a non-empty separator, returning a
// `list of bytes`: binary.split(b, sep). The natural way to cut a MIME
// multipart body at its boundary in one Go pass.
func splitFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("binary.split expects 2 arguments (b, sep), got %d", len(args))
	}
	b, err := takeBytes("binary.split", args, 0, "first argument")
	if err != nil {
		return interpreter.Null(), err
	}
	sep, err := takeBytes("binary.split", args, 1, "separator")
	if err != nil {
		return interpreter.Null(), err
	}
	if len(sep) == 0 {
		return interpreter.Null(), fmt.Errorf("binary.split: separator must be non-empty")
	}
	parts := bytes.Split(b, sep)
	out := make([]Value, len(parts))
	for i, p := range parts {
		cp := make([]byte, len(p))
		copy(cp, p)
		out[i] = interpreter.BytesVal(cp)
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeBytes), out), nil
}

// startsWithFn reports whether b begins with prefix: binary.startsWith(b, prefix).
func startsWithFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("binary.startsWith expects 2 arguments (b, prefix), got %d", len(args))
	}
	b, err := takeBytes("binary.startsWith", args, 0, "first argument")
	if err != nil {
		return interpreter.Null(), err
	}
	prefix, err := takeBytes("binary.startsWith", args, 1, "prefix")
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BoolVal(bytes.HasPrefix(b, prefix)), nil
}

// endsWithFn reports whether b ends with suffix: binary.endsWith(b, suffix).
func endsWithFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("binary.endsWith expects 2 arguments (b, suffix), got %d", len(args))
	}
	b, err := takeBytes("binary.endsWith", args, 0, "first argument")
	if err != nil {
		return interpreter.Null(), err
	}
	suffix, err := takeBytes("binary.endsWith", args, 1, "suffix")
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.BoolVal(bytes.HasSuffix(b, suffix)), nil
}
