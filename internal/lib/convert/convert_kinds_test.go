// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package convert

import (
	"math"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dataerr"
	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// M29.21: a string that does not parse is kind "convert"; converting a program
// value that has no result, and every misuse, stays "runtime" ("").
func TestErrorKinds(t *testing.T) {
	str := interpreter.StringVal
	i := interpreter.IntVal
	cases := []struct {
		name string
		fn   interpreter.Builtin
		args []interpreter.Value
		kind string
	}{
		{"toInt text", toIntFn, []interpreter.Value{str("abc")}, "convert"},
		{"toInt prefix", toIntFn, []interpreter.Value{str("0x1f")}, "convert"},
		{"toInt overflow", toIntFn, []interpreter.Value{str("99999999999999999999")}, "convert"},
		{"toInt radix digit", toIntFn, []interpreter.Value{str("12"), i(2)}, "convert"},
		{"toInt radix prefix", toIntFn, []interpreter.Value{str("0xff"), i(16)}, "convert"},
		{"toFloat text", toFloatFn, []interpreter.Value{str("1.2.3")}, "convert"},
		{"toFloat NaN text", toFloatFn, []interpreter.Value{str("NaN")}, "convert"},
		{"toFloat overflow", toFloatFn, []interpreter.Value{str("1e999")}, "convert"},
		{"toBool text", toBoolFn, []interpreter.Value{str("yes")}, "convert"},
		{"toInt NaN float", toIntFn, []interpreter.Value{interpreter.FloatVal(math.NaN())}, ""},
		{"toInt null", toIntFn, []interpreter.Value{interpreter.Null()}, ""},
		{"toInt bad radix", toIntFn, []interpreter.Value{str("12"), i(3)}, ""},
		{"toInt radix on int", toIntFn, []interpreter.Value{i(1), i(16)}, ""},
		{"toBool int", toBoolFn, []interpreter.Value{i(2)}, ""},
		{"toBool float", toBoolFn, []interpreter.Value{interpreter.FloatVal(0.5)}, ""},
		{"toCodepoint two runes", toCodepointFn, []interpreter.Value{str("ab")}, ""},
		{"arity", toFloatFn, nil, ""},
	}
	for _, c := range cases {
		_, err := c.fn(interpreter.BuiltinCtx{}, c.args)
		if err == nil {
			t.Errorf("%s: no error", c.name)
			continue
		}
		if got := dataerr.KindOf(err); got != c.kind {
			t.Errorf("%s: %v: kind %q, want %q", c.name, err, got, c.kind)
		}
	}
}
