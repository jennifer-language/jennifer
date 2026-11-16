// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package regexlib

import (
	"errors"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestCapsAreKindLimit: an over-long or over-large pattern is kind "limit"
// (M28.54), not "runtime"; an invalid pattern stays "runtime".
func TestCapsAreKindLimit(t *testing.T) {
	var ce *limits.CapError
	if _, err := callRegex("matches", strings.Repeat("a", maxPatternBytes+1), "x"); !errors.As(err, &ce) {
		t.Errorf("pattern length: %T %v", err, err)
	}
	if _, err := callRegex("matches", strings.Repeat("x{1000}", 70), "x"); !errors.As(err, &ce) {
		t.Errorf("program size: %T %v", err, err)
	}
	if _, err := callRegex("matches", "x[", "x"); err == nil || errors.As(err, &ce) {
		t.Errorf("invalid pattern: %T %v", err, err)
	}
}
