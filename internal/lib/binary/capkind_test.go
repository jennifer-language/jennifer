// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package binarylib

import (
	"errors"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestCapsAreKindLimit: make / join / split / readInts past their size or
// count caps are kind "limit", not "runtime".
func TestCapsAreKindLimit(t *testing.T) {
	var ce *limits.CapError
	ctx := interpreter.BuiltinCtx{}
	if _, err := makeFn(ctx, []Value{iv(maxMakeBytes + 1)}); !errors.As(err, &ce) {
		t.Errorf("make: %T %v", err, err)
	}
	half := bv(make([]byte, maxMakeBytes/2+1))
	if _, err := joinFn(ctx, []Value{listOf([]Value{half, half})}); !errors.As(err, &ce) {
		t.Errorf("join: %T %v", err, err)
	}
	if _, err := joinFn(ctx, []Value{listOf([]Value{bv([]byte{1}), bv([]byte{2})}), bv(make([]byte, maxMakeBytes))}); !errors.As(err, &ce) {
		t.Errorf("join separator: %T %v", err, err)
	}
	commas := make([]byte, limits.MaxRangeElements)
	for i := range commas {
		commas[i] = ','
	}
	if _, err := splitFn(ctx, []Value{bv(commas), bv([]byte{','})}); !errors.As(err, &ce) {
		t.Errorf("split: %T %v", err, err)
	}
	big := bv(make([]byte, limits.MaxRangeElements+1))
	if _, err := readIntsFn(ctx, []Value{sv("le"), big, iv(0), iv(limits.MaxRangeElements + 1), iv(1)}); !errors.As(err, &ce) {
		t.Errorf("readInts: %T %v", err, err)
	}
}
