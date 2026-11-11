// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package doctree

import (
	"math"
	"strconv"
	"strings"
)

// FormatFloat writes a finite float the way the document encoders share:
// plain digits for 1e-6 <= |f| < 1e21 (`1000000.0`, not `1e+06`), an exponent
// outside that range without zero padding (`1e-7`, `1e+21`), and a `.0` on an
// integral value so it decodes back as a float. It is the form JavaScript
// and Go's encoding/json print, and valid JSON, TOML and YAML alike. The
// caller spells the non-finite values its format has.
func FormatFloat(f float64) string {
	form := byte('f')
	if abs := math.Abs(f); abs != 0 && (abs < 1e-6 || abs >= 1e21) {
		form = 'e'
	}
	s := strconv.FormatFloat(f, form, -1, 64)
	if form == 'e' {
		// e-07 -> e-7
		if n := len(s); n >= 4 && s[n-4] == 'e' && s[n-3] == '-' && s[n-2] == '0' {
			s = s[:n-2] + s[n-1:]
		}
		return s
	}
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}
