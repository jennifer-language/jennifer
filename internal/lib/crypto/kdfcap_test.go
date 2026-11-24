// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package cryptolib

import (
	"errors"
	"math/bits"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestScryptParallelismCap: p up to 4 runs, p 5 is a "limit" error. Before
// 0.27.0 the cap was 16 (ln=20, r=8, p=16 took 48 s).
func TestScryptParallelismCap(t *testing.T) {
	var ce *limits.CapError
	args := func(p int64) []interpreter.Value {
		return []interpreter.Value{interpreter.BytesVal([]byte("pw")), interpreter.BytesVal([]byte("saltsalt")),
			interpreter.IntVal(16), interpreter.IntVal(1), interpreter.IntVal(p), interpreter.IntVal(16)}
	}
	if _, err := scryptFn(interpreter.BuiltinCtx{}, args(4)); err != nil {
		t.Fatalf("p=4: %v", err)
	}
	if _, err := scryptFn(interpreter.BuiltinCtx{}, args(5)); !errors.As(err, &ce) {
		t.Fatalf("p=5: %T %v, want a limits.CapError", err, err)
	}
}

// TestScryptMemoryCapBoundary: 128 * r * 2^ln at maxScryptMemory passes the
// bounds check, one step of r past it does not.
func TestScryptMemoryCapBoundary(t *testing.T) {
	ln := int64(bits.TrailingZeros64(maxScryptMemory / 128))
	if err := scryptBounds(ln, 1, 1); err != nil {
		t.Fatalf("at the cap (ln=%d, r=1): %v", ln, err)
	}
	if err := scryptBounds(ln, 2, 1); err == nil {
		t.Fatalf("past the cap (ln=%d, r=2) passed", ln)
	}
}
