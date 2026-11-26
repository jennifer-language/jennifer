// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package binarylib

import (
	"bytes"
	"errors"
	"math"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

func fv(x float64) Value { return interpreter.FloatVal(x) }

func intList(ns ...int64) Value {
	vs := make([]Value, len(ns))
	for i, n := range ns {
		vs[i] = iv(n)
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeInt), vs)
}

func floatList(xs ...float64) Value {
	vs := make([]Value, len(xs))
	for i, x := range xs {
		vs[i] = fv(x)
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeFloat), vs)
}

// TestWriteIntRoundTrip: every size and order, at both ends of the range and
// in between, reads back through readInt / readUint / readInts, and set*
// writes the same bytes as from* at an offset.
func TestWriteIntRoundTrip(t *testing.T) {
	for _, order := range []string{"le", "be"} {
		for _, size := range []int64{1, 2, 4, 8} {
			lo, hi := intRange(int(size), false)
			_, uhi := intRange(int(size), true)
			for _, n := range []int64{lo, lo + 1, -1, 0, 1, 0x5a, hi - 1, hi} {
				b := call(t, fromIntFn, sv(order), iv(n), iv(size))
				if len(b.Bytes()) != int(size) {
					t.Fatalf("fromInt %s %d %d: %d bytes", order, n, size, len(b.Bytes()))
				}
				if got := call(t, readIntFn, sv(order), b, iv(0), iv(size)).Int(); got != n {
					t.Errorf("fromInt %s %d size %d read back %d", order, n, size, got)
				}
				set := call(t, setIntFn, sv(order), bv(make([]byte, size+3)), iv(2), iv(n), iv(size))
				if !bytes.Equal(set.Bytes()[2:2+size], b.Bytes()) {
					t.Errorf("setInt %s %d size %d: % x, want % x", order, n, size, set.Bytes(), b.Bytes())
				}
			}
			for _, n := range []int64{0, 1, 0x7f, uhi - 1, uhi} {
				b := call(t, fromUintFn, sv(order), iv(n), iv(size))
				if got := call(t, readUintFn, sv(order), b, iv(0), iv(size)).Int(); got != n {
					t.Errorf("fromUint %s %d size %d read back %d", order, n, size, got)
				}
				set := call(t, setUintFn, sv(order), bv(make([]byte, size)), iv(0), iv(n), iv(size))
				if !bytes.Equal(set.Bytes(), b.Bytes()) {
					t.Errorf("setUint %s %d size %d: % x, want % x", order, n, size, set.Bytes(), b.Bytes())
				}
			}
			ns := []int64{lo, -1, 0, 1, hi}
			b := call(t, fromIntsFn, sv(order), intList(ns...), iv(size))
			back := call(t, readIntsFn, sv(order), b, iv(0), iv(int64(len(ns))), iv(size)).List()
			for i, n := range ns {
				if back[i].Int() != n {
					t.Errorf("fromInts %s size %d element %d: %d, want %d", order, size, i, back[i].Int(), n)
				}
			}
		}
	}
	// Known bytes, so the order is pinned and not only self-consistent.
	if got := call(t, fromIntFn, sv("le"), iv(-2), iv(2)).Bytes(); !bytes.Equal(got, []byte{0xfe, 0xff}) {
		t.Errorf("fromInt le -2: % x", got)
	}
	if got := call(t, fromUintFn, sv("be"), iv(0x01020304), iv(4)).Bytes(); !bytes.Equal(got, []byte{1, 2, 3, 4}) {
		t.Errorf("fromUint be: % x", got)
	}
}

