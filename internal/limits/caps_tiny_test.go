// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build tinygo

package limits

import "testing"

// TestCapValues pins the caps whose values were measured and set on
// 2026-11-24, so a change to one is a deliberate edit here too. The derived
// caps follow MaxRangeElements.
func TestCapValues(t *testing.T) {
	for _, c := range []struct {
		name      string
		got, want int
	}{
		{"MaxTokens", MaxTokens, 1 << 18},
		{"MaxCallDepth", MaxCallDepth, 400},
		{"MaxRangeElements", MaxRangeElements, 1 << 20},
		{"MaxSplitParts", MaxSplitParts, MaxRangeElements / 4},
	} {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}
