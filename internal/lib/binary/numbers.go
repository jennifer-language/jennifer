// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package binarylib

import (
	"encoding/binary"
	"fmt"
	"math"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// Fixed-width number readers: integers of 1, 2, 4 or 8 bytes and IEEE 754
// floats of 4 or 8 bytes at a byte offset, in either byte order. A `.j`
// decoder of a binary format (a data file, a wire header) otherwise rebuilds
// each number from single bytes, which costs a dozen interpreted operations
// per value. The bulk forms read `count` values `stride` bytes apart, so a
// column of a row-major record block is one call.
//
// Jennifer has no NaN or infinity, so a float whose bits encode one is an
// error; a format that stores payloads in NaNs (SAS missing values) reads the
// bits with readUint instead.

// takeOrder reads the byte-order argument: "le" or "be".
func takeOrder(fn string, args []Value, idx int) (binary.ByteOrder, error) {
	v := args[idx]
	if v.Kind != interpreter.KindString {
		return nil, fmt.Errorf("%s: order must be a string (\"le\" or \"be\"), got %s", fn, v.Kind)
	}
	switch v.Str {
	case "le":
		return binary.LittleEndian, nil
	case "be":
		return binary.BigEndian, nil
	}
	return nil, fmt.Errorf("%s: order must be \"le\" or \"be\", got %q", fn, v.Str)
}

// Widths accepted by the integer and float readers.
var (
	intSizes   = []int64{1, 2, 4, 8}
	floatSizes = []int64{4, 8}
)

// takeSize reads the width argument and checks it against the allowed set.
func takeSize(fn string, args []Value, idx int, allowed []int64) (int, error) {
	n, err := takeInt(fn, args, idx, "size")
	if err != nil {
		return 0, err
	}
	for _, a := range allowed {
		if n == a {
			return int(n), nil
		}
	}
	return 0, fmt.Errorf("%s: size must be one of %v, got %d", fn, allowed, n)
}

// checkSpan checks that [off, off+size) lies inside b.
func checkSpan(fn string, b []byte, off int64, size int) error {
	if off < 0 {
		return fmt.Errorf("%s: offset %d is negative", fn, off)
	}
	if off > int64(len(b)) || int64(len(b))-off < int64(size) {
		return fmt.Errorf("%s: %d-byte value at offset %d runs past the end of %d bytes", fn, size, off, len(b))
	}
	return nil
}

// rawUint returns the size-byte unsigned value at b[0:size].
func rawUint(b []byte, size int, order binary.ByteOrder) uint64 {
	switch size {
	case 1:
		return uint64(b[0])
	case 2:
		return uint64(order.Uint16(b))
	case 4:
		return uint64(order.Uint32(b))
	}
	return order.Uint64(b)
}

// signed sign-extends a size-byte value.
func signed(u uint64, size int) int64 {
	shift := uint(64 - 8*size)
	return int64(u<<shift) >> shift
}

// floatAt decodes the size-byte IEEE 754 value at b[0:size]; a NaN or an
// infinity is an error.
func floatAt(fn string, b []byte, size int, order binary.ByteOrder, off int64) (float64, error) {
	var f float64
	if size == 4 {
		f = float64(math.Float32frombits(order.Uint32(b)))
	} else {
		f = math.Float64frombits(order.Uint64(b))
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("%s: the %d bytes at offset %d are not a finite number (NaN or infinity)", fn, size, off)
	}
	return f, nil
}

// numberArgs reads (order, b, offset, size) for the single-value readers.
// The byte order is a format name, so it comes first.
func numberArgs(fn string, args []Value, allowed []int64) ([]byte, int64, int, binary.ByteOrder, error) {
	if len(args) != 4 {
		return nil, 0, 0, nil, fmt.Errorf("%s expects 4 arguments (order, b, offset, size), got %d", fn, len(args))
	}
	order, err := takeOrder(fn, args, 0)
	if err != nil {
		return nil, 0, 0, nil, err
	}
	b, err := takeBytes(fn, args, 1, "second argument")
	if err != nil {
		return nil, 0, 0, nil, err
	}
	off, err := takeInt(fn, args, 2, "offset")
	if err != nil {
		return nil, 0, 0, nil, err
	}
	size, err := takeSize(fn, args, 3, allowed)
	if err != nil {
		return nil, 0, 0, nil, err
	}
	if err := checkSpan(fn, b, off, size); err != nil {
		return nil, 0, 0, nil, err
	}
	return b, off, size, order, nil
}

// readIntFn reads a signed integer: binary.readInt(order, b, offset, size).
func readIntFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	b, off, size, order, err := numberArgs("binary.readInt", args, intSizes)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.IntVal(signed(rawUint(b[off:], size, order), size)), nil
}

