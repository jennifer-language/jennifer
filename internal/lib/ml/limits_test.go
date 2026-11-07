// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package mllib

import (
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

// TestPCAFeatureCap: d features need a d x d covariance; past
// limits.MaxMatrixElements it is refused. 1100 features is a 9.7 MB
// covariance, so the pre-fix code ran (slowly) instead.
func TestPCAFeatureCap(t *testing.T) {
	d := 1100
	if limits.MaxMatrixElements < 1<<20 {
		d = 300
	}
	row := func(off int64) interpreter.Value {
		return intList(d, func(i int) int64 { return int64(i)*off + 1 })
	}
	x := interpreter.ListVal(parser.ListType(parser.PrimitiveType(parser.TypeInt)), []interpreter.Value{row(1), row(2)})
	refuses(t, "ml.pca: ", 4<<20, func() error {
		_, err := mlCall("pca", x, interpreter.IntVal(1))
		return err
	})
}

// TestConfusionMatrixLabelCap: 1100 distinct labels need a 1.21e6-cell
// matrix, past limits.MaxMatrixElements; refused before allocating (40000
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
		_, err := mlCall("kFold", interpreter.IntVal(1_100_000), interpreter.IntVal(2))
		return err
	})
	refuses(t, "ml.kFold: ", 1<<20, func() error {
		_, err := mlCall("kFold", interpreter.IntVal(1<<62), interpreter.IntVal(4))
		return err
	})
	v, err := mlCall("kFold", interpreter.IntVal(10), interpreter.IntVal(3))
	if err != nil || len(v.List) != 3 {
		t.Fatalf("small kFold: %v", err)
	}
}
