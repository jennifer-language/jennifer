// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package lint_test

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/lint"
)

// TestDeprecatedRestValues: L301 reports the deprecated names and call shapes
// of M29.39 (ml.free, the fit functions' chained hyper-parameters) and M29.48
// (stats.multipleRegression, convert.toInt on a float, os.hasFlag / os.flag,
// decimal.toString). The current forms are not reported, and L109 / L110
// accept the deprecated shapes.
func TestDeprecatedRestValues(t *testing.T) {
	cfg := lint.DefaultConfig()
	const ml = "use ml;\ndef X as list of list of float init [[1.0], [2.0]];\ndef y as list of int init [0, 1];\n"
	cases := []struct {
		name string
		src  string
		want int
		frag string
	}{
		{"ml.free", ml + "def m as ml.Model init ml.linearRegression($X, [1.0, 2.0]);\nml.free($m);", 1,
			"`ml.free` is deprecated and removed in 0.29.0; use nothing (drop the call; a model is a value"},
		{"logistic lr", ml + "def m as ml.Model init ml.logisticRegression($X, $y, 0.05);", 1,
			"`ml.logisticRegression(X, y, lr[, epochs])` is deprecated and removed in 0.29.0; use `ml.logisticRegressionWith(X, y, ml.LogisticOptions{learningRate: ..., epochs: ...})`"},
		{"logistic lr epochs", ml + "def m as ml.Model init ml.logisticRegression($X, $y, 0.05, 50);", 1, "logisticRegressionWith"},
		{"logistic plain", ml + "def m as ml.Model init ml.logisticRegression($X, $y);", 0, ""},
		{"logisticWith", ml + "def m as ml.Model init ml.logisticRegressionWith($X, $y, ml.LogisticOptions{learningRate: 0.05, epochs: 50});", 0, ""},
		{"kMeans maxIter", ml + "def m as ml.Model init ml.kMeans($X, 2, 10);", 1, "use `ml.kMeansWith(X, k, ml.KMeansOptions{maxIter: ...})`"},
		{"kMeans plain", ml + "def m as ml.Model init ml.kMeans($X, 2);", 0, ""},
		{"kMeansWith", ml + "def m as ml.Model init ml.kMeansWith($X, 2, ml.KMeansOptions{maxIter: 10});", 0, ""},
		{"decisionTree depth", ml + "def m as ml.Model init ml.decisionTree($X, $y, 3);", 1, "use `ml.decisionTreeWith(X, y, ml.TreeOptions{maxDepth: ...})`"},
		{"decisionTreeRegressor depth", ml + "def m as ml.Model init ml.decisionTreeRegressor($X, $y, 3);", 1, "`ml.decisionTreeRegressorWith("},
		{"decisionTreeWith", ml + "def m as ml.Model init ml.decisionTreeWith($X, $y, ml.TreeOptions{maxDepth: 3});", 0, ""},
		{"randomForest nTrees", ml + "def m as ml.Model init ml.randomForest($X, $y, 20);", 1, "use `ml.randomForestWith(X, y, ml.ForestOptions{nTrees: ..., maxDepth: ...})`"},
		{"randomForestRegressor both", "use ml as m;\ndef X as list of list of float;\ndef y as list of float;\ndef r as m.Model init m.randomForestRegressor($X, $y, 20, 4);", 1,
			"`m.randomForestRegressor(X, y, nTrees[, maxDepth])` is deprecated and removed in 0.29.0; use `m.randomForestRegressorWith(X, y, m.ForestOptions{"},
		{"randomForestWith", ml + "def m as ml.Model init ml.randomForestWith($X, $y, ml.ForestOptions{nTrees: 20});", 0, ""},
		{"multipleRegression", "use stats;\ndef c as list of float init stats.multipleRegression([[1.0], [2.0], [3.0]], [1.0, 2.0, 3.0]);", 1,
			"`stats.multipleRegression` is deprecated and removed in 0.29.0; use `ml.linearRegression` with `ml.intercept` / `ml.coefficients`"},
		{"toInt float var", "use convert;\ndef f as float init 2.7;\ndef n as int init convert.toInt($f);", 1,
			"`convert.toInt(float)` truncates and from 0.29.0 accepts only an integral float; use `math.trunc` / `round` / `floor` / `ceil`, which return the int"},
		{"toInt float literal", "use convert;\ndef n as int init convert.toInt(2.5);", 1, "`convert.toInt(float)`"},
		{"toInt float expr", "use convert;\nuse math;\ndef n as int init convert.toInt(math.sqrt(9.0) * 2.0);", 1, "`convert.toInt(float)`"},
		{"toInt integral float literal", "use convert;\ndef n as int init convert.toInt(3.0);", 0, ""},
		{"toInt string", "use convert;\ndef n as int init convert.toInt(\"42\");", 0, ""},
		{"toInt int", "use convert;\ndef n as int init convert.toInt(42);", 0, ""},
		{"toInt radix", "use convert;\ndef n as int init convert.toInt(\"ff\", 16);", 0, ""},
		{"hasFlag", "use os;\ndef v as bool init os.hasFlag(\"-v\");", 1, "`os.hasFlag` is deprecated and removed in 0.29.0; use the `args` module"},
		{"flag", "use os;\ndef p as string init os.flag(\"--port\");", 1, "`os.flag` is deprecated and removed in 0.29.0; use the `args` module"},
		{"decimal.toString", "use decimal;\ndef s as string init decimal.toString(decimal.from(\"1.5\"));", 1,
			"`decimal.toString` is deprecated and removed in 0.29.0; use `convert.toString` or `%v`"},
		{"os.getEnvBytes", "use os;\ndef b as bytes init os.getEnvBytes(\"HOME\");\ndef a as list of bytes init os.argsBytes();", 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := lintSrcResolved(t, c.src, only("L301", "L109", "L110"), cfg)
			if got := countID(diags, "L301"); got != c.want {
				t.Errorf("L301 count = %d, want %d: %v", got, c.want, diags)
			}
			if n := countID(diags, "L109") + countID(diags, "L110"); n != 0 {
				t.Errorf("L109/L110 also reported: %v", diags)
			}
			for _, d := range diags {
				if d.ID == "L301" && !strings.Contains(d.Message, c.frag) {
					t.Errorf("L301 message %q lacks %q", d.Message, c.frag)
				}
			}
		})
	}
}
