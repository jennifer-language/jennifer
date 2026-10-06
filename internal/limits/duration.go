// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package limits

import (
	"math"
	"time"
)

// A time.Duration is an int64 nanosecond count, so forming one from a
// user-supplied count of larger units - `time.Duration(ms) * time.Millisecond`,
// `n * int64(time.Second)` - silently overflows and wraps to a negative or tiny
// duration once the count is large enough. That turns a huge "never time out"
// value into an instant deadline, an immediate TTL expiry, or a NewTicker panic.
// MaxDurationUnits and DurationFitsUnits are the shared guard; each call site
// either clamps an over-range value (treating it as "no deadline" / "no expiry")
// or raises, per that library's contract.

// MaxDurationUnits is the largest non-negative count of unit that fits in a
// time.Duration without overflowing int64 (floor of MaxInt64 / unit).
func MaxDurationUnits(unit time.Duration) int64 {
	if unit <= 1 {
		return math.MaxInt64
	}
	return int64(math.MaxInt64) / int64(unit)
}

// DurationFitsUnits reports whether time.Duration(n)*unit fits in a
// time.Duration without int64 overflow, for n >= 0. A negative n never fits
// (callers reject negatives separately with their own message).
func DurationFitsUnits(n int64, unit time.Duration) bool {
	return n >= 0 && n <= MaxDurationUnits(unit)
}

// DurationFromUnits returns n*unit in nanoseconds and whether it fit in int64,
// for a signed n (a duration may be negative). Used where the result must be
// representable and overflow should raise rather than wrap - the time library's
// duration constructors, which follow the language's "int64 overflow is an
// error" rule.
func DurationFromUnits(n int64, unit time.Duration) (int64, bool) {
	u := int64(unit)
	if u <= 1 {
		return n, true // 1ns or less: scaling cannot overflow
	}
	if n > math.MaxInt64/u || n < math.MinInt64/u {
		return 0, false
	}
	return n * u, true
}
