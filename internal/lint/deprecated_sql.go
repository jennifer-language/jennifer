// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package lint

import (
	"fmt"

	"jennifer-lang.dev/jennifer/internal/parser"
)

// checkDeprecatedSQLDriver reports (L301) a `sql.open` / `sql.openWith` whose
// driver argument is the literal "mariadb" or "postgresql": aliases 0.28.0
// deprecates (M29.36, removed in 0.29.0). A driver name from a variable is
// not checked.
func checkDeprecatedSQLDriver(c *checkCtx, lib string, qc *parser.QualifiedCallExpr) {
	if lib != "sql" || (qc.Callee != "open" && qc.Callee != "openWith") || len(qc.Args) == 0 {
		return
	}
	lit, ok := qc.Args[0].(*parser.StringLit)
	if !ok {
		return
	}
	instead := map[string]string{"mariadb": "mysql", "postgresql": "postgres"}[lit.Value]
	if instead == "" {
		return
	}
	c.report("L301", qc, fmt.Sprintf("the `%s.%s` driver %q is deprecated and removed in 0.29.0; use %q", qc.Prefix, qc.Callee, lit.Value, instead))
}
