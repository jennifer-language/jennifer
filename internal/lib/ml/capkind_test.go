// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package mllib

import (
	"errors"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestCapsAreKindLimit: kFold past its index cap is kind "limit" (M28.54),
// not "runtime".
func TestCapsAreKindLimit(t *testing.T) {
	var ce *limits.CapError
	if _, err := mlCall("kFold", interpreter.IntVal(1<<40), interpreter.IntVal(1<<20)); !errors.As(err, &ce) {
		t.Errorf("kFold: %T %v", err, err)
	}
}
