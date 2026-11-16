// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package asn1lib

import (
	"errors"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestEncodeSizeCapIsKindLimit: an encoding past the size cap, or one whose
// size overflows, is kind "limit" like the nesting and node caps.
func TestEncodeSizeCapIsKindLimit(t *testing.T) {
	var ce *limits.CapError
	for _, doublings := range []int{30, 64} {
		v := mustBuild(t, nullFn)
		for i := 0; i < doublings; i++ {
			v = mustBuild(t, sequenceFn, interpreter.ListVal(childListType, []interpreter.Value{v, v}))
		}
		if _, err := call(encodeFn, v); !errors.As(err, &ce) {
			t.Errorf("%d doublings: %T %v, want a limits.CapError", doublings, err, err)
		}
	}
}
