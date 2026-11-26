// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package binarylib

import (
	"encoding/binary"
	"fmt"
	"math"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// Fixed-width number writers, the inverse of the readers in numbers.go:
// from* build a fresh bytes holding the value(s), set* return a copy of b
// with the value written at an offset. A value that does not fit its size
// is an error, never truncated; a float beyond the float32 range does not
// become an infinity.

// intRange returns the inclusive range of a size-byte signed or unsigned
// integer, clipped to int64 (an 8-byte unsigned takes 0 up to the int max).
func intRange(size int, unsigned bool) (int64, int64) {
	if size == 8 {
		if unsigned {
			return 0, math.MaxInt64
		}
		return math.MinInt64, math.MaxInt64
	}
	bits := uint(8 * size)
	if unsigned {
		return 0, int64(1)<<bits - 1
	}
	return -(int64(1) << (bits - 1)), int64(1)<<(bits-1) - 1
}

// checkIntFits checks n against a size's range; what names the value in the
// message ("value" or "element 3").
func checkIntFits(fn, what string, n int64, size int, unsigned bool) error {
	lo, hi := intRange(size, unsigned)
	if n < lo || n > hi {
		return fmt.Errorf("%s: %s %d is outside the %d-byte range [%d, %d]", fn, what, n, size, lo, hi)
	}
	return nil
}

// putUint writes the low size bytes of u to b[0:size].
func putUint(b []byte, u uint64, size int, order binary.ByteOrder) {
	switch size {
	case 1:
		b[0] = byte(u)
	case 2:
		order.PutUint16(b, uint16(u))
	case 4:
		order.PutUint32(b, uint32(u))
	default:
		order.PutUint64(b, u)
	}
}

// checkFloatFits refuses a non-finite x and, for size 4, a magnitude beyond
// the float32 range; within it the float32 conversion rounds to nearest.
func checkFloatFits(fn, what string, x float64, size int) error {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return fmt.Errorf("%s: %s is not a finite number (NaN or infinity)", fn, what)
	}
	if size == 4 && math.Abs(x) > math.MaxFloat32 {
		return fmt.Errorf("%s: %s %v is outside the float32 range [-%v, %v]", fn, what, x, float32(math.MaxFloat32), float32(math.MaxFloat32))
	}
	return nil
}

// putFloat writes x as a size-byte IEEE 754 value to b[0:size].
func putFloat(b []byte, x float64, size int, order binary.ByteOrder) {
	if size == 4 {
		order.PutUint32(b, math.Float32bits(float32(x)))
		return
	}
	order.PutUint64(b, math.Float64bits(x))
}

// takeFloat reads a positional float argument.
func takeFloat(fn string, args []Value, idx int, role string) (float64, error) {
	if args[idx].Kind != interpreter.KindFloat {
		return 0, fmt.Errorf("%s: %s must be float, got %s", fn, role, args[idx].Kind)
	}
	return args[idx].Float(), nil
}

// fromArgs reads (order, value, size) for the single-value builders.
func fromArgs(fn string, args []Value, allowed []int64) (binary.ByteOrder, int, error) {
	if len(args) != 3 {
		return nil, 0, fmt.Errorf("%s expects 3 arguments (order, value, size), got %d", fn, len(args))
	}
	order, err := takeOrder(fn, args, 0)
	if err != nil {
		return nil, 0, err
	}
	size, err := takeSize(fn, args, 2, allowed)
	if err != nil {
		return nil, 0, err
	}
	return order, size, nil
}

// fromIntegerFn builds binary.fromInt / binary.fromUint.
func fromIntegerFn(fn string, unsigned bool) interpreter.Builtin {
	return func(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
		order, size, err := fromArgs(fn, args, intSizes)
		if err != nil {
			return interpreter.Null(), err
		}
		n, err := takeInt(fn, args, 1, "value")
		if err != nil {
			return interpreter.Null(), err
		}
		if err := checkIntFits(fn, "value", n, size, unsigned); err != nil {
			return interpreter.Null(), err
		}
		out := make([]byte, size)
		putUint(out, uint64(n), size, order)
		return interpreter.BytesVal(out), nil
	}
}

// fromFloatFn: binary.fromFloat(order, x, size), size 4 or 8.
func fromFloatFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	const fn = "binary.fromFloat"
	order, size, err := fromArgs(fn, args, floatSizes)
	if err != nil {
		return interpreter.Null(), err
	}
	x, err := takeFloat(fn, args, 1, "value")
	if err != nil {
		return interpreter.Null(), err
	}
	if err := checkFloatFits(fn, "value", x, size); err != nil {
		return interpreter.Null(), err
	}
	out := make([]byte, size)
	putFloat(out, x, size, order)
	return interpreter.BytesVal(out), nil
}

// bulkResultSize checks that count values of size bytes fit under maxBytes
// (the allocation cap; a parameter so the test can lower it).
func bulkResultSize(fn string, count, size int, maxBytes int64) (int, error) {
	if int64(count) > maxBytes/int64(size) {
		return 0, limits.Capf("%s: %d values of %d bytes exceed the %d-byte limit", fn, count, size, maxBytes)
	}
	return count * size, nil
}

