// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

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

// maxMakeBytes caps binary.make's allocation so a negative or oversized `n`
// (often wire- or caller-derived) is a catchable error instead of a Go panic /
// OOM in the recover-less interpreter. Mirrors net's per-call read ceiling.
const maxMakeBytes = 256 << 20

// Value type-alias keeps signatures short.
type Value = interpreter.Value

// Install registers the binary library functions on an interpreter. Namespaced:
// every name lives behind the `binary.` prefix.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespaced(LibraryName, "make", makeFn)
	in.RegisterNamespaced(LibraryName, "concat", concatFn)
	in.RegisterNamespaced(LibraryName, "join", joinFn)
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

// makeFn allocates a fresh bytes of `n` bytes, each set to `fill` (default 0):
// binary.make(n[, fill]). This is the byte-buffer allocator - `bytes` has no
// literal, so a fixed-size buffer (a disk-image block, a zeroed frame) otherwise
// has to be conjured through convert.bytesFromString(strings.repeat("\0", n), ...).
// `n` must be in [0, maxMakeBytes] and `fill` a byte value in [0, 255] (matching
// the `$b[] = byte` element rule); a bad size or fill is a positioned error.
func makeFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 && len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("binary.make expects 1 or 2 arguments (n[, fill]), got %d", len(args))
	}
	n, err := takeInt("binary.make", args, 0, "n")
	if err != nil {
		return interpreter.Null(), err
	}
	if n < 0 {
		return interpreter.Null(), fmt.Errorf("binary.make: n must be >= 0, got %d", n)
	}
	if n > maxMakeBytes {
		return interpreter.Null(), fmt.Errorf("binary.make: %d exceeds the %d-byte limit", n, maxMakeBytes)
	}
	fill := int64(0)
	if len(args) == 2 {
		fill, err = takeInt("binary.make", args, 1, "fill")
		if err != nil {
			return interpreter.Null(), err
		}
		if fill < 0 || fill > 255 {
			return interpreter.Null(), fmt.Errorf("binary.make: fill must be a byte value in [0, 255], got %d", fill)
		}
	}
	out := make([]byte, n)
	if fill != 0 {
		for i := range out {
			out[i] = byte(fill)
		}
	}
	return interpreter.BytesVal(out), nil
}

// concatFn joins two byte sequences into a fresh bytes: binary.concat(a, b).
// concat is O(len(a)+len(b)) per call, so a concat-in-a-loop is O(n^2); to
// assemble many chunks you already hold use binary.join (append to a list, join
// once), and to build a buffer from a stream prefer net.readAll / net.readN
// (which grow one Go slice).
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

// joinFn concatenates every `bytes` in `parts`, with an optional separator
// between them, into one freshly-allocated bytes: binary.join(parts[, sep]).
// The byte-data counterpart to strings.join, and the O(n) way to assemble many
// byte chunks - a list-append + one join stays linear where a concat-in-a-loop
// (each binary.concat re-copies the whole accumulator) is O(n^2). `parts` must
// be a `list of bytes`; any non-bytes element is a positioned error. An empty
// list yields empty bytes; the separator defaults to empty.
func joinFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 && len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("binary.join expects 1 or 2 arguments (parts[, sep]), got %d", len(args))
	}
	parts := args[0]
	if parts.Kind != interpreter.KindList {
		return interpreter.Null(), fmt.Errorf("binary.join: first argument must be a list of bytes, got %s", parts.Kind)
	}
	var sep []byte
	if len(args) == 2 {
		s, err := takeBytes("binary.join", args, 1, "separator")
		if err != nil {
			return interpreter.Null(), err
		}
		sep = s
	}
	total := 0
	for i, e := range parts.List {
		if e.Kind != interpreter.KindBytes {
			return interpreter.Null(), fmt.Errorf("binary.join: element %d is %s, expected bytes", i, e.Kind)
		}
		total += len(e.Bytes)
	}
	if len(parts.List) > 1 {
		total += len(sep) * (len(parts.List) - 1)
	}
	out := make([]byte, 0, total)
	for i, e := range parts.List {
		if i > 0 {
			out = append(out, sep...)
		}
		out = append(out, e.Bytes...)
	}
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

// indexOfFn returns the byte index of the first occurrence of needle in haystack
// at or after `from`, or -1 if absent:
// binary.indexOf(haystack, needle[, from[, limit]]). `from` defaults to 0 and
// `limit` to len(haystack); the search is the half-open window [from, limit), so
// a match is reported only when it lies wholly inside it. Bounds are strict:
// 0 <= from <= limit <= len(haystack) (a search may resume at the end - an empty
// tail - but not past it). The `limit` is what keeps a reader that probes a short
// range from overscanning the whole rest of the buffer when the needle is absent:
// `indexOf(src, probe, from, limit)` is `indexOf(slice(src, from, limit), probe)`
// without the slice copy. The `from = idx + 1` idiom makes finding every
// occurrence a single O(n) pass instead of re-slicing the unsearched tail (O(n *
// matches)). An empty needle matches at `from`. Named to match strings.indexOf
// (same argument order, -1-on-absent); byte-indexed, so unlike the rune-indexed
// strings.indexOf it carries the offset. Runs at native speed (bytes.Index) - a
// large-buffer delimiter scan (a MIME boundary, a CRLF) pays no per-byte
// interpreted loop.
func indexOfFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) < 2 || len(args) > 4 {
		return interpreter.Null(), fmt.Errorf("binary.indexOf expects 2 to 4 arguments (haystack, needle[, from[, limit]]), got %d", len(args))
	}
	hay, err := takeBytes("binary.indexOf", args, 0, "haystack")
	if err != nil {
		return interpreter.Null(), err
	}
	needle, err := takeBytes("binary.indexOf", args, 1, "needle")
	if err != nil {
		return interpreter.Null(), err
	}
	from := int64(0)
	if len(args) >= 3 {
		from, err = takeInt("binary.indexOf", args, 2, "from")
		if err != nil {
			return interpreter.Null(), err
		}
		if from < 0 || from > int64(len(hay)) {
			return interpreter.Null(), fmt.Errorf("binary.indexOf: from %d out of range for %d bytes", from, len(hay))
		}
	}
	limit := int64(len(hay))
	if len(args) == 4 {
		limit, err = takeInt("binary.indexOf", args, 3, "limit")
		if err != nil {
			return interpreter.Null(), err
		}
		if limit < from || limit > int64(len(hay)) {
			return interpreter.Null(), fmt.Errorf("binary.indexOf: limit %d out of range for from %d and %d bytes", limit, from, len(hay))
		}
	}
	rel := bytes.Index(hay[from:limit], needle)
	if rel < 0 {
		return interpreter.IntVal(-1), nil
	}
	return interpreter.IntVal(from + int64(rel)), nil
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
