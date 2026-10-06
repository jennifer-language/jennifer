// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

package netlib

import (
	"math"
	"testing"
	"time"
)

// TestClampMillisNoOverflow proves a huge "never time out" millisecond value
// clamps to the maximum representable duration rather than wrapping negative,
// which turned a deadline into an instant timeout or an infinite wait.
func TestClampMillisNoOverflow(t *testing.T) {
	if d := clampMillis(1000); d != time.Second {
		t.Errorf("clampMillis(1000) = %v, want 1s", d)
	}
	if d := clampMillis(0); d != 0 {
		t.Errorf("clampMillis(0) = %v, want 0", d)
	}
	for _, ms := range []int64{9_300_000_000_000, math.MaxInt64} {
		d := clampMillis(ms)
		if d <= 0 {
			t.Errorf("clampMillis(%d) = %v, must stay positive (no wrap)", ms, d)
		}
		if d != time.Duration(math.MaxInt64)/time.Millisecond*time.Millisecond {
			t.Errorf("clampMillis(%d) = %v, want the max representable millisecond duration", ms, d)
		}
	}
}
