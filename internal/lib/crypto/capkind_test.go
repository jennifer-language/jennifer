// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package cryptolib

import (
	"errors"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestRandBytesCapIsKindLimit: a randBytes length past the 64 MiB cap is kind
// "limit"; a negative one stays an ordinary error.
func TestRandBytesCapIsKindLimit(t *testing.T) {
	var ce *limits.CapError
	if _, err := randBytesFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(maxRandBytes + 1)}); !errors.As(err, &ce) {
		t.Errorf("over the cap: %T %v, want a limits.CapError", err, err)
	}
	if _, err := randBytesFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(-1)}); err == nil || errors.As(err, &ce) {
		t.Errorf("negative: %T %v, want a plain error", err, err)
	}
}
