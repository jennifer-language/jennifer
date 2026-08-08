// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package statslib

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func TestInstallRegistersInferenceStructs(t *testing.T) {
	in := interpreter.New()
	Install(in)
	for _, name := range []string{"mean", "median", "tTest", "tTest2", "fTest", "normalSample", "anova", "describe", "tQuantile"} {
		if in.LookupNamespacedBuiltin("stats", name) == nil {
			t.Errorf("stats.%s is not registered", name)
		}
	}
}

func TestNormalSample(t *testing.T) {
	// A draw is a finite float (finiteResult rejects a non-finite one).
	if v := call(t, normalSampleFn, interpreter.FloatVal(5), interpreter.FloatVal(2)); v.Kind != interpreter.KindFloat {
		t.Errorf("normalSample(5,2) = %+v, want a float", v)
	}
	// sd must be positive; arity is checked.
	callFail(t, normalSampleFn, interpreter.FloatVal(0), interpreter.FloatVal(0))
	callFail(t, normalSampleFn, interpreter.FloatVal(0), interpreter.FloatVal(-1))
	callFail(t, normalSampleFn, interpreter.FloatVal(0))
}

func TestTwoSampleTests(t *testing.T) {
	a := floatList(1, 2, 3, 4, 5)
	b := floatList(3, 4, 5, 6, 7)

	// tTest2 (Welch) and fTest both return a stats.Test struct.
	if v := call(t, tTest2Fn, a, b); v.Kind != interpreter.KindStruct {
		t.Errorf("tTest2 = %+v, want a struct", v)
	}
	if v := call(t, fTestFn, a, b); v.Kind != interpreter.KindStruct {
		t.Errorf("fTest = %+v, want a struct", v)
	}

	// Error branches.
	callFail(t, tTest2Fn, a)                                      // arity
	callFail(t, tTest2Fn, floatList(1), floatList(2, 3))          // fewer than 2 observations
	callFail(t, tTest2Fn, floatList(3, 3, 3), floatList(3, 3, 3)) // both samples zero-variance
	callFail(t, fTestFn, a)                                       // arity
	callFail(t, fTestFn, floatList(1), floatList(2, 3))           // too few observations
}

func TestTQuantile(t *testing.T) {
	// The t distribution is symmetric: the 0.5 quantile is 0 for any df.
	if v := call(t, tQuantileFn, interpreter.FloatVal(0.5), interpreter.FloatVal(10)); v.Kind != interpreter.KindFloat {
		t.Errorf("tQuantile(0.5, 10) = %+v, want a float", v)
	}
	// An upper-tail quantile is positive.
	up := call(t, tQuantileFn, interpreter.FloatVal(0.975), interpreter.FloatVal(10))
	if up.Kind != interpreter.KindFloat || up.Float <= 0 {
		t.Errorf("tQuantile(0.975, 10) = %+v, want a positive float", up)
	}
	// p outside (0, 1) is an error.
	callFail(t, tQuantileFn, interpreter.FloatVal(1.5), interpreter.FloatVal(10))
	callFail(t, tQuantileFn, interpreter.FloatVal(0.5), interpreter.FloatVal(0)) // df must be positive
}
