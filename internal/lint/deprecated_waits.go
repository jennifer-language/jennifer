// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package lint

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/parser"
)

// checkDeprecatedWaits reports (L301) the optional-argument call shapes 0.28.0
// deprecates for options structs (M29.45; removed in 0.29.0, M30.25) on a call
// to the built-in library lib: the positional interval of `fs.watch` and the
// options map of `os.spawn`.
func checkDeprecatedWaits(c *checkCtx, lib string, qc *parser.QualifiedCallExpr) {
	if msg := deprecatedWaitShape(lib, qc); msg != "" {
		c.report("L301", qc, msg)
	}
}

// deprecatedWaitShape names the fix for a deprecated call shape, or "".
func deprecatedWaitShape(lib string, qc *parser.QualifiedCallExpr) string {
	p, n := qc.Prefix, len(qc.Args)
	const removed = "is deprecated and removed in 0.29.0"
	switch {
	case lib == "fs" && qc.Callee == "watch" && n == 2:
		return fmt.Sprintf("`%s.watch(path, intervalMs)` %s; use `%s.watchWith(path, %s.WatchOptions{intervalMs: ...})`", p, removed, p, p)
	case lib == "os" && qc.Callee == "spawn" && n == 2:
		return fmt.Sprintf("`%s.spawn(argv, options)` %s; use `%s.spawnWith(argv, %s.SpawnOptions{stdout: ..., stderr: ...})`", p, removed, p, p)
	}
	return ""
}
