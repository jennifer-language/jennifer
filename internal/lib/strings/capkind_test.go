// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package stringslib

import (
	"errors"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestCapsAreKindLimit: replicate / replace past the byte cap are kind "limit"
// (M28.54), not "runtime".
func TestCapsAreKindLimit(t *testing.T) {
	var ce *limits.CapError
	if _, err := replicate("replicate", []interpreter.Value{interpreter.StringVal("ab"), interpreter.IntVal(1 << 40)}); !errors.As(err, &ce) {
		t.Errorf("replicate: %T %v", err, err)
	}
	big := interpreter.StringVal(string(make([]byte, 1<<20)))
	if _, err := replaceFn(interpreter.BuiltinCtx{}, []interpreter.Value{big, interpreter.StringVal("\x00"), interpreter.StringVal(string(make([]byte, 4096)))}); !errors.As(err, &ce) {
		t.Errorf("replace: %T %v", err, err)
	}
}
