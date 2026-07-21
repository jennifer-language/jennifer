// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package main

import "testing"

func TestIsVerboseFlag(t *testing.T) {
	verbose := []string{"-v", "-vv", "-vvv", "-vvvvv", "--verbose"}
	for _, s := range verbose {
		if !isVerboseFlag(s) {
			t.Errorf("isVerboseFlag(%q) = false, want true", s)
		}
	}
	plain := []string{"", "-", "--v", "-vx", "-x", "verbose", "--verbosee", "-vv-"}
	for _, s := range plain {
		if isVerboseFlag(s) {
			t.Errorf("isVerboseFlag(%q) = true, want false", s)
		}
	}
}
