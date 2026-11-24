// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package imageslib

import (
	"math"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func decodeOpts(maxPixels int64) Value {
	return interpreter.NamespacedStructVal(LibraryName, "DecodeOptions", []interpreter.StructField{
		{Name: "maxPixels", Value: interpreter.IntVal(maxPixels)},
	})
}

// M29.44: decodeWith's maxPixels is 0 for the default, a negative value
// throws "runtime", math.MAX_INT decodes without a cap, a reached cap is
// "limit".
func TestDecodeWith(t *testing.T) {
	png := call(t, encodeFn, str("png"), checker(4, 3))
	want := call(t, decodeFn, png)
	for _, m := range []int64{0, 12, math.MaxInt64} {
		got := call(t, decodeWithFn, png, decodeOpts(m))
		if !payload(t, got).EqualNative(payload(t, want)) {
			t.Errorf("maxPixels %d: image differs from images.decode's", m)
		}
	}
	_, err := decodeWithFn(interpreter.BuiltinCtx{}, []Value{png, decodeOpts(11)})
	if k := errorKind(t, err); k != "limit" {
		t.Errorf("maxPixels 11: kind %q, want limit", k)
	}
	_, err = decodeWithFn(interpreter.BuiltinCtx{}, []Value{png, decodeOpts(-1)})
	if err == nil || dataerr.KindOf(err) != "" {
		t.Errorf("maxPixels -1: %v, want a runtime error", err)
	}
	fails(t, decodeWithFn, "must be an images.DecodeOptions", png, num(12))
	// The deprecated positional form keeps refusing a value below 1.
	fails(t, decodeFn, "maxPixels must be at least 1", png, num(0))
	if _, err := decodeFn(interpreter.BuiltinCtx{}, []Value{png, num(11)}); errorKind(t, err) != "limit" {
		t.Errorf("positional maxPixels 11: want kind limit")
	}
}