// TestWriteFloatRoundTrip: float64 is exact; float32 rounds to nearest and
// takes the float32 extremes, subnormals included.
func TestWriteFloatRoundTrip(t *testing.T) {
	for _, order := range []string{"le", "be"} {
		for _, x := range []float64{0, -1.5, math.Pi, math.MaxFloat64, -math.MaxFloat64, math.SmallestNonzeroFloat64} {
			b := call(t, fromFloatFn, sv(order), fv(x), iv(8))
			if got := call(t, readFloatFn, sv(order), b, iv(0), iv(8)).Float(); got != x {
				t.Errorf("fromFloat %s %v size 8: %v", order, x, got)
			}
		}
		for _, x := range []float64{0, -1.5, math.Pi, 0.1, math.MaxFloat32, -math.MaxFloat32, math.SmallestNonzeroFloat32} {
			b := call(t, fromFloatFn, sv(order), fv(x), iv(4))
			want := float64(float32(x))
			if got := call(t, readFloatFn, sv(order), b, iv(0), iv(4)).Float(); got != want {
				t.Errorf("fromFloat %s %v size 4: %v, want %v", order, x, got, want)
			}
			set := call(t, setFloatFn, sv(order), bv(make([]byte, 6)), iv(1), fv(x), iv(4))
			if !bytes.Equal(set.Bytes()[1:5], b.Bytes()) {
				t.Errorf("setFloat %s %v: % x, want % x", order, x, set.Bytes(), b.Bytes())
			}
		}
		xs := []float64{1.5, -2.25, 1e300}
		b := call(t, fromFloatsFn, sv(order), floatList(xs...), iv(8))
		back := call(t, readFloatsFn, sv(order), b, iv(0), iv(3), iv(8)).List()
		for i, x := range xs {
			if back[i].Float() != x {
				t.Errorf("fromFloats %s element %d: %v", order, i, back[i].Float())
			}
		}
		b = call(t, fromFloatsFn, sv(order), floatList(0.1, 2.5), iv(4))
		back = call(t, readFloatsFn, sv(order), b, iv(0), iv(2), iv(4)).List()
		if back[0].Float() != float64(float32(0.1)) || back[1].Float() != 2.5 {
			t.Errorf("fromFloats %s size 4: %v", order, back)
		}
	}
	if got := call(t, fromFloatFn, sv("be"), fv(1.5), iv(4)).Bytes(); !bytes.Equal(got, []byte{0x3f, 0xc0, 0, 0}) {
		t.Errorf("fromFloat be 1.5: % x", got)
	}
}

// TestSetLeavesRestAlone: set* changes only its span, returns a copy, and
// leaves the input untouched.
func TestSetLeavesRestAlone(t *testing.T) {
	in := []byte{9, 9, 9, 9, 9, 9, 9, 9}
	got := call(t, setUintFn, sv("le"), bv(in), iv(3), iv(0x0102), iv(2)).Bytes()
	if !bytes.Equal(got, []byte{9, 9, 9, 2, 1, 9, 9, 9}) {
		t.Errorf("setUint: % x", got)
	}
	if !bytes.Equal(in, []byte{9, 9, 9, 9, 9, 9, 9, 9}) {
		t.Errorf("setUint changed its input: % x", in)
	}
	got = call(t, setIntFn, sv("be"), bv(in), iv(4), iv(-1), iv(4)).Bytes()
	if !bytes.Equal(got, []byte{9, 9, 9, 9, 255, 255, 255, 255}) {
		t.Errorf("setInt at the end: % x", got)
	}
	if got := call(t, fromIntsFn, sv("le"), intList(), iv(4)).Bytes(); len(got) != 0 {
		t.Errorf("fromInts of an empty list: % x", got)
	}
}

