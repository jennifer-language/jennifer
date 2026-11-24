// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package timelib

import (
	"testing"
	stdtime "time"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// M29.45: time.sleep(0) returns at once and a negative Duration throws
// "runtime" (it used to return at once like Go's time.Sleep).
func TestSleepBounds(t *testing.T) {
	dur := makeDuration
	ctx := interpreter.BuiltinCtx{}
	start := stdtime.Now()
	if _, err := sleepFn(ctx, []interpreter.Value{dur(0)}); err != nil {
		t.Fatalf("time.sleep(0): %v", err)
	}
	if el := stdtime.Since(start); el > 100*stdtime.Millisecond {
		t.Errorf("time.sleep(0) took %v", el)
	}
	_, err := sleepFn(ctx, []interpreter.Value{dur(-1)})
	if err == nil {
		t.Fatal("time.sleep(-1 ns): no error")
	}
	if k := dataerr.KindOf(err); k != "" {
		t.Errorf("time.sleep(-1 ns): kind %q, want runtime", k)
	}
}
