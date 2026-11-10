// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package binarylib

import (
	"bytes"
	"math"
	"strings"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// These tests call the builtins directly: a `.j` program copies a builtin's
// result at the `def` bind site, which would hide an aliased result.

func bv(b []byte) Value { return interpreter.BytesVal(b) }
func iv(n int64) Value  { return interpreter.IntVal(n) }
func sv(s string) Value { return interpreter.StringVal(s) }
func listOf(vs []Value) Value {
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeBytes), vs)
}

func call(t *testing.T, fn interpreter.Builtin, args ...Value) Value {
	t.Helper()
	v, err := fn(interpreter.BuiltinCtx{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return v
}

// scribble overwrites every byte of b.
func scribble(b []byte) {
	for i := range b {
		b[i] = 'z'
	}
}

// TestResultsDoNotAlias mutates every bytes result that could share an
// input's backing array and checks the inputs are unchanged.
func TestResultsDoNotAlias(t *testing.T) {
	src := []byte("abc,def")
	key := []byte("1234567")
	empty := []byte{}
	check := func(name string) {
		t.Helper()
		if string(src) != "abc,def" || string(key) != "1234567" {
			t.Fatalf("%s: result aliased an input: src %q key %q", name, src, key)
		}
	}
	scribble(call(t, sliceFn, bv(src), iv(0)).Bytes)
	check("slice whole")
	scribble(call(t, sliceFn, bv(src), iv(1), iv(4)).Bytes)
	check("slice range")
	for _, p := range call(t, splitFn, bv(src), bv([]byte(","))).List {
		scribble(p.Bytes)
	}
	check("split")
	for _, p := range call(t, splitFn, bv(src), bv([]byte(";"))).List {
		scribble(p.Bytes)
	}
	check("split no match")
	scribble(call(t, joinFn, listOf([]Value{bv(src)})).Bytes)
	check("join single part")
	scribble(call(t, joinFn, listOf([]Value{bv(src)}), bv(empty)).Bytes)
	check("join empty sep")
	scribble(call(t, concatFn, bv(src), bv(empty)).Bytes)
	check("concat empty b")
	scribble(call(t, concatFn, bv(empty), bv(src)).Bytes)
	check("concat empty a")
	for _, fn := range []interpreter.Builtin{bitXorFn, bitAndFn, bitOrFn} {
		scribble(call(t, fn, bv(src), bv(key)).Bytes)
		check("bitwise equal-length key")
		scribble(call(t, fn, bv(src[:3]), bv(key)).Bytes)
		check("bitwise long key")
	}
	scribble(call(t, bitNotFn, bv(src)).Bytes)
	check("bitNot")
	for _, fn := range []interpreter.Builtin{readIntsFn, readFloatsFn} {
		call(t, fn, sv("le"), bv(make([]byte, 8)), iv(0), iv(1), iv(8))
	}
}

// TestJoinLimit: a result past maxMakeBytes is a prompt, catchable error,
// not an allocation.
func TestJoinLimit(t *testing.T) {
	sep := make([]byte, 16<<20)
	parts := make([]Value, 1_000_000)
	for i := range parts {
		parts[i] = bv(nil)
	}
	start := time.Now()
	_, err := joinFn(interpreter.BuiltinCtx{}, []Value{listOf(parts), bv(sep)})
	if err == nil || !strings.Contains(err.Error(), "exceeds the") {
		t.Fatalf("got %v, want a size-limit error", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("join took %v to refuse", d)
	}
	// Parts alone past the limit.
	big := bv(make([]byte, maxMakeBytes/2+1))
	_, err = joinFn(interpreter.BuiltinCtx{}, []Value{listOf([]Value{big, big})})
	if err == nil || !strings.Contains(err.Error(), "exceeds the") {
		t.Fatalf("got %v, want a size-limit error", err)
	}
	// Exactly at the limit is allowed.
	half := bv(make([]byte, maxMakeBytes/2-1))
	v := call(t, joinFn, listOf([]Value{half, half}), bv([]byte("ab")))
	if len(v.Bytes) != maxMakeBytes {
		t.Fatalf("len %d, want %d", len(v.Bytes), maxMakeBytes)
	}
}

// TestElementLimit: split, readInts and readFloats refuse a result of more
// than limits.MaxRangeElements values before allocating it.
func TestElementLimit(t *testing.T) {
	n := limits.MaxRangeElements
	seps := bytes.Repeat([]byte{','}, n) // n separators: n + 1 parts
	_, err := splitFn(interpreter.BuiltinCtx{}, []Value{bv(seps), bv([]byte{','})})
	if err == nil || !strings.Contains(err.Error(), "the limit of") {
		t.Fatalf("split: got %v, want an element-limit error", err)
	}
	if v := call(t, splitFn, bv(seps[1:]), bv([]byte{','})); len(v.List) != n {
		t.Fatalf("split at the limit: %d parts, want %d", len(v.List), n)
	}
	buf := make([]byte, 8*(n+1))
	for _, c := range []struct {
		name string
		fn   interpreter.Builtin
		size int64
	}{{"readInts", readIntsFn, 1}, {"readFloats", readFloatsFn, 8}} {
		_, err := c.fn(interpreter.BuiltinCtx{}, []Value{sv("le"), bv(buf), iv(0), iv(int64(n) + 1), iv(c.size)})
		if err == nil || !strings.Contains(err.Error(), "the limit of") {
			t.Fatalf("%s: got %v, want an element-limit error", c.name, err)
		}
	}
}

// TestBulkSpanOverflow: (count-1)*stride wrapping int64 must not pass the
// span check (it did for buffers above ~3 GiB).
func TestBulkSpanOverflow(t *testing.T) {
	const n = int64(1)<<32 + 1
	// (count-1)*stride = 2^64 wraps to 0, so the old check saw last == off.
	if err := checkStrided("f", n, 0, n, 1, 1<<32); err == nil {
		t.Fatal("wrapped span accepted")
	}
	if err := checkStrided("f", n, 0, 2, 1, 1<<32); err != nil {
		t.Fatalf("exact fit refused: %v", err)
	}
	if err := checkStrided("f", n, 1, 2, 1, 1<<32); err == nil {
		t.Fatal("one byte past the end accepted")
	}
	if err := checkStrided("f", 8, 0, 1, 8, 100); err != nil {
		t.Fatalf("count 1 with stride > len refused: %v", err)
	}
	if err := checkStrided("f", 8, 1, 1, 8, 1); err == nil {
		t.Fatal("first value past the end accepted")
	}
}

// TestBitwiseKeyBlocks checks the repeated-key block against a modulo
// reference across key lengths around the block boundaries.
func TestBitwiseKeyBlocks(t *testing.T) {
	ops := []struct {
		fn  interpreter.Builtin
		ref func(a, b byte) byte
	}{
		{bitXorFn, func(a, b byte) byte { return a ^ b }},
		{bitAndFn, func(a, b byte) byte { return a & b }},
		{bitOrFn, func(a, b byte) byte { return a | b }},
	}
	data := make([]byte, 10000)
	for i := range data {
		data[i] = byte(i*7 + i>>8)
	}
	for _, dl := range []int{0, 1, 15, 63, 64, 65, 4095, 4096, 4097, 10000} {
		for _, kl := range []int{1, 3, 4, 63, 64, 65, 4095, 4096, 4097, 9999, 10000, 10001} {
			key := make([]byte, kl)
			for i := range key {
				key[i] = byte(i*13 + 5)
			}
			for _, op := range ops {
				got := call(t, op.fn, bv(data[:dl]), bv(key)).Bytes
				if len(got) != dl {
					t.Fatalf("data %d key %d: len %d", dl, kl, len(got))
				}
				for i := range got {
					if got[i] != op.ref(data[i], key[i%kl]) {
						t.Fatalf("data %d key %d: byte %d = %d, want %d", dl, kl, i, got[i], op.ref(data[i], key[i%kl]))
					}
				}
			}
		}
	}
}

// TestMakeFill checks every byte of a filled buffer, including lengths that
// are not powers of two.
func TestMakeFill(t *testing.T) {
	for _, n := range []int64{0, 1, 2, 3, 1000, 4097} {
		b := call(t, makeFn, iv(n), iv(171)).Bytes
		if int64(len(b)) != n || bytes.Count(b, []byte{171}) != int(n) {
			t.Fatalf("make(%d, 171) = %d bytes, %d filled", n, len(b), bytes.Count(b, []byte{171}))
		}
	}
}

// TestReadFloatEdgePatterns covers the IEEE 754 patterns other than NaN:
// infinities throw, -0 keeps its sign, subnormals decode.
func TestReadFloatEdgePatterns(t *testing.T) {
	read := func(b []byte, size int64, order string) (float64, error) {
		v, err := readFloatFn(interpreter.BuiltinCtx{}, []Value{sv(order), bv(b), iv(0), iv(size)})
		return v.Float, err
	}
	for _, c := range []struct {
		b    []byte
		size int64
	}{
		{[]byte{0x7f, 0x80, 0, 0}, 4},             // float32 +Inf
		{[]byte{0xff, 0x80, 0, 0}, 4},             // float32 -Inf
		{[]byte{0x7f, 0xf0, 0, 0, 0, 0, 0, 0}, 8}, // float64 +Inf
		{[]byte{0xff, 0xf0, 0, 0, 0, 0, 0, 0}, 8}, // float64 -Inf
		{[]byte{0x7f, 0xf0, 0, 0, 0, 0, 0, 1}, 8}, // float64 signalling NaN
		{[]byte{0x7f, 0xc0, 0, 0}, 4},             // float32 quiet NaN
	} {
		if _, err := read(c.b, c.size, "be"); err == nil || !strings.Contains(err.Error(), "not a finite number") {
			t.Errorf("% x: got %v, want a non-finite error", c.b, err)
		}
	}
	for _, size := range []int64{4, 8} {
		b := make([]byte, size)
		b[0] = 0x80
		f, err := read(b, size, "be")
		if err != nil || f != 0 || !math.Signbit(f) {
			t.Errorf("size %d -0: got %v, %v", size, f, err)
		}
	}
	f, err := read([]byte{0, 0, 0, 1}, 4, "be")
	if err != nil || f != float64(math.SmallestNonzeroFloat32) {
		t.Errorf("float32 subnormal: got %v, %v", f, err)
	}
	f, err = read([]byte{1, 0, 0, 0, 0, 0, 0, 0}, 8, "le")
	if err != nil || f != math.SmallestNonzeroFloat64 {
		t.Errorf("float64 subnormal: got %v, %v", f, err)
	}
}
