// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package timelib

import (
	"math"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// TestDurationConstructorOverflow proves the duration constructors raise a
// catchable error (not a silent wrap) when the count overflows a time.Duration.
func TestDurationConstructorOverflow(t *testing.T) {
	ctx := interpreter.BuiltinCtx{}
	fns := map[string]func(interpreter.BuiltinCtx, []interpreter.Value) (interpreter.Value, error){
		"fromSeconds":      fromSecondsFn,
		"fromMilliseconds": fromMillisecondsFn,
		"fromMinutes":      fromMinutesFn,
		"fromHours":        fromHoursFn,
	}
	for name, fn := range fns {
		if _, err := fn(ctx, []interpreter.Value{interpreter.IntVal(math.MaxInt64)}); err == nil || !strings.Contains(err.Error(), "overflow") {
			t.Errorf("%s(MaxInt64): expected an overflow error, got %v", name, err)
		}
		if _, err := fn(ctx, []interpreter.Value{interpreter.IntVal(math.MinInt64)}); err == nil || !strings.Contains(err.Error(), "overflow") {
			t.Errorf("%s(MinInt64): expected an overflow error, got %v", name, err)
		}
		// A small value still constructs.
		if _, err := fn(ctx, []interpreter.Value{interpreter.IntVal(3)}); err != nil {
			t.Errorf("%s(3): unexpected error %v", name, err)
		}
	}
}

// TestSubOverflow proves time.sub detects int64 wrap in the nanosecond
// difference of two far-apart instants and raises instead of returning a wrong
// (wrapped) duration.
func TestSubOverflow(t *testing.T) {
	ctx := interpreter.BuiltinCtx{}
	hi, err := fromUnixNanosFn(ctx, []interpreter.Value{interpreter.IntVal(math.MaxInt64)})
	if err != nil {
		t.Fatal(err)
	}
	lo, err := fromUnixNanosFn(ctx, []interpreter.Value{interpreter.IntVal(math.MinInt64)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := subFn(ctx, []interpreter.Value{hi, lo}); err == nil || !strings.Contains(err.Error(), "overflow") {
		t.Errorf("sub(MaxInt64ns, MinInt64ns): expected an overflow error, got %v", err)
	}
	// Two close instants subtract fine.
	if _, err := subFn(ctx, []interpreter.Value{hi, hi}); err != nil {
		t.Errorf("sub(t, t): unexpected error %v", err)
	}
}
