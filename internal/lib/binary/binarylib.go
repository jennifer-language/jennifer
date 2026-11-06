// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package binarylib implements Jennifer's `binary` library: bulk
// operations on `bytes` (concat, join, slice, search, split, bitwise
// ops, fixed-width number reads) run in Go instead of a per-byte `.j`
// loop. Named `binary` because `bytes` is a type keyword.
//
// Every function is non-mutating; each result is freshly allocated and
// never shares an input's backing array.
package binarylib

import (
	"bytes"
	"crypto/subtle"
	"fmt"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the Jennifer name programs `use` to enable these functions.
const LibraryName = "binary"

// maxMakeBytes caps the bytes results of binary.make and binary.join so an
// oversized size (often wire- or caller-derived) is a catchable error instead
// of a Go panic / OOM in the recover-less interpreter. Mirrors net's per-call
// read ceiling.
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
	in.RegisterNamespaced(LibraryName, "bitXor", bitXorFn)
	in.RegisterNamespaced(LibraryName, "bitAnd", bitAndFn)
	in.RegisterNamespaced(LibraryName, "bitOr", bitOrFn)
	in.RegisterNamespaced(LibraryName, "bitNot", bitNotFn)
	in.RegisterNamespaced(LibraryName, "readInt", readIntFn)
	in.RegisterNamespaced(LibraryName, "readUint", readUintFn)
	in.RegisterNamespaced(LibraryName, "readFloat", readFloatFn)
	in.RegisterNamespaced(LibraryName, "readInts", readIntsFn)
	in.RegisterNamespaced(LibraryName, "readFloats", readFloatsFn)
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

// makeFn allocates n bytes set to fill (default 0): binary.make(n[, fill]).
// `bytes` has no literal, so this is the sized-buffer constructor. n must
// be in [0, maxMakeBytes], fill in [0, 255] (the `$b[] = byte` rule).
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
	if fill != 0 && n > 0 {
		out[0] = byte(fill)
		for j := 1; j < len(out); j *= 2 {
			copy(out[j:], out[:j])
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
// list yields empty bytes; the separator defaults to empty. A result above
// maxMakeBytes is an error, checked before allocating.
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
	// int64 and checked per step: the sum cannot overflow, even on 32-bit.
	total := int64(0)
	for i, e := range parts.List {
		if e.Kind != interpreter.KindBytes {
			return interpreter.Null(), fmt.Errorf("binary.join: element %d is %s, expected bytes", i, e.Kind)
		}
		total += int64(len(e.Bytes))
		if total > maxMakeBytes {
			return interpreter.Null(), fmt.Errorf("binary.join: result exceeds the %d-byte limit", maxMakeBytes)
		}
	}
	if gaps := int64(len(parts.List)) - 1; gaps > 0 && len(sep) > 0 {
		if gaps > (maxMakeBytes-total)/int64(len(sep)) {
			return interpreter.Null(), fmt.Errorf("binary.join: result exceeds the %d-byte limit", maxMakeBytes)
		}
		total += gaps * int64(len(sep))
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

// indexOfFn returns the absolute byte offset of the first needle wholly
// inside [from, limit), or -1: binary.indexOf(haystack, needle[, from[,
// limit]]). from defaults to 0, limit to len(haystack); 0 <= from <=
// limit <= len. An empty needle matches at from. `from = idx + 1` finds
// every match in one O(n) pass; limit stops a short probe from scanning
// the rest of the buffer.
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

// splitFn splits b at every occurrence of a non-empty sep:
// binary.split(b, sep). Empty pieces are kept; each part is a copy. More
// than limits.MaxRangeElements parts is an error, checked before allocating.
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
	n := bytes.Count(b, sep) + 1
	if n > limits.MaxRangeElements {
		return interpreter.Null(), fmt.Errorf("binary.split: %d parts exceed the limit of %d", n, limits.MaxRangeElements)
	}
	out := make([]Value, n)
	for i := 0; i < n-1; i++ {
		j := bytes.Index(b, sep)
		out[i] = interpreter.BytesVal(bytes.Clone(b[:j]))
		b = b[j+len(sep):]
	}
	out[n-1] = interpreter.BytesVal(bytes.Clone(b))
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

// bitXorFn, bitAndFn and bitOrFn combine data with key bit-wise, repeating key
// over the data: binary.bitXor / bitAnd / bitOr(data, key). (Not xor / and /
// or: `and` and `or` are keywords.) A key as long as data is a plain
// element-wise operation (SCRAM's ClientKey ^ ClientSignature); a short key is a
// repeating mask (WebSocket's 4-byte frame mask with xor, a fixed bit mask with
// and / or). The result has data's length. An empty key is an error, not a
// silent copy. Key bytes past len(data) are ignored.
func bitXorFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	return bitwise("binary.bitXor", args, func(dst, a, b []byte) {
		subtle.XORBytes(dst, a, b)
	})
}

func bitAndFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	return bitwise("binary.bitAnd", args, func(dst, a, b []byte) {
		for i := range dst {
			dst[i] = a[i] & b[i]
		}
	})
}

func bitOrFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	return bitwise("binary.bitOr", args, func(dst, a, b []byte) {
		for i := range dst {
			dst[i] = a[i] | b[i]
		}
	})
}

// keyBlockBytes is the target length of the repeated-key block in bitwise.
const keyBlockBytes = 4096

// bitwise checks the (data, key) arguments and applies op chunk-wise: op
// combines equal-length slices. A key shorter than data is repeated into
// one block of keyBlockBytes (rounded to a key multiple, at most len(data)),
// so op runs on long slices with no modulo.
func bitwise(fn string, args []Value, op func(dst, a, b []byte)) (Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("%s expects 2 arguments (data, key), got %d", fn, len(args))
	}
	data, err := takeBytes(fn, args, 0, "data")
	if err != nil {
		return interpreter.Null(), err
	}
	key, err := takeBytes(fn, args, 1, "key")
	if err != nil {
		return interpreter.Null(), err
	}
	if len(key) == 0 {
		return interpreter.Null(), fmt.Errorf("%s: key must not be empty", fn)
	}
	out := make([]byte, len(data))
	block := key
	if len(key) < len(data) {
		n := max(keyBlockBytes/len(key), 1) * len(key)
		block = make([]byte, min(n, len(data)))
		copy(block, key)
		for j := len(key); j < len(block); j *= 2 {
			copy(block[j:], block[:j])
		}
	}
	for off := 0; off < len(data); off += len(block) {
		end := off + len(block)
		if end > len(data) {
			end = len(data)
		}
		op(out[off:end], data[off:end], block[:end-off])
	}
	return interpreter.BytesVal(out), nil
}

// bitNotFn inverts every bit of data: binary.bitNot(data).
func bitNotFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("binary.bitNot expects 1 argument (data), got %d", len(args))
	}
	data, err := takeBytes("binary.bitNot", args, 0, "data")
	if err != nil {
		return interpreter.Null(), err
	}
	out := make([]byte, len(data))
	for i, b := range data {
		out[i] = ^b
	}
	return interpreter.BytesVal(out), nil
}
