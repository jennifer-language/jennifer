// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package mllib

import (
	"errors"
	"math"
	"runtime"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

func mlCall(name string, args ...interpreter.Value) (interpreter.Value, error) {
	in := interpreter.New()
	Install(in)
	return in.LookupNamespacedBuiltin("ml", name)(interpreter.BuiltinCtx{}, args)
}

func intList(n int, f func(i int) int64) interpreter.Value {
	out := make([]interpreter.Value, n)
	for i := range out {
		out[i] = interpreter.IntVal(f(i))
	}
	return interpreter.ListVal(parser.PrimitiveType(parser.TypeInt), out)
}

// refuses runs f and fails unless it errors with want, quickly, without
// allocating the oversized result (the pre-fix code allocated it).
func refuses(t *testing.T, want string, maxAlloc uint64, f func() error) {
	t.Helper()
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	err := f()
	runtime.ReadMemStats(&b)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("got %v, want %q", err, want)
	}
	if got := b.TotalAlloc - a.TotalAlloc; got > maxAlloc {
		t.Fatalf("allocated %d bytes before refusing", got)
	}
}

// TestPCAFeatureCap: the Jacobi solve costs about 3*d^3 per sweep, so past
// maxPCAFeatures features pca throws "limit" before building the covariance.
func TestPCAFeatureCap(t *testing.T) {
	d := maxPCAFeatures + 1
	row := func(off int64) interpreter.Value {
		return intList(d, func(i int) int64 { return int64(i)*off + 1 })
	}
	x := interpreter.ListVal(parser.ListType(parser.PrimitiveType(parser.TypeInt)), []interpreter.Value{row(1), row(2)})
	refuses(t, "ml.pca: ", 4<<20, func() error {
		_, err := mlCall("pca", x, interpreter.IntVal(1))
		var ce *limits.CapError
		if !errors.As(err, &ce) {
			t.Fatalf("got %v (%T), want a limits.CapError", err, err)
		}
		return err
	})
}

// TestPCACancel: a cancelled task stops pca at the next Jacobi sweep.
func TestPCACancel(t *testing.T) {
	rows := make([][]float64, 3)
	for i := range rows {
		rows[i] = make([]float64, 8)
		for j := range rows[i] {
			rows[i][j] = float64((i+1)*(j+3)%7) + float64(j)
		}
	}
	ts := &interpreter.TaskState{}
	ts.RequestCancel()
	_, err := pcaFn(interpreter.BuiltinCtx{Cancel: ts}, []interpreter.Value{mat(rows...), interpreter.IntVal(1)})
	if err == nil || !strings.Contains(err.Error(), "task cancelled") {
		t.Fatalf("got %v, want task cancelled", err)
	}
	if _, err := pcaFn(interpreter.BuiltinCtx{Cancel: &interpreter.TaskState{}}, []interpreter.Value{mat(rows...), interpreter.IntVal(1)}); err != nil {
		t.Fatalf("uncancelled task: %v", err)
	}
}

// TestConfusionMatrixLabelCap: just past sqrt(limits.MaxMatrixElements)
// distinct labels need a matrix past the cap; refused before allocating (40000
// labels were 12.8 GB, a fatal OOM, before the cap).
func TestConfusionMatrixLabelCap(t *testing.T) {
	n := int(math.Sqrt(float64(limits.MaxMatrixElements))) + 76
	y := intList(n, func(i int) int64 { return int64(i) })
	refuses(t, "ml.confusionMatrix: ", 32<<20, func() error {
		_, err := mlCall("confusionMatrix", y, y)
		return err
	})
}

// TestKFoldCap: nSamples*k past maxFoldWork is refused; MaxInt64 inputs do
// not wrap past the check.
func TestKFoldCap(t *testing.T) {
	refuses(t, "ml.kFold: nSamples*k exceeds", 1<<20, func() error {
		_, err := mlCall("kFold", interpreter.IntVal(int64(maxFoldWork)/2+1), interpreter.IntVal(2))
		return err
	})
	refuses(t, "ml.kFold: ", 1<<20, func() error {
		_, err := mlCall("kFold", interpreter.IntVal(1<<62), interpreter.IntVal(4))
		return err
	})
	v, err := mlCall("kFold", interpreter.IntVal(10), interpreter.IntVal(3))
	if err != nil || len(v.List()) != 3 {
		t.Fatalf("small kFold: %v", err)
	}
}
