// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package convert

import (
	"math"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// convert.toInt truncates a float toward zero but must reject the values int64
// cannot hold (NaN, +/-Inf, out of range) rather than returning garbage from a
// bare int64(f) cast - convert is canonical-only.
func TestToIntRejectsUnrepresentableFloats(t *testing.T) {
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 1e300, -1e300, 9223372036854775808.0} {
		if _, err := toIntFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.FloatVal(v)}); err == nil {
			t.Errorf("toInt(%g) should error, got nil", v)
		}
	}
	// In-range truncation still works.
	if r, err := toIntFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.FloatVal(3.9)}); err != nil || r.Int != 3 {
		t.Errorf("toInt(3.9) = %+v, err %v; want 3", r, err)
	}
}

// toFloat must reject the non-finite spellings strconv.ParseFloat accepts
// ("NaN", "Inf", "Infinity"): Jennifer's float model forbids those values.
func TestToFloatRejectsNonFinite(t *testing.T) {
	for _, s := range []string{"NaN", "nan", "Inf", "inf", "+Inf", "-Inf", "Infinity"} {
		if _, err := toFloatFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal(s)}); err == nil {
			t.Errorf("toFloat(%q) should error", s)
		}
	}
	// A normal float string still parses.
	if v, err := toFloatFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("3.5")}); err != nil || v.Float != 3.5 {
		t.Errorf("toFloat(\"3.5\") = %+v, err %v", v, err)
	}
}
