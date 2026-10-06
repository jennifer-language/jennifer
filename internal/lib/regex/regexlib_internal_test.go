// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package regexlib

import (
	"strings"
	"testing"
)

func TestPatternProgramSizeCap(t *testing.T) {
	resetCacheForTest()
	saved := maxProgramSize
	maxProgramSize = 100
	defer func() { maxProgramSize = saved }()
	if _, err := compilePattern("regex.test", "a{1000}"); err == nil || !strings.Contains(err.Error(), "over the limit") {
		t.Errorf("oversized pattern: expected a program-size error, got %v", err)
	}
	if _, err := compilePattern("regex.test", "abc"); err != nil {
		t.Errorf("small pattern should compile, got %v", err)
	}
}