// fromListArgs reads (order, xs, size) for the bulk builders and allocates
// the result.
func fromListArgs(fn string, args []Value, allowed []int64, maxBytes int64) (binary.ByteOrder, []Value, int, []byte, error) {
	order, size, err := fromArgs(fn, args, allowed)
	if err != nil {
		return nil, nil, 0, nil, err
	}
	if args[1].Kind != interpreter.KindList {
		return nil, nil, 0, nil, fmt.Errorf("%s: second argument must be a list, got %s", fn, args[1].Kind)
	}
	xs := args[1].List()
	total, err := bulkResultSize(fn, len(xs), size, maxBytes)
	if err != nil {
		return nil, nil, 0, nil, err
	}
	return order, xs, size, make([]byte, total), nil
}

// fromIntsFn: binary.fromInts(order, xs, size), every element in size
// bytes, back to back, two's complement.
func fromIntsFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	return fromIntsCapped(args, maxMakeBytes)
}

func fromIntsCapped(args []Value, maxBytes int64) (Value, error) {
	const fn = "binary.fromInts"
	order, xs, size, out, err := fromListArgs(fn, args, intSizes, maxBytes)
	if err != nil {
		return interpreter.Null(), err
	}
	for i, e := range xs {
		if e.Kind != interpreter.KindInt {
			return interpreter.Null(), fmt.Errorf("%s: element %d is %s, expected int", fn, i, e.Kind)
		}
		if err := checkIntFits(fn, fmt.Sprintf("element %d", i), e.Int(), size, false); err != nil {
			return interpreter.Null(), err
		}
		putUint(out[i*size:], uint64(e.Int()), size, order)
	}
	return interpreter.BytesVal(out), nil
}

// fromFloatsFn: binary.fromFloats(order, xs, size), every element as a
// size-byte IEEE 754 value, back to back.
func fromFloatsFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	return fromFloatsCapped(args, maxMakeBytes)
}

func fromFloatsCapped(args []Value, maxBytes int64) (Value, error) {
	const fn = "binary.fromFloats"
	order, xs, size, out, err := fromListArgs(fn, args, floatSizes, maxBytes)
	if err != nil {
		return interpreter.Null(), err
	}
	for i, e := range xs {
		if e.Kind != interpreter.KindFloat {
			return interpreter.Null(), fmt.Errorf("%s: element %d is %s, expected float", fn, i, e.Kind)
		}
		if err := checkFloatFits(fn, fmt.Sprintf("element %d", i), e.Float(), size); err != nil {
			return interpreter.Null(), err
		}
		putFloat(out[i*size:], e.Float(), size, order)
	}
	return interpreter.BytesVal(out), nil
}

// setArgs reads (order, b, offset, value, size), checks the span and returns
// a copy of b to write into.
func setArgs(fn string, args []Value, allowed []int64) (binary.ByteOrder, []byte, int64, int, error) {
	if len(args) != 5 {
		return nil, nil, 0, 0, fmt.Errorf("%s expects 5 arguments (order, b, offset, value, size), got %d", fn, len(args))
	}
	order, err := takeOrder(fn, args, 0)
	if err != nil {
		return nil, nil, 0, 0, err
	}
	b, err := takeBytes(fn, args, 1, "second argument")
	if err != nil {
		return nil, nil, 0, 0, err
	}
	off, err := takeInt(fn, args, 2, "offset")
	if err != nil {
		return nil, nil, 0, 0, err
	}
	size, err := takeSize(fn, args, 4, allowed)
	if err != nil {
		return nil, nil, 0, 0, err
	}
	if err := checkSpan(fn, b, off, size); err != nil {
		return nil, nil, 0, 0, err
	}
	out := make([]byte, len(b))
	copy(out, b)
	return order, out, off, size, nil
}

// setIntegerFn builds binary.setInt / binary.setUint.
func setIntegerFn(fn string, unsigned bool) interpreter.Builtin {
	return func(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
		order, out, off, size, err := setArgs(fn, args, intSizes)
		if err != nil {
			return interpreter.Null(), err
		}
		n, err := takeInt(fn, args, 3, "value")
		if err != nil {
			return interpreter.Null(), err
		}
		if err := checkIntFits(fn, "value", n, size, unsigned); err != nil {
			return interpreter.Null(), err
		}
		putUint(out[off:], uint64(n), size, order)
		return interpreter.BytesVal(out), nil
	}
}

// setFloatFn: binary.setFloat(order, b, offset, x, size).
func setFloatFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	const fn = "binary.setFloat"
	order, out, off, size, err := setArgs(fn, args, floatSizes)
	if err != nil {
		return interpreter.Null(), err
	}
	x, err := takeFloat(fn, args, 3, "value")
	if err != nil {
		return interpreter.Null(), err
	}
	if err := checkFloatFits(fn, "value", x, size); err != nil {
		return interpreter.Null(), err
	}
	putFloat(out[off:], x, size, order)
	return interpreter.BytesVal(out), nil
}

var (
	fromIntFn  = fromIntegerFn("binary.fromInt", false)
	fromUintFn = fromIntegerFn("binary.fromUint", true)
	setIntFn   = setIntegerFn("binary.setInt", false)
	setUintFn  = setIntegerFn("binary.setUint", true)
)
