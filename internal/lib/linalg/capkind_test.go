// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package linalglib

import (
	"errors"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestCapsAreKindLimit: a matrix past the element cap is kind "limit"
// (M28.54), not "runtime".
func TestCapsAreKindLimit(t *testing.T) {
	var ce *limits.CapError
	if _, err := zerosFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(1 << 20), interpreter.IntVal(1 << 20)}); !errors.As(err, &ce) {
		t.Errorf("zeros: %T %v", err, err)
	}
	if _, err := identityFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(1 << 20)}); !errors.As(err, &ce) {
		t.Errorf("identity: %T %v", err, err)
	}
}
