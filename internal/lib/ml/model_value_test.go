// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package mllib

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	tasklib "jennifer-lang.dev/jennifer/internal/lib/task"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// modelOf returns the fitted model inside an ml.Model value.
func modelOf(v interpreter.Value) any {
	nv, ok := v.AsNativeObject(LibraryName, "Model")
	if !ok {
		return nil
	}
	return nv.(*modelValue).m
}

func runML(t *testing.T, src string) (string, error) {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	tasklib.Install(in)
	Install(in)
	err = in.Run(prog)
	return buf.String(), err
}

const mlData = `
use io; use ml; use task;
def X as list of list of float init [[1.0, 1.0], [1.0, 2.0], [2.0, 2.0], [8.0, 8.0], [8.0, 9.0], [9.0, 9.0]];
def y as list of int init [0, 0, 0, 1, 1, 1];
`

// TestModelIsValue (M29.39): a model is a value. A copy equals its original,
// ml.free (deprecated, a no-op) on a copy leaves the original working, and a
// model works from a struct field, a list and a spawned task.
func TestModelIsValue(t *testing.T) {
	out, err := runML(t, mlData+`
def struct Box { m as ml.Model };
def m as ml.Model init ml.logisticRegression($X, $y);
def c as ml.Model init $m;
io.printf("%v %v ", $c == $m, $m == ml.logisticRegression($X, $y));
ml.free($c);
io.printf("%v ", ml.predict($m, [[9.0, 8.0]]));
def b as Box init Box{m: $m};
def l as list of ml.Model init [$m, $c];
io.printf("%v %v ", ml.predict($b.m, [[1.0, 1.5]]), ml.predict($l[1], [[9.0, 9.0]]));
def tk as task of list of int init spawn { return ml.predict($m, [[0.5, 0.5]]); };
io.printf("%v %v", task.wait($tk), $m);
`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "true false [1] [0] [1] [0] <ml.Model logisticRegression>"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// TestZeroModelRefused: an unfitted `def m as ml.Model;` is refused, not a
// handle to whichever model was fitted first.
func TestZeroModelRefused(t *testing.T) {
	_, err := runML(t, mlData+`
def a as ml.Model init ml.linearRegression($X, [1.0, 2.0, 3.0, 4.0, 5.0, 6.0]);
def m as ml.Model;
ml.predict($m, $X);
`)
	if err == nil || !strings.Contains(err.Error(), "not fitted") {
		t.Fatalf("zero model: %v", err)
	}
}

// TestOptionsDefaults: zero options give the defaults of the old optional
// arguments (the same model under the same random seed).
func TestOptionsDefaults(t *testing.T) {
	X := mat([]float64{1, 1}, []float64{1, 2}, []float64{2, 2}, []float64{8, 8}, []float64{8, 9}, []float64{9, 9})
	y := vec(0, 0, 0, 1, 1, 1)
	opt := func(name string, fields ...interpreter.StructField) interpreter.Value {
		return interpreter.NamespacedStructVal(LibraryName, name, fields)
	}
	f := func(n string, v interpreter.Value) interpreter.StructField {
		return interpreter.StructField{Name: n, Value: v}
	}

	lOld := modelOf(ok(t, logisticRegressionFn, X, y, interpreter.FloatVal(0.1), iv(1000))).(*logisticModel)
	lNew := modelOf(ok(t, logisticRegressionWithFn, X, y, opt("LogisticOptions", f("learningRate", interpreter.FloatVal(0)), f("epochs", iv(0))))).(*logisticModel)
	if lOld.b != lNew.b || lOld.w[0] != lNew.w[0] || lOld.w[1] != lNew.w[1] {
		t.Errorf("logistic defaults differ: %v %v / %v %v", lOld.w, lOld.b, lNew.w, lNew.b)
	}
	tOld := modelOf(ok(t, decisionTreeFn, X, y, iv(8))).(*treeModel)
	tNew := modelOf(ok(t, decisionTreeWithFn, X, y, opt("TreeOptions", f("maxDepth", iv(0))))).(*treeModel)
	if depth(tOld.root) != depth(tNew.root) {
		t.Errorf("tree defaults differ")
	}
	fOld := modelOf(ok(t, randomForestFn, X, y, iv(10), iv(8))).(*forestModel)
	fNew := modelOf(ok(t, randomForestWithFn, X, y, opt("ForestOptions", f("nTrees", iv(0)), f("maxDepth", iv(0))))).(*forestModel)
	if len(fOld.trees) != 10 || len(fNew.trees) != 10 {
		t.Errorf("forest defaults: %d / %d trees", len(fOld.trees), len(fNew.trees))
	}
	kOld := modelOf(ok(t, kMeansFn, X, iv(2), iv(100))).(*kMeansModel)
	kNew := modelOf(ok(t, kMeansWithFn, X, iv(2), opt("KMeansOptions", f("maxIter", iv(0))))).(*kMeansModel)
	if len(kOld.centroids) != len(kNew.centroids) {
		t.Errorf("kMeans defaults differ")
	}

	// Negative throws; past the cap is "limit"; a set value is used.
	fails(t, logisticRegressionWithFn, X, y, opt("LogisticOptions", f("learningRate", interpreter.FloatVal(-0.1)), f("epochs", iv(0))))
	fails(t, logisticRegressionWithFn, X, y, opt("LogisticOptions", f("learningRate", interpreter.FloatVal(0)), f("epochs", iv(-1))))
	fails(t, decisionTreeWithFn, X, y, opt("TreeOptions", f("maxDepth", iv(-1))))
	fails(t, randomForestWithFn, X, y, opt("ForestOptions", f("nTrees", iv(-1)), f("maxDepth", iv(0))))
	fails(t, kMeansWithFn, X, iv(2), opt("KMeansOptions", f("maxIter", iv(-5))))
	fails(t, kMeansWithFn, X, iv(2), opt("TreeOptions", f("maxDepth", iv(1))))
	var ce *limits.CapError
	if _, err := randomForestWithFn(interpreter.BuiltinCtx{}, []interpreter.Value{X, y, opt("ForestOptions", f("nTrees", iv(maxForestTrees+1)), f("maxDepth", iv(0)))}); !errors.As(err, &ce) {
		t.Errorf("nTrees past the cap: %T %v", err, err)
	}
	one := modelOf(ok(t, decisionTreeRegressorWithFn, X, y, opt("TreeOptions", f("maxDepth", iv(1))))).(*treeModel)
	if depth(one.root) != 1 {
		t.Errorf("maxDepth 1: depth %d", depth(one.root))
	}
}

func depth(n *treeNode) int {
	if n == nil || n.left == nil {
		return 0
	}
	l, r := depth(n.left), depth(n.right)
	if r > l {
		l = r
	}
	return l + 1
}
