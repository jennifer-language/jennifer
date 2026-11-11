// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package tomllib

import (
	"math"
	"testing"
)

// Floats encode in the shared form (plain digits between 1e-6 and 1e21) and
// decode back to the same float.
func TestEncodeFloatForm(t *testing.T) {
	cases := map[float64]string{
		1e6: "1000000.0", 123456789: "123456789.0", 1731300000.5: "1731300000.5",
		1e-7: "1e-7", 1e21: "1e+21", -2.5: "-2.5", math.Inf(1): "inf", math.Inf(-1): "-inf",
	}
	for f, want := range cases {
		got := formatTomlFloat(f)
		if got != want {
			t.Errorf("formatTomlFloat(%v) = %s, want %s", f, got, want)
			continue
		}
		v, err := decodeToml("a = " + got)
		if err != nil {
			t.Errorf("decode(a = %s): %v", got, err)
			continue
		}
		if back := v.Elem(0); back.TypeName() != "float" || back.FloatVal() != f {
			t.Errorf("decode(a = %s) = %v %s", got, back.FloatVal(), back.TypeName())
		}
	}
}
