// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package limits

import (
	"math"
	"testing"
	"time"
)

func TestDurationFitsUnits(t *testing.T) {
	maxMs := MaxDurationUnits(time.Millisecond)
	cases := []struct {
		n    int64
		unit time.Duration
		want bool
	}{
		{0, time.Millisecond, true},
		{1000, time.Millisecond, true},
		{maxMs, time.Millisecond, true},
		{maxMs + 1, time.Millisecond, false},
		{math.MaxInt64, time.Millisecond, false},
		{-1, time.Millisecond, false}, // negatives never fit (callers reject separately)
		{math.MaxInt64, time.Nanosecond, true},
	}
	for _, c := range cases {
		if got := DurationFitsUnits(c.n, c.unit); got != c.want {
			t.Errorf("DurationFitsUnits(%d, %v) = %v, want %v", c.n, c.unit, got, c.want)
		}
	}
}

func TestDurationFromUnits(t *testing.T) {
	if ns, ok := DurationFromUnits(5, time.Second); !ok || ns != 5*int64(time.Second) {
		t.Errorf("fromUnits(5s) = %d, %v", ns, ok)
	}
	if ns, ok := DurationFromUnits(-5, time.Second); !ok || ns != -5*int64(time.Second) {
		t.Errorf("fromUnits(-5s) = %d, %v", ns, ok)
	}
	// Overflow both directions.
	if _, ok := DurationFromUnits(math.MaxInt64, time.Second); ok {
		t.Error("MaxInt64 seconds should overflow")
	}
	if _, ok := DurationFromUnits(math.MinInt64, time.Second); ok {
		t.Error("MinInt64 seconds should overflow")
	}
	// A nanosecond unit never overflows from scaling.
	if ns, ok := DurationFromUnits(math.MaxInt64, time.Nanosecond); !ok || ns != math.MaxInt64 {
		t.Errorf("MaxInt64 ns = %d, %v", ns, ok)
	}
}
