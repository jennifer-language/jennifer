// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package lint

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/parser"
)

// gpioPinOnlyArity is the argument count of each deprecated pin-only `gpio`
// call (M29.49, removed in 0.29.0): the forms on the process-wide chip that
// `gpio.chip` / JENNIFER_GPIO_CHIP select. The current forms take a
// `gpio.Chip` first, one argument more.
var gpioPinOnlyArity = map[string]int{"setup": 2, "read": 1, "write": 2, "release": 1}

// checkDeprecatedRestHandles reports (L301) the deprecated call shapes of the
// handle batch that only the argument count tells apart: the pin-only `gpio`
// calls. Whole deprecated functions (`gpio.chip`, `spi.configure`,
// `httpd.listenTLS`, ...) are in libsig and reported by checkDeprecated.
func checkDeprecatedRestHandles(c *checkCtx, lib string, qc *parser.QualifiedCallExpr) {
	if lib != "gpio" {
		return
	}
	n, ok := gpioPinOnlyArity[qc.Callee]
	if !ok || len(qc.Args) != n {
		return
	}
	p := qc.Prefix
	c.report("L301", qc, fmt.Sprintf("the pin-only `%s.%s` (on the process-wide chip) is deprecated and removed in 0.29.0; "+
		"open the chip with `%s.open(path)` and pass the `%s.Chip` first: `%s.%s(chip, pin, ...)`", p, qc.Callee, p, p, p, qc.Callee))
}
