// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package lint

import (
	"fmt"
	"math"

	"jennifer-lang.dev/jennifer/internal/parser"
)

// checkDeprecatedRestValues reports (L301) a deprecated call shape of M29.39
// and M29.48; deprecatedRestValues names the fix.
func checkDeprecatedRestValues(c *checkCtx, t *typer, lib string, qc *parser.QualifiedCallExpr, resolve func(string) *binding) {
	if msg := deprecatedRestValues(t, lib, qc, resolve); msg != "" {
		c.report("L301", qc, msg)
	}
}

// deprecatedRestValues names the fix for a deprecated call shape of M29.39
// (the `ml` fit functions' chained hyper-parameter arguments) and M29.48
// (`convert.toInt` on a float), removed in 0.29.0, or "" for any other call.
// A name deprecated outright (`ml.free`, `stats.multipleRegression`, ...) is
// in libsig and reported before this runs.
func deprecatedRestValues(t *typer, lib string, qc *parser.QualifiedCallExpr, resolve func(string) *binding) string {
	p, n := qc.Prefix, len(qc.Args)
	const removed = "is deprecated and removed in 0.29.0"
	switch lib {
	case "convert":
		if qc.Callee != "toInt" || n != 1 {
			return ""
		}
		// An integral float literal is the form 0.29.0 keeps.
		if lit, ok := qc.Args[0].(*parser.FloatLit); ok && lit.Value == math.Trunc(lit.Value) {
			return ""
		}
		if ty, ok := t.typeOf(qc.Args[0], resolve); ok && ty.Kind == parser.TypeFloat {
			return fmt.Sprintf("`%s.toInt(float)` truncates and from 0.29.0 accepts only an integral float; use `math.trunc` / `round` / `floor` / `ceil`, which return the int", p)
		}
	case "ml":
		switch {
		case qc.Callee == "logisticRegression" && n >= 3:
			return fmt.Sprintf("`%s.logisticRegression(X, y, lr[, epochs])` %s; use `%s.logisticRegressionWith(X, y, %s.LogisticOptions{learningRate: ..., epochs: ...})`", p, removed, p, p)
		case qc.Callee == "kMeans" && n == 3:
			return fmt.Sprintf("`%s.kMeans(X, k, maxIter)` %s; use `%s.kMeansWith(X, k, %s.KMeansOptions{maxIter: ...})`", p, removed, p, p)
		case (qc.Callee == "decisionTree" || qc.Callee == "decisionTreeRegressor") && n == 3:
			return fmt.Sprintf("`%s.%s(X, y, maxDepth)` %s; use `%s.%sWith(X, y, %s.TreeOptions{maxDepth: ...})`", p, qc.Callee, removed, p, qc.Callee, p)
		case (qc.Callee == "randomForest" || qc.Callee == "randomForestRegressor") && n >= 3:
			return fmt.Sprintf("`%s.%s(X, y, nTrees[, maxDepth])` %s; use `%s.%sWith(X, y, %s.ForestOptions{nTrees: ..., maxDepth: ...})`", p, qc.Callee, removed, p, qc.Callee, p)
		}
	}
	return ""
}
