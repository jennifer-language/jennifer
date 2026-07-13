// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package mathlib

import (
	"math"
	"testing"

	"github.com/mplx/jennifer-lang/internal/interpreter"
)

// randInt over ranges whose width exceeds 2^63 must not panic (Int63n
// rejects a non-positive span) and must return a value inside [lo, hi].
// Covers both wide-range branches: uspan == 0 (the full int64 range) and
// uspan != 0 (a wide but not-full range).
func TestRandIntWideRanges(t *testing.T) {
	cases := []struct {
		name   string
		lo, hi int64
	}{
		{"full-int64-range", math.MinInt64, math.MaxInt64},
		{"zero-to-max", 0, math.MaxInt64},
		{"min-to-zero", math.MinInt64, 0},
		{"narrow", -5, 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			args := []interpreter.Value{interpreter.IntVal(c.lo), interpreter.IntVal(c.hi)}
			for i := 0; i < 1000; i++ {
				v, err := randIntFn(interpreter.BuiltinCtx{}, args)
				if err != nil {
					t.Fatalf("randInt(%d, %d): unexpected error: %v", c.lo, c.hi, err)
				}
				if v.Kind != interpreter.KindInt {
					t.Fatalf("randInt returned %s, want int", v.Kind)
				}
				if v.Int < c.lo || v.Int > c.hi {
					t.Fatalf("randInt(%d, %d) = %d, out of range", c.lo, c.hi, v.Int)
				}
			}
		})
	}
}
