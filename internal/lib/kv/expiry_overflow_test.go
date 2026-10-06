// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package kv

import (
	"math"
	"testing"
	"time"
)

// TestExpiryForOverflow proves a TTL whose seconds overflow a time.Duration is
// treated as no expiry (zero Time), not wrapped to a past deadline that would
// evict the key immediately.
func TestExpiryForOverflow(t *testing.T) {
	now := time.Now()
	if e := expiryFor(math.MaxInt64, now); !e.IsZero() {
		t.Errorf("expiryFor(MaxInt64) = %v, want zero (no expiry)", e)
	}
	if e := expiryFor(0, now); !e.IsZero() {
		t.Error("ttl 0 should be no expiry")
	}
	if e := expiryFor(60, now); e.IsZero() || !e.After(now) {
		t.Error("ttl 60 should be a future deadline")
	}
}
