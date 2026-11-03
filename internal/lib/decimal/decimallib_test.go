// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package decimallib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

func str(s string) Value { return interpreter.StringVal(s) }
func num(n int64) Value  { return interpreter.IntVal(n) }

func call(t *testing.T, fn interpreter.Builtin, args ...Value) Value {
	t.Helper()
	v, err := fn(interpreter.BuiltinCtx{}, args)
	if err != nil {
		t.Fatalf("call%v: %v", args, err)
	}
	return v
}

// kindOf returns the error kind (an *ErrorSignal's kind, or "runtime").
func kindOf(err error) string {
	if sig, ok := err.(*interpreter.ErrorSignal); ok {
		for _, f := range sig.Value.Fields {
			if f.Name == "kind" {
				return f.Value.Str
			}
		}
	}
	return "runtime"
}

func fails(t *testing.T, kind, want string, fn interpreter.Builtin, args ...Value) {
	t.Helper()
	_, err := fn(interpreter.BuiltinCtx{}, args)
	if err == nil {
		t.Fatalf("call%v: want a %s error", args, kind)
	}
	msg := err.Error()
	if sig, ok := err.(*interpreter.ErrorSignal); ok {
		for _, f := range sig.Value.Fields {
			if f.Name == "message" {
				msg = f.Value.Str
			}
		}
	}
	if kindOf(err) != kind || !strings.Contains(msg, want) {
		t.Fatalf("call%v: got %s %q, want %s containing %q", args, kindOf(err), msg, kind, want)
	}
}

func s(v Value) string { return v.Display() }

func TestParseAndDisplay(t *testing.T) {
	for in, want := range map[string]string{
		"0":        "0",
		"-0":       "0",
		"12.340":   "12.340",
		"-0.05":    "-0.05",
		"+7":       "7",
		".5":       "0.5",
		"5.":       "5",
		"1e3":      "1000",
		"1.5E-3":   "0.0015",
		"-2.50e1":  "-25.0",
		"00012.30": "12.30",
		"123456789012345678901234567890.123456789": "123456789012345678901234567890.123456789",
	} {
		if got := s(call(t, fromFn, str(in))); got != want {
			t.Errorf("from(%q) = %s, want %s", in, got, want)
		}
	}
	for _, bad := range []string{"", "-", ".", "1.2.3", "1e", "1e+", "abc", "1_000", " 1", "NaN", "Inf", "0x10", "1,5"} {
		fails(t, "decimal", "is not a decimal number", fromFn, str(bad))
	}
	fails(t, "limit", "more than", fromFn, str("1e999999999"))
	if got := s(call(t, fromFn, num(-42))); got != "-42" {
		t.Errorf("from(-42) = %s", got)
	}
	// a float goes in by its shortest form, not its binary expansion
	for f, want := range map[float64]string{0.1: "0.1", 2.5: "2.5", 1e21: "1000000000000000000000", -3.0: "-3"} {
		if got := s(call(t, fromFn, interpreter.FloatVal(f))); got != want {
			t.Errorf("from(%v) = %s, want %s", f, got, want)
		}
	}
}

func TestExactArithmetic(t *testing.T) {
	cases := []struct {
		fn   interpreter.Builtin
		a, b string
		want string
	}{
		{binary("add", add), "0.1", "0.2", "0.3"},
		{binary("add", add), "10.00", "0.005", "10.005"},
		{binary("sub", sub), "1", "0.999", "0.001"},
		{binary("sub", sub), "-1.5", "2.25", "-3.75"},
		{binary("mul", mul), "10.00", "1.2", "12.000"},
		{binary("mul", mul), "-0.5", "0.5", "-0.25"},
	}
	for _, c := range cases {
		if got := s(call(t, c.fn, str(c.a), str(c.b))); got != c.want {
			t.Errorf("%s op %s = %s, want %s", c.a, c.b, got, c.want)
		}
	}
	// operands may be ints, strings or Decimals; floats must go through from
	d := call(t, fromFn, str("2.5"))
	if got := s(call(t, binary("mul", mul), d, num(4))); got != "10.0" {
		t.Errorf("2.5 * 4 = %s", got)
	}
	fails(t, "runtime", "through decimal.from", binary("add", add), d, interpreter.FloatVal(1.5))
	fails(t, "runtime", "operand must be", binary("add", add), d, interpreter.BoolVal(true))
}

func TestDivision(t *testing.T) {
	divs := []struct {
		args []Value
		want string
	}{
		{[]Value{str("1"), str("4")}, "0.25"},
		{[]Value{str("10"), str("8")}, "1.25"},
		{[]Value{str("7.50"), str("2.5")}, "3.0"}, // keeps scale 2 - 1, as Java and Python do
		{[]Value{str("1"), str("0.002")}, "500"},
		{[]Value{str("-3"), str("-0.6")}, "5"},
		{[]Value{str("1"), str("-8")}, "-0.125"},
		{[]Value{str("0"), str("7")}, "0"},
		{[]Value{str("10"), str("3"), num(4)}, "3.3333"},
		{[]Value{str("20"), str("3"), num(2)}, "6.67"},
		{[]Value{str("-20"), str("3"), num(2)}, "-6.67"},
		{[]Value{str("2"), str("3"), num(0), str("down")}, "0"},
		{[]Value{str("1"), str("8"), num(2)}, "0.12"}, // 0.125 half-even
		{[]Value{str("1"), str("8"), num(2), str("half-up")}, "0.13"},
		{[]Value{str("1"), str("4"), num(4)}, "0.2500"},
	}
	for _, c := range divs {
		if got := s(call(t, divFn, c.args...)); got != c.want {
			t.Errorf("div%v = %s, want %s", c.args, got, c.want)
		}
	}
	fails(t, "decimal", "no finite decimal form", divFn, str("1"), str("3"))
	fails(t, "decimal", "division by zero", divFn, str("1"), str("0.00"))
	fails(t, "decimal", "division by zero", divFn, str("1"), str("0"), num(2))
	fails(t, "runtime", "unknown rounding mode", divFn, str("1"), str("3"), num(2), str("nearest"))
	fails(t, "runtime", "scale must be in", divFn, str("1"), str("3"), num(-1))
}

