// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package lint_test

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/lint"
)

// TestRequirementHeaderSuffix: L303 reports a version pragma whose floor or
// bound carries a pre-release or build suffix (it was read as the bare core).
func TestRequirementHeaderSuffix(t *testing.T) {
	for _, v := range []string{">=0.25.0+junk", ">=0.25.0-rc1", ">=0.25.0 <0.26.0-dev"} {
		src := "# pragma-jennifer-version: " + v + "\nuse io;\n"
		d := lintSrc(t, src, only("L303"), lint.DefaultConfig())
		if countID(d, "L303") != 1 || !strings.Contains(d[0].Message, "suffix") {
			t.Errorf("%s: expected one L303 about the suffix, got %v", v, d)
		}
	}
}
