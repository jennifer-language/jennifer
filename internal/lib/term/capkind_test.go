// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package termlib

import (
	"errors"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestMakeRawCapIsKindLimit: makeRaw past the raw-state cap is kind "limit"
// (M29.1). The cap is checked before the terminal is touched, so placeholder
// entries reach it without a tty.
func TestMakeRawCapIsKindLimit(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	statesMu.Lock()
	for id := 0; id < maxRawStates; id++ {
		states[id] = &rawEntry{fd: -1}
	}
	statesMu.Unlock()
	var ce *limits.CapError
	if _, err := makeRawFn(interpreter.BuiltinCtx{}, []Value{interpreter.StringVal("stdin")}); !errors.As(err, &ce) {
		t.Errorf("makeRaw at the cap: %T %v, want a limits.CapError", err, err)
	}
}