// Every mode against the classic table (Java's RoundingMode documentation),
// rounding to an integer.
func TestRoundingModes(t *testing.T) {
	inputs := []string{"5.5", "2.5", "1.6", "1.1", "1.0", "-1.0", "-1.1", "-1.6", "-2.5", "-5.5"}
	want := map[string][]string{
		"up":        {"6", "3", "2", "2", "1", "-1", "-2", "-2", "-3", "-6"},
		"down":      {"5", "2", "1", "1", "1", "-1", "-1", "-1", "-2", "-5"},
		"ceiling":   {"6", "3", "2", "2", "1", "-1", "-1", "-1", "-2", "-5"},
		"floor":     {"5", "2", "1", "1", "1", "-1", "-2", "-2", "-3", "-6"},
		"half-up":   {"6", "3", "2", "1", "1", "-1", "-1", "-2", "-3", "-6"},
		"half-down": {"5", "2", "2", "1", "1", "-1", "-1", "-2", "-2", "-5"},
		"half-even": {"6", "2", "2", "1", "1", "-1", "-1", "-2", "-2", "-6"},
	}
	for m, outs := range want {
		for i, in := range inputs {
			if got := s(call(t, roundFn, str(in), num(0), str(m))); got != outs[i] {
				t.Errorf("round(%s, 0, %s) = %s, want %s", in, m, got, outs[i])
			}
		}
	}
	// half-even is the default; a larger scale pads with zeros
	for _, c := range [][3]string{{"2.675", "2", "2.68"}, {"2.665", "2", "2.66"}, {"1.5", "3", "1.500"}, {"0.0049", "2", "0.00"}} {
		n := map[string]int64{"2": 2, "3": 3}[c[1]]
		if got := s(call(t, roundFn, str(c[0]), num(n))); got != c[2] {
			t.Errorf("round(%s, %s) = %s, want %s", c[0], c[1], got, c[2])
		}
	}
}

func TestEqualityAndCompare(t *testing.T) {
	a := call(t, fromFn, str("1.0"))
	b := call(t, fromFn, str("1.00"))
	if !a.Equal(b) {
		t.Error("1.0 == 1.00 should be true")
	}
	if s(b) != "1.00" {
		t.Errorf("the scale is kept for display: %s", s(b))
	}
	if a.Equal(call(t, fromFn, str("1.01"))) {
		t.Error("1.0 == 1.01 should be false")
	}
	for _, c := range []struct {
		a, b string
		want int64
	}{{"1.5", "1.50", 0}, {"-2", "1", -1}, {"0.1", "0.09", 1}, {"100", "1e2", 0}} {
		if got := call(t, compareFn, str(c.a), str(c.b)).Int; got != c.want {
			t.Errorf("compare(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestHelpers(t *testing.T) {
	if got := s(call(t, unary("normalize", func(d dec) (dec, error) { return normalize(d), nil }), str("12.3400"))); got != "12.34" {
		t.Errorf("normalize = %s", got)
	}
	if got := s(call(t, unary("normalize", func(d dec) (dec, error) { return normalize(d), nil }), str("100"))); got != "100" {
		t.Errorf("normalize keeps integer zeros: %s", got)
	}
	if got := s(call(t, powFn, str("2"), num(100))); got != "1267650600228229401496703205376" {
		t.Errorf("2^100 = %s", got)
	}
	if got := s(call(t, powFn, str("1.1"), num(2))); got != "1.21" {
		t.Errorf("1.1^2 = %s", got)
	}
	if got := s(call(t, powFn, str("-3"), num(0))); got != "1" {
		t.Errorf("x^0 = %s", got)
	}
	fails(t, "limit", "more than", powFn, str("10"), num(200000))
	fails(t, "limit", "more than", powFn, str("1.5"), num(100000))
	list := interpreter.ListVal(parser.Type{}, []Value{str("19.99"), num(5), call(t, fromFn, str("0.01"))})
	if got := s(call(t, sumFn, list)); got != "25.00" {
		t.Errorf("sum = %s", got)
	}
	if got := s(call(t, sumFn, interpreter.ListVal(parser.Type{}, nil))); got != "0" {
		t.Errorf("empty sum = %s", got)
	}
	if got := call(t, info("isInteger", func(d dec) Value { return interpreter.BoolVal(isInteger(d)) }), str("3.000")); !got.Bool {
		t.Error("3.000 is an integer")
	}
	if got := call(t, toIntFn, str("42.00")).Int; got != 42 {
		t.Errorf("toInt = %d", got)
	}
	fails(t, "decimal", "fractional part", toIntFn, str("1.5"))
	fails(t, "decimal", "does not fit an int", toIntFn, str("9223372036854775808"))
	if got := call(t, toFloatFn, str("0.1")).Float; got != 0.1 {
		t.Errorf("toFloat = %v", got)
	}
	fails(t, "decimal", "out of float range", toFloatFn, str("1e400"))
}
