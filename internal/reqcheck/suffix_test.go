// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package reqcheck

import (
	"strings"
	"testing"
)

// TestVersionRangeSuffixRefused: `>=0.25.0+junk` was accepted as `>=0.25.0`.
// A suffix on either bound is an error that says so.
func TestVersionRangeSuffixRefused(t *testing.T) {
	for _, in := range []string{
		">=0.25.0+junk", ">=0.25.0-rc1", ">=0.25.0-dev+183.aa1f0122",
		">=0.25.0 <0.26.0-rc1", ">=0.25.0 <0.26.0+x",
	} {
		_, _, err := ParseVersionRange(in)
		if err == nil || !strings.Contains(err.Error(), "suffix") {
			t.Errorf("ParseVersionRange(%q) = %v, want a suffix error", in, err)
		}
	}
	for _, in := range []string{">=", ">= <0.26.0"} {
		if _, _, err := ParseVersionRange(in); err == nil {
			t.Errorf("ParseVersionRange(%q): expected an error", in)
		}
	}
}