// TestWriteRefusals pins every refusal, its message and its kind: misuse is
// "runtime" (dataerr kind ""), naming the range.
func TestWriteRefusals(t *testing.T) {
	b := bv(make([]byte, 4))
	cases := []struct {
		name string
		fn   interpreter.Builtin
		args []Value
		want string
	}{
		{"order", fromIntFn, []Value{sv("LE"), iv(1), iv(1)}, `order must be "le" or "be"`},
		{"order type", fromIntFn, []Value{iv(0), iv(1), iv(1)}, "order must be a string"},
		{"int size", fromIntFn, []Value{sv("le"), iv(1), iv(3)}, "size must be one of [1 2 4 8], got 3"},
		{"float size", fromFloatFn, []Value{sv("le"), fv(1), iv(2)}, "size must be one of [4 8], got 2"},
		{"uint 256 in 1", fromUintFn, []Value{sv("le"), iv(256), iv(1)}, "value 256 is outside the 1-byte range [0, 255]"},
		{"uint negative", fromUintFn, []Value{sv("le"), iv(-1), iv(8)}, "value -1 is outside the 8-byte range [0, 9223372036854775807]"},
		{"int 128 in 1", fromIntFn, []Value{sv("be"), iv(128), iv(1)}, "value 128 is outside the 1-byte range [-128, 127]"},
		{"int -32769 in 2", fromIntFn, []Value{sv("be"), iv(-32769), iv(2)}, "range [-32768, 32767]"},
		{"int 2^31 in 4", fromIntFn, []Value{sv("be"), iv(1 << 31), iv(4)}, "range [-2147483648, 2147483647]"},
		{"uint 2^32 in 4", fromUintFn, []Value{sv("be"), iv(1 << 32), iv(4)}, "range [0, 4294967295]"},
		{"int value type", fromIntFn, []Value{sv("le"), fv(1), iv(1)}, "value must be int, got float"},
		{"float value type", fromFloatFn, []Value{sv("le"), iv(1), iv(8)}, "value must be float, got int"},
		{"NaN", fromFloatFn, []Value{sv("le"), fv(math.NaN()), iv(8)}, "not a finite number"},
		{"infinity", fromFloatFn, []Value{sv("le"), fv(math.Inf(-1)), iv(4)}, "not a finite number"},
		{"float32 overflow", fromFloatFn, []Value{sv("le"), fv(1e39), iv(4)}, "outside the float32 range"},
		{"float32 just past max", fromFloatFn, []Value{sv("le"), fv(math.Nextafter(math.MaxFloat32, math.Inf(1))), iv(4)}, "outside the float32 range"},
		{"from arity", fromIntFn, []Value{sv("le"), iv(1)}, "expects 3 arguments"},
		{"ints element type", fromIntsFn, []Value{sv("le"), floatList(1), iv(1)}, "element 0 is float, expected int"},
		{"ints element range", fromIntsFn, []Value{sv("le"), intList(1, 2, 300), iv(1)}, "element 2 300 is outside the 1-byte range [-128, 127]"},
		{"ints not a list", fromIntsFn, []Value{sv("le"), iv(1), iv(1)}, "second argument must be a list"},
		{"floats element type", fromFloatsFn, []Value{sv("le"), intList(1), iv(8)}, "element 0 is int, expected float"},
		{"floats element range", fromFloatsFn, []Value{sv("le"), floatList(1, -1e40), iv(4)}, "element 1 -1e+40 is outside the float32 range"},
		{"floats element NaN", fromFloatsFn, []Value{sv("le"), floatList(math.NaN()), iv(8)}, "element 0 is not a finite number"},
		{"set past end", setUintFn, []Value{sv("le"), b, iv(1), iv(0), iv(4)}, "4-byte value at offset 1 runs past the end of 4 bytes"},
		{"set offset past end", setIntFn, []Value{sv("le"), b, iv(5), iv(0), iv(1)}, "runs past the end of 4 bytes"},
		{"set negative offset", setFloatFn, []Value{sv("le"), b, iv(-1), fv(0), iv(4)}, "offset -1 is negative"},
		{"set value range", setUintFn, []Value{sv("le"), b, iv(0), iv(65536), iv(2)}, "value 65536 is outside the 2-byte range [0, 65535]"},
		{"set float32 overflow", setFloatFn, []Value{sv("le"), b, iv(0), fv(-1e39), iv(4)}, "outside the float32 range"},
		{"set b type", setIntFn, []Value{sv("le"), iv(0), iv(0), iv(0), iv(1)}, "second argument must be bytes"},
		{"set arity", setIntFn, []Value{sv("le"), b, iv(0), iv(0)}, "expects 5 arguments"},
	}
	for _, c := range cases {
		_, err := c.fn(interpreter.BuiltinCtx{}, c.args)
		if err == nil {
			t.Errorf("%s: no error", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
		if k := dataerr.KindOf(err); k != "" {
			t.Errorf("%s: kind %q, want runtime", c.name, k)
		}
		var ce *limits.CapError
		if errors.As(err, &ce) {
			t.Errorf("%s: a cap error", c.name)
		}
	}
}

// TestWriteBulkCap: a bulk result over the allocation cap is kind "limit",
// checked before any element. The real cap (256 MiB) needs a list of 32M
// values, so the check runs with a lowered cap; the bound at the cap passes.
func TestWriteBulkCap(t *testing.T) {
	var ce *limits.CapError
	if _, err := fromIntsCapped([]Value{sv("le"), intList(1, 2, 3), iv(4)}, 11); !errors.As(err, &ce) {
		t.Errorf("fromInts over the cap: %T %v", err, err)
	}
	if _, err := fromIntsCapped([]Value{sv("le"), intList(1, 2, 3), iv(4)}, 12); err != nil {
		t.Errorf("fromInts at the cap: %v", err)
	}
	if _, err := fromFloatsCapped([]Value{sv("le"), floatList(1, 2), iv(8)}, 15); !errors.As(err, &ce) {
		t.Errorf("fromFloats over the cap: %T %v", err, err)
	} else if !strings.Contains(err.Error(), "2 values of 8 bytes exceed the 15-byte limit") {
		t.Errorf("fromFloats cap message: %v", err)
	}
	if maxMakeBytes != limits.MaxAllocBytes {
		t.Errorf("bulk writers cap at %d, want limits.MaxAllocBytes", maxMakeBytes)
	}
}
