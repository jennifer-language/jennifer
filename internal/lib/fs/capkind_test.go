// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package fslib

import (
	"errors"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestCapsAreKindLimit: a reached size cap is kind "limit" (M28.54), not
// "runtime", so a caller tells it from an I/O failure without the message.
func TestCapsAreKindLimit(t *testing.T) {
	var ce *limits.CapError
	if err := wholeTooLarge("fs.readString", "/x"); !errors.As(err, &ce) {
		t.Errorf("readString cap: %T %v", err, err)
	}
	if _, err := readCharsFn(interpreter.BuiltinCtx{}, []Value{makeFile(1), interpreter.IntVal(maxReadChars + 1)}); !errors.As(err, &ce) {
		t.Errorf("readChars cap: %T %v", err, err)
	}
	if _, err := readBytesDispatchFn(interpreter.BuiltinCtx{}, []Value{makeFile(1), interpreter.IntVal(maxHandleRead + 1)}); !errors.As(err, &ce) {
		t.Errorf("readBytes cap: %T %v", err, err)
	}
}
