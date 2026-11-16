// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package listslib

import (
	"errors"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestCapsAreKindLimit: push / concat / range past their caps are kind
// "limit" (M28.54), not "runtime".
func TestCapsAreKindLimit(t *testing.T) {
	defer func(n int) { maxListElements = n }(maxListElements)
	maxListElements = 10
	full := interpreter.Value{Kind: interpreter.KindList, List: make([]interpreter.Value, 10)}
	var ce *limits.CapError
	if _, err := pushFn(interpreter.BuiltinCtx{}, []interpreter.Value{full, interpreter.IntVal(1)}); !errors.As(err, &ce) {
		t.Errorf("push: %T %v", err, err)
	}
	if _, err := concatFn(interpreter.BuiltinCtx{}, []interpreter.Value{full, full}); !errors.As(err, &ce) {
		t.Errorf("concat: %T %v", err, err)
	}
	if _, err := rangeFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(0), interpreter.IntVal(1 << 40)}); !errors.As(err, &ce) {
		t.Errorf("range: %T %v", err, err)
	}
}
