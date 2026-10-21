// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package interpreter_test

import (
	"strings"
	"testing"
)

// The int / bool fast path (intfast.go) reads operands in place and gives up on
// anything it does not handle exactly. These cases sit on its edges: results it
// must compute with the general evaluator's semantics (floored `//` / `%`,
// short-circuit, int/float and int/string comparison), and every error it hands
// back to the general path, which must raise it with the same message at the
// same position as before the fast path existed.
func TestIntFastPathMatchesGeneralEvaluator(t *testing.T) {
	cases := []struct {
		name, src, out string
	}{
		{"floored division and modulo", `use io; def a as int init -7; def b as int init 2; def c as int init 3; def d as int init -3; def m as int init 7; io.printf("%d %d %d %d\n", $a // $b, $a % $c, $m % $d, $m // $d);`, "-4 2 -2 -3\n"},
		{"int against float and string", `use io; def i as int init 2; io.printf("%t %t %t %t\n", $i < 2.5, $i == 2.0, $i != 9007199254740993, $i == "2");`, "true true true false\n"},
		{"int and bool assignment in a loop", `use io; def i as int init 0; def flag as bool; while ($i < 5) { $i = $i + 2; $flag = $i >= 4 and not ($i == 5); } io.printf("%d %t\n", $i, $flag);`, "6 true\n"},
		{"global written from a function", `use io; def g as int init 0; def on as bool; func bump() { $g = $g + 1; $on = not $on; } bump(); bump(); bump(); io.printf("%d %t\n", $g, $on);`, "3 true\n"},
		{"list and bytes element reads", `use io; use convert; def xs as list of int init [5, 6, 7]; def ss as list of string init ["a"]; def fs as list of float init [1.5]; def b as bytes init convert.bytesFromString("AB", "utf-8"); io.printf("%d %d %t %t\n", $xs[1] + $xs[2], $b[1] - $b[0], $ss[0] == "a", $fs[0] > 1);`, "13 1 true true\n"},
		{"short circuit skips a bad right operand", `use io; def s as string init "x"; def n as int init 1; io.printf("%t %t\n", false and $s > 1, $n == 1 or $s > 1);`, "false true\n"},
		{"guarded index", `use io; def n as int init 3; def s as list of int init [1, 2]; def i as int init 5; if ($i < len($s) and $s[$i] == 1) { io.printf("bad\n"); } else { io.printf("ok %d\n", $n * -$n); }`, "ok -9\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := run(t, c.src)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if out != c.out {
				t.Fatalf("got %q, want %q", out, c.out)
			}
		})
	}

	errCases := []struct {
		name, src, want string
	}{
		{"overflow in a def", `def x as int init 9223372036854775807; def y as int init $x + 1;`, "1:58: integer overflow: 9223372036854775807 + 1"},
		{"overflow in an assignment", `def x as int init 9223372036854775807; $x = $x + 1;`, "1:45: integer overflow: 9223372036854775807 + 1"},
		{"overflow in a condition", `def x as int init 9223372036854775807; if ($x * 2 > 0) { }`, "1:44: integer overflow: 9223372036854775807 * 2"},
		{"overflow on the right of and", `def x as int init 9223372036854775807; def ok as bool init $x > 0 and $x + 1 > 0;`, "1:71: integer overflow: 9223372036854775807 + 1"},
		{"division by zero", `def a as int init 1; def z as int init 0; def y as int init $a // $z;`, "1:61: integer division by zero"},
		{"modulo by zero", `def a as int init 1; def z as int init 0; while ($a % $z == 0) { }`, "1:50: integer modulo by zero"},
		{"min int floor-divided by -1", `def a as int init -9223372036854775807 - 1; def m as int init -1; $a = $a // $m;`, "1:72: integer overflow: -9223372036854775808 // -1"},
		{"bytes index out of bounds", `use convert; def b as bytes init convert.bytesFromString("AB", "utf-8"); def i as int init 2; if ($b[$i] == 0) { }`, "1:101: bytes index 2 out of bounds (len 2)"},
		{"negative list index", `def xs as list of int init [1]; def i as int init -1; def y as int init $xs[$i] + 1;`, "1:76: list index -1 out of bounds (len 1)"},
		{"int operand of and", `def n as int init 1; if (true and $n) { }`, "1:35: right operand of `and` must be bool, got int"},
		{"int into a float variable", `def f as float init 0.0; def i as int init 1; $f = $i + 1;`, "1:47: cannot assign int to float variable \"f\""},
		{"int into a bool variable", `def b as bool; def i as int init 1; $b = $i + 1;`, "1:37: cannot assign int to bool variable \"b\""},
		{"int loop condition", `def i as int init 1; while ($i + 1) { }`, "1:29: `while` condition must be bool, got int"},
		{"not over an int", `def i as int init 1; if (not ($i == 1) or not $i) { }`, "1:43: unary `not` requires bool, got int"},
	}
	for _, c := range errCases {
		t.Run(c.name, func(t *testing.T) {
			_, err := run(t, c.src)
			if err == nil {
				t.Fatalf("want error %q, got none", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %q, want it to contain %q", err.Error(), c.want)
			}
		})
	}
}
