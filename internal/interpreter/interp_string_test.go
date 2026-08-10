// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"strings"
	"testing"
)

// TestInterpString covers cooked-string `{expr}` interpolation end to end: the
// value forms each slot kind renders, brace escapes, and that a raw string never
// interpolates.
func TestInterpString(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"var", `use io; def x as int init 5; io.printf("x={$x}");`, "x=5"},
		{"arith", `use io; def n as int init 3; io.printf("{$n + 1}");`, "4"},
		{"multi", `use io; def a as int init 1; def b as int init 2; io.printf("{$a}+{$b}={$a + $b}");`, "1+2=3"},
		{"float", `use io; def f as float init 1.5; io.printf("f={$f}");`, "f=1.5"},
		{"bool", `use io; def b as bool init true; io.printf("{$b}");`, "true"},
		{"null", `use io; def n as null; io.printf("{$n}");`, "null"},
		{"list", `use io; def xs as list of int init [1, 2, 3]; io.printf("{$xs}");`, "[1, 2, 3]"},
		{"map-slot", `use io; io.printf("{mlen()}"); func mlen() { def m as map of string to int init {"a": 1, "b": 2}; return len($m); }`, "2"},
		{"escape", `use io; io.printf("a\{b\}c");`, "a{b}c"},
		{"index", `use io; def xs as list of int init [7, 8]; io.printf("{$xs[1]}");`, "8"},
		{"raw-no-interp", `use io; def x as int init 5; io.printf('{$x}');`, "{$x}"},
		{"string-in-slot", `use io; use strings; io.printf("{strings.upper('a}b')}");`, "A}B"},
		// A qualified call and a namespaced constant inside a slot: these go through
		// the qualified-ref pre-stamping pass, which must descend into slots.
		// Exercises both the library-call and the const branch.
		{"qualified-call-slot", `use io; use strings; io.printf("{strings.repeat('ab', 3)}");`, "ababab"},
		{"qualified-const-slot", `use io; use math; io.printf("{math.PI}");`, "3.141592653589793"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := run(t, c.src)
			if err != nil {
				t.Fatalf("run error: %v", err)
			}
			if out != c.want {
				t.Errorf("output = %q, want %q", out, c.want)
			}
		})
	}
}

// TestInterpStringErrors covers the positioned rejections: an empty slot, an
// undefined variable inside a slot (a parse-time error, not a silent literal), and
// a bare unescaped `}`.
func TestInterpStringErrors(t *testing.T) {
	cases := []struct{ name, src, msg string }{
		{"empty-slot", `use io; io.printf("x {} y");`, "empty interpolation slot"},
		{"undef-var", `use io; io.printf("{$typo}");`, "undefined variable"},
		{"bare-close", `use io; io.printf("a } b");`, "unexpected `}`"},
		{"statement-in-slot", `use io; def x as int init 1; io.printf("{$x = 2}");`, "single expression"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := run(t, c.src)
			if err == nil {
				t.Fatalf("expected an error containing %q", c.msg)
			}
			if !strings.Contains(err.Error(), c.msg) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), c.msg)
			}
		})
	}
}