// readUintFn reads an unsigned integer: binary.readUint(order, b, offset,
// size). An 8-byte value above the int range is an error.
func readUintFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	b, off, size, order, err := numberArgs("binary.readUint", args, intSizes)
	if err != nil {
		return interpreter.Null(), err
	}
	u := rawUint(b[off:], size, order)
	if u > math.MaxInt64 {
		return interpreter.Null(), fmt.Errorf("binary.readUint: the value at offset %d (%d) exceeds the int range", off, u)
	}
	return interpreter.IntVal(int64(u)), nil
}

// readFloatFn reads an IEEE 754 float: binary.readFloat(order, b, offset,
// size), size 4 or 8.
func readFloatFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	b, off, size, order, err := numberArgs("binary.readFloat", args, floatSizes)
	if err != nil {
		return interpreter.Null(), err
	}
	f, err := floatAt("binary.readFloat", b[off:], size, order, off)
	if err != nil {
		return interpreter.Null(), err
	}
	return interpreter.FloatVal(f), nil
}

// bulkArgs reads (order, b, offset, count, size[, stride]) and checks that
// every value lies inside b. stride defaults to size and must be >= 1; count
// is capped at limits.MaxRangeElements (each element is a full Value).
func bulkArgs(fn string, args []Value, allowed []int64) ([]byte, int64, int64, int, binary.ByteOrder, int64, error) {
	if len(args) != 5 && len(args) != 6 {
		return nil, 0, 0, 0, nil, 0, fmt.Errorf("%s expects 5 or 6 arguments (order, b, offset, count, size[, stride]), got %d", fn, len(args))
	}
	order, err := takeOrder(fn, args, 0)
	if err != nil {
		return nil, 0, 0, 0, nil, 0, err
	}
	b, err := takeBytes(fn, args, 1, "second argument")
	if err != nil {
		return nil, 0, 0, 0, nil, 0, err
	}
	off, err := takeInt(fn, args, 2, "offset")
	if err != nil {
		return nil, 0, 0, 0, nil, 0, err
	}
	count, err := takeInt(fn, args, 3, "count")
	if err != nil {
		return nil, 0, 0, 0, nil, 0, err
	}
	size, err := takeSize(fn, args, 4, allowed)
	if err != nil {
		return nil, 0, 0, 0, nil, 0, err
	}
	stride := int64(size)
	if len(args) == 6 {
		stride, err = takeInt(fn, args, 5, "stride")
		if err != nil {
			return nil, 0, 0, 0, nil, 0, err
		}
		if stride < 1 {
			return nil, 0, 0, 0, nil, 0, fmt.Errorf("%s: stride must be >= 1, got %d", fn, stride)
		}
	}
	if count < 0 || count > int64(len(b)) {
		return nil, 0, 0, 0, nil, 0, fmt.Errorf("%s: count must be in [0, %d], got %d", fn, len(b), count)
	}
	if count > limits.MaxRangeElements {
		return nil, 0, 0, 0, nil, 0, fmt.Errorf("%s: count %d exceeds the limit of %d values", fn, count, limits.MaxRangeElements)
	}
	if err := checkStrided(fn, int64(len(b)), off, count, size, stride); err != nil {
		return nil, 0, 0, 0, nil, 0, err
	}
	return b, off, count, size, order, stride, nil
}

// checkStrided checks that count values of size bytes, stride apart from off,
// lie inside n bytes. The offset is checked even when count is 0. Divides
// instead of computing (count-1)*stride, which can overflow int64.
func checkStrided(fn string, n, off, count int64, size int, stride int64) error {
	if off < 0 || off > n {
		return fmt.Errorf("%s: offset %d is outside %d bytes", fn, off, n)
	}
	if count == 0 {
		return nil
	}
	room := n - off - int64(size) // the last value starts at most room bytes after off
	if room < 0 || (count > 1 && stride > room/(count-1)) {
		return fmt.Errorf("%s: %d values %d bytes apart from offset %d run past the end of %d bytes", fn, count, stride, off, n)
	}
	return nil
}

// readIntsFn reads count signed integers: binary.readInts(order, b, offset,
// count, size[, stride]).
func readIntsFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	b, off, count, size, order, stride, err := bulkArgs("binary.readInts", args, intSizes)
	if err != nil {
		return interpreter.Null(), err
	}
	out := make([]Value, count)
	for i := int64(0); i < count; i++ {
		p := off + i*stride
		out[i] = interpreter.IntVal(signed(rawUint(b[p:], size, order), size))
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeInt), out), nil
}

// readFloatsFn reads count IEEE 754 floats: binary.readFloats(order, b,
// offset, count, size[, stride]). A non-finite value is an error naming its
// offset.
func readFloatsFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	b, off, count, size, order, stride, err := bulkArgs("binary.readFloats", args, floatSizes)
	if err != nil {
		return interpreter.Null(), err
	}
	out := make([]Value, count)
	for i := int64(0); i < count; i++ {
		p := off + i*stride
		f, err := floatAt("binary.readFloats", b[p:], size, order, p)
		if err != nil {
			return interpreter.Null(), err
		}
		out[i] = interpreter.FloatVal(f)
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeFloat), out), nil
}
