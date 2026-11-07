// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package timelib

import (
	"math"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// TestHandBuiltOffsetRejected: a time.Time or time.Zone literal with an offset
// past the 26 h cap is refused at the call boundary, as time.zone refuses it.
// Before, iso/format printed nonsense offsets and inZone shifted the calendar
// by centuries.
func TestHandBuiltOffsetRejected(t *testing.T) {
	ctx := interpreter.BuiltinCtx{}
	tm := func(off int64) interpreter.Value {
		return interpreter.NamespacedStructVal(LibraryName, "Time", []interpreter.StructField{
			{Name: "nanos", Value: interpreter.IntVal(0)},
			{Name: "offset", Value: interpreter.IntVal(off)},
		})
	}
	for _, off := range []int64{math.MinInt64, math.MaxInt64, maxOffsetSeconds + 1, -maxOffsetSeconds - 1} {
		if _, err := isoFn(ctx, []interpreter.Value{tm(off)}); err == nil || !strings.Contains(err.Error(), "outside") {
			t.Errorf("iso(offset %d): err = %v, want an out-of-range error", off, err)
		}
		if _, err := formatFn(ctx, []interpreter.Value{tm(off), interpreter.StringVal("%z")}); err == nil || !strings.Contains(err.Error(), "outside") {
			t.Errorf("format(offset %d): err = %v, want an out-of-range error", off, err)
		}
		zone := makeZone(off, "x")
		if _, err := inZoneFn(ctx, []interpreter.Value{tm(0), zone}); err == nil || !strings.Contains(err.Error(), "outside") {
			t.Errorf("inZone(zone offset %d): err = %v, want an out-of-range error", off, err)
		}
	}
	if _, err := isoFn(ctx, []interpreter.Value{tm(maxOffsetSeconds)}); err != nil {
		t.Errorf("iso(offset %d): %v", maxOffsetSeconds, err)
	}
	if _, err := inZoneFn(ctx, []interpreter.Value{tm(0), makeZone(-maxOffsetSeconds, "x")}); err != nil {
		t.Errorf("inZone(offset %d): %v", -maxOffsetSeconds, err)
	}
}
