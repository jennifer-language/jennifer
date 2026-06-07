// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package interpreter_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mplx/jennifer-lang/internal/interpreter"
	"github.com/mplx/jennifer-lang/internal/lib/convert"
	"github.com/mplx/jennifer-lang/internal/lib/io"
	"github.com/mplx/jennifer-lang/internal/lib/math"
	"github.com/mplx/jennifer-lang/internal/lib/core"
	"github.com/mplx/jennifer-lang/internal/lib/strings"
	"github.com/mplx/jennifer-lang/internal/parser"
)

// run lexes/parses/installs the io library/runs a program and returns captured stdout.
//
// Convenience: if the source defines `app()` and has no top-level statements
// other than imports/defs, we append `app();` automatically. This keeps the
// legacy test style (`func app() { ... }` only) working after the app()
// requirement was dropped; new tests can simply put statements at top level.
func run(t *testing.T, src string) (string, error) {
	t.Helper()
	if strings.Contains(src, "func app(") {
		src = src + "\napp();"
	}
	prog, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	convert.Install(in)
	mathlib.Install(in)
	stringslib.Install(in)
	corelib.Install(in)
	if err := in.Run(prog); err != nil {
		return buf.String(), err
	}
	return buf.String(), nil
}

func TestHelloProgramPrints42(t *testing.T) {
	out, err := run(t, `
use io;
func app() {
    def x as int init 21;
    printf($x + $x);
}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "42" {
		t.Errorf("got %q, want %q", out, "42")
	}
}

func TestStringLiteralPrints(t *testing.T) {
	out, err := run(t, `
use io;
func app() {
    printf("hello, jennifer\n");
}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "hello, jennifer\n" {
		t.Errorf("got %q", out)
	}
}

func TestArithmeticPrecedence(t *testing.T) {
	out, err := run(t, `
use io;
func app() {
    def r as int init 2 + 3 * 4;
    printf($r);
}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "14" {
		t.Errorf("got %q, want %q", out, "14")
	}
}

func TestDivisionAndModulo(t *testing.T) {
	out, err := run(t, `
use io;
func app() {
    def a as int init 17 div 5;
    def b as int init 17 % 5;
    def c as float init 17 / 5;
    printf($a);
    printf(" ");
    printf($b);
    printf(" ");
    printf($c);
}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "3 2 3.4" {
		t.Errorf("got %q, want %q", out, "3 2 3.4")
	}
}

func TestEmptyProgramRunsCleanly(t *testing.T) {
	// app() is no longer required. An empty program (or one with only imports
	// and method defs that are never called) is valid and produces no output.
	out, err := run(t, `use io;`)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if out != "" {
		t.Errorf("expected no output, got %q", out)
	}
}

func TestTopLevelStatementsRun(t *testing.T) {
	// Bare top-level form - no `app()` wrapper.
	out, err := run(t, `
use io;
def x as int init 21;
printf($x + $x);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "42" {
		t.Errorf("got %q, want %q", out, "42")
	}
}

func TestMethodSeesGlobals(t *testing.T) {
	out, err := run(t, `
use io;
def greeting as string init "hello";
func show() { printf($greeting); }
show();
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "hello" {
		t.Errorf("got %q", out)
	}
}

func TestMethodCannotShadowGlobal(t *testing.T) {
	_, err := run(t, `
use io;
def x as int init 1;
func f() { def x as int init 2; }
f();
`)
	if err == nil || !strings.Contains(err.Error(), "already defined") {
		t.Errorf("expected shadowing error, got %v", err)
	}
}

func TestErrorOnPrintfWithoutImport(t *testing.T) {
	_, err := run(t, `func app() { printf(1); }`)
	if err == nil || !strings.Contains(err.Error(), "use io") {
		t.Errorf("expected use-io error, got %v", err)
	}
}

func TestErrorOnDivisionByZero(t *testing.T) {
	_, err := run(t, `
use io;
func app() { def x as int init 1 / 0; }`)
	if err == nil || !strings.Contains(err.Error(), "division by zero") {
		t.Errorf("expected division-by-zero error, got %v", err)
	}
}

func TestErrorOnTypeMismatch(t *testing.T) {
	_, err := run(t, `
use io;
func app() { def x as int init "nope"; }`)
	if err == nil || !strings.Contains(err.Error(), "cannot initialize int") {
		t.Errorf("expected type-mismatch error, got %v", err)
	}
}

func TestErrorOnUndefinedVar(t *testing.T) {
	_, err := run(t, `
use io;
func app() { printf($missing); }`)
	if err == nil || !strings.Contains(err.Error(), `undefined variable "missing"`) {
		t.Errorf("expected undefined-var error, got %v", err)
	}
}

func TestErrorOnUnknownFunction(t *testing.T) {
	_, err := run(t, `
use io;
func app() { nope(1); }`)
	if err == nil || !strings.Contains(err.Error(), "unknown function") {
		t.Errorf("expected unknown-function error, got %v", err)
	}
}

func TestErrorOnDuplicateMethod(t *testing.T) {
	_, err := run(t, `
func app() {}
func app() {}`)
	if err == nil || !strings.Contains(err.Error(), "defined more than once") {
		t.Errorf("expected duplicate-method error, got %v", err)
	}
}

func TestUserMethodCannotShadowIOBuiltin(t *testing.T) {
	// With `use io;`, defining `func printf()` is a shadowing error.
	_, err := run(t, `
use io;
func printf() {}
printf();
`)
	if err == nil || !strings.Contains(err.Error(), "shadows a builtin from `io`") {
		t.Errorf("expected shadowing error, got %v", err)
	}
}

func TestUnknownLibraryErrors(t *testing.T) {
	_, err := run(t, `use blub; func app() {}`)
	if err == nil || !strings.Contains(err.Error(), `unknown library "blub"`) {
		t.Errorf("expected unknown-library error, got %v", err)
	}
}

func TestUnknownLibraryErrorListsAvailable(t *testing.T) {
	_, err := run(t, `use blub; func app() {}`)
	if err == nil || !strings.Contains(err.Error(), "available: convert, io, math, strings") {
		t.Errorf("expected error to list available libraries, got %v", err)
	}
}

func TestReturnValue(t *testing.T) {
	out, err := run(t, `
use io;
func answer() { return 42; }
printf(answer());
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "42" {
		t.Errorf("got %q, want %q", out, "42")
	}
}

func TestReturnBare(t *testing.T) {
	out, err := run(t, `
use io;
func nothing() { return; }
printf(nothing());
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "null" {
		t.Errorf("got %q, want %q", out, "null")
	}
}

func TestReturnEndsMethodEarly(t *testing.T) {
	out, err := run(t, `
use io;
func early() {
    printf("a");
    return 1;
    printf("b");
}
printf(early());
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "a1" {
		t.Errorf("got %q, want %q", out, "a1")
	}
}

func TestReturnInsideNestedBlock(t *testing.T) {
	out, err := run(t, `
use io;
func find() {
    for (def i as int init 0; $i < 10; $i = $i + 1) {
        if ($i == 3) {
            return $i;
        }
    }
    return 99;
}
printf(find());
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "3" {
		t.Errorf("got %q, want %q", out, "3")
	}
}

// ---- M3 parameters ----

func TestParamsAddTwoInts(t *testing.T) {
	out, err := run(t, `
use io;
func add(a as int, b as int) { return $a + $b; }
printf(add(3, 4));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "7" {
		t.Errorf("got %q", out)
	}
}

func TestParamTypeMismatch(t *testing.T) {
	_, err := run(t, `
use io;
func add(a as int, b as int) { return $a + $b; }
add(3, "four");
`)
	if err == nil || !strings.Contains(err.Error(), "argument 2") {
		t.Errorf("expected per-arg type error, got %v", err)
	}
}

func TestParamArityMismatch(t *testing.T) {
	_, err := run(t, `
use io;
func add(a as int, b as int) { return $a + $b; }
add(3);
`)
	if err == nil || !strings.Contains(err.Error(), "takes 2") {
		t.Errorf("expected arity error, got %v", err)
	}
}

func TestRecursionFactorial(t *testing.T) {
	out, err := run(t, `
use io;
func fact(n as int) {
    if ($n == 0) { return 1; }
    return $n * fact($n - 1);
}
printf(fact(7));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "5040" {
		t.Errorf("got %q, want 5040", out)
	}
}

func TestParamSeesGlobalsThroughChain(t *testing.T) {
	// Params bind in the call frame; globals visible via the parent chain.
	out, err := run(t, `
use io;
def greeting as string init "hi ";
func greet(name as string) {
    printf($greeting + $name);
}
greet("Jennifer");
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "hi Jennifer" {
		t.Errorf("got %q", out)
	}
}

func TestConstReferenceBare(t *testing.T) {
	out, err := run(t, `
use io;
def const MAX as int init 100;
printf("%d", MAX);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "100" {
		t.Errorf("got %q, want %q", out, "100")
	}
}

// TestConstNameWithUnderscore exercises the relaxed constant naming rule:
// constants may carry interior `_` separators like MAX_RETRIES. The
// lexer keeps the name as a single IDENT and the interpreter treats it
// like any other constant.
func TestConstNameWithUnderscore(t *testing.T) {
	out, err := run(t, `
use io;
def const MAX_RETRIES as int init 3;
def const HTTP_OK as int init 200;
printf("%d %d", MAX_RETRIES, HTTP_OK);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "3 200" {
		t.Errorf("got %q, want %q", out, "3 200")
	}
}

func TestConstInExpression(t *testing.T) {
	out, err := run(t, `
use io;
def const MAX as int init 10;
def y as int init MAX + 5;
printf("%d", $y);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "15" {
		t.Errorf("got %q, want %q", out, "15")
	}
}

func TestBareVariableRefErrorsHelpfully(t *testing.T) {
	_, err := run(t, `
use io;
def x as int init 5;
printf("%d", x);
`)
	if err == nil || !strings.Contains(err.Error(), "use `$x`") {
		t.Errorf("expected $-prefix hint, got %v", err)
	}
}

func TestBareUndefinedNameError(t *testing.T) {
	_, err := run(t, `
use io;
printf("%d", NOPE);
`)
	if err == nil || !strings.Contains(err.Error(), "undefined name") {
		t.Errorf("expected undefined-name error, got %v", err)
	}
}

func TestParamRejectsDollarAtDefSite(t *testing.T) {
	_, err := run(t, `
use io;
func bad($x as int) { return $x; }
`)
	if err == nil || !strings.Contains(err.Error(), "no `$`") {
		t.Errorf("expected $-at-param error, got %v", err)
	}
}

func TestUserMethodCanReuseBuiltinNameWithoutImportingLib(t *testing.T) {
	// Without `use io;`, the name is free - the user's printf is the only one.
	out, err := run(t, `
func printf() {}
printf();
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "" {
		t.Errorf("got %q, want empty (user printf is a no-op)", out)
	}
}

// ---- M2 tests ----

func TestM2FloatArithmetic(t *testing.T) {
	out, err := run(t, `
use io;
func app() {
    def a as float init 1.5;
    def b as float init 2.5;
    printf($a + $b);
}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "4.0" {
		t.Errorf("got %q, want %q", out, "4.0")
	}
}

func TestM2IntFloatPromotion(t *testing.T) {
	out, err := run(t, `
use io;
func app() {
    def a as int init 3;
    def b as float init 0.5;
    def r as float init $a + $b;
    printf($r);
}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "3.5" {
		t.Errorf("got %q, want %q", out, "3.5")
	}
}

func TestM2StringConcatenation(t *testing.T) {
	out, err := run(t, `
use io;
func app() {
    def a as string init "hello, ";
    def b as string init "world";
    printf($a + $b);
}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "hello, world" {
		t.Errorf("got %q", out)
	}
}

func TestM2BoolLiterals(t *testing.T) {
	out, err := run(t, `
use io;
func app() {
    def t as bool init true;
    def f as bool init false;
    printf($t);
    printf(" ");
    printf($f);
}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "true false" {
		t.Errorf("got %q", out)
	}
}

func TestM2NullLiteral(t *testing.T) {
	out, err := run(t, `
use io;
func app() {
    def n as null init null;
    printf($n);
}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "null" {
		t.Errorf("got %q", out)
	}
}

func TestM2UninitializedZeroValues(t *testing.T) {
	out, err := run(t, `
use io;
func app() {
    def i as int;
    def f as float;
    def s as string;
    def b as bool;
    printf($i);
    printf(" ");
    printf($f);
    printf(" ");
    printf($s);
    printf(" ");
    printf($b);
}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "0 0.0  false" {
		t.Errorf("got %q, want %q", out, "0 0.0  false")
	}
}

// ---- M4 logical operators + unary minus ----

func TestLogicalNotAndOr(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{"not true", "false"},
		{"not false", "true"},
		{"not (1 == 2)", "true"},
		{"true and true", "true"},
		{"true and false", "false"},
		{"false and true", "false"},
		{"false or true", "true"},
		{"true or false", "true"},
		{"false or false", "false"},
		{"not not true", "true"},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
def r as bool init `+c.expr+`;
printf("%t", $r);
`)
		if err != nil {
			t.Errorf("%s: err %v", c.expr, err)
			continue
		}
		if out != c.want {
			t.Errorf("%s: got %q, want %q", c.expr, out, c.want)
		}
	}
}

func TestLogicalPrecedence(t *testing.T) {
	// `not` binds less tightly than comparison.
	// `and` binds tighter than `or`.
	cases := []struct {
		expr string
		want string
	}{
		// not (1 == 2) -> not false -> true
		{"not 1 == 2", "true"},
		// (1 > 0) and (2 > 1) -> true and true -> true
		{"1 > 0 and 2 > 1", "true"},
		// (true and false) or true -> false or true -> true
		{"true and false or true", "true"},
		// true or (false and false) -> true or false -> true
		{"true or false and false", "true"},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
def r as bool init `+c.expr+`;
printf("%t", $r);
`)
		if err != nil {
			t.Errorf("%s: err %v", c.expr, err)
			continue
		}
		if out != c.want {
			t.Errorf("%s: got %q, want %q", c.expr, out, c.want)
		}
	}
}

func TestLogicalShortCircuit(t *testing.T) {
	// rhs of `and` is not evaluated when lhs is false; rhs of `or` is not
	// evaluated when lhs is true. We prove it by having the rhs call a method
	// that prints a side effect.
	out, err := run(t, `
use io;
func boom() {
    printf("BOOM");
    return true;
}
def a as bool init false and boom();
def b as bool init true or boom();
printf("|%t|%t", $a, $b);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "|false|true" {
		t.Errorf("got %q, want %q", out, "|false|true")
	}
}

func TestLogicalTypeErrors(t *testing.T) {
	cases := []struct {
		expr string
		want string // substring of error
	}{
		{"not 1", "`not` requires bool"},
		{"true and 1", "`and`"},
		{"1 or false", "`or`"},
	}
	for _, c := range cases {
		_, err := run(t, `
use io;
def r as bool init `+c.expr+`;
`)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want substring %q", c.expr, err, c.want)
		}
	}
}

func TestUnaryMinus(t *testing.T) {
	out, err := run(t, `
use io;
def i as int init -5;
def f as float init -0.25;
def doubleNeg as int init - -7;
def x as int init 10;
def neg as int init -$x;
printf("%d|%f|%d|%d", $i, $f, $doubleNeg, $neg);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "-5|-0.25|7|-10" {
		t.Errorf("got %q", out)
	}
}

func TestUnaryMinusPrecedence(t *testing.T) {
	// -3 + 10 -> (-3) + 10 -> 7
	// -3 * 2 -> (-3) * 2 -> -6
	out, err := run(t, `
use io;
printf("%d|%d", -3 + 10, -3 * 2);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "7|-6" {
		t.Errorf("got %q, want %q", out, "7|-6")
	}
}

func TestUnaryMinusTypeError(t *testing.T) {
	_, err := run(t, `
use io;
def s as string init "x";
def r as int init -$s;
`)
	if err == nil || !strings.Contains(err.Error(), "`-` requires int or float") {
		t.Errorf("got %v, want type-error mentioning unary -", err)
	}
}

// ---- M4 division semantics ----

func TestSlashAlwaysReturnsFloat(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{"5 / 2", "2.5"},
		{"5.0 / 2.0", "2.5"},
		{"5 / 2.0", "2.5"},
		{"4 / 2", "2.0"}, // even result still prints as float
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
def r as float init `+c.expr+`;
printf("%f", $r);
`)
		if err != nil {
			t.Errorf("%s: err %v", c.expr, err)
			continue
		}
		if out != c.want {
			t.Errorf("%s: got %q, want %q", c.expr, out, c.want)
		}
	}
}

func TestSlashIntoIntErrors(t *testing.T) {
	_, err := run(t, `
use io;
def x as int init 5 / 2;
`)
	if err == nil || !strings.Contains(err.Error(), "cannot initialize int") {
		t.Errorf("expected type-mismatch, got %v", err)
	}
}

func TestDivKeyword(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{"5 div 2", "2"},
		{"6 div 2", "3"},
		// Python-style floor division: -5 div 2 = -3 (toward -inf)
		// We don't have unary minus on literals via prefix in numeric ctx
		// without space, but `0 - 5` works.
		{"(0 - 5) div 2", "-3"},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
def r as int init `+c.expr+`;
printf("%d", $r);
`)
		if err != nil {
			t.Errorf("%s: err %v", c.expr, err)
			continue
		}
		if out != c.want {
			t.Errorf("%s: got %q, want %q", c.expr, out, c.want)
		}
	}
}

func TestDivOnFloatsReturnsFloatFloor(t *testing.T) {
	out, err := run(t, `
use io;
def r as float init 5.7 div 2.0;
printf("%f", $r);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "2.0" {
		t.Errorf("got %q, want %q", out, "2.0")
	}
}

// ---- M4 float display ----

func TestFloatDisplayAlwaysHasDot(t *testing.T) {
	// 5.0 should print as "5.0", not "5".
	out, err := run(t, `
use io;
def x as float init 5.0;
printf("%f", $x);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "5.0" {
		t.Errorf("got %q, want %q", out, "5.0")
	}
}

// ---- M4 convert library ----

func TestConvertInt(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{`int("42")`, "42"},
		{`int(3.7)`, "3"}, // truncate
		{`int(true)`, "1"},
		{`int(false)`, "0"},
		{`int(99)`, "99"}, // identity
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use convert;
def r as int init `+c.expr+`;
printf("%d", $r);
`)
		if err != nil {
			t.Errorf("%s: err %v", c.expr, err)
			continue
		}
		if out != c.want {
			t.Errorf("%s: got %q, want %q", c.expr, out, c.want)
		}
	}
}

func TestConvertIntErrors(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{`int("abc")`, "not a valid integer"},
		{`int(null)`, "cannot convert null"},
	}
	for _, c := range cases {
		_, err := run(t, `
use io;
use convert;
def r as int init `+c.expr+`;
`)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want substring %q", c.expr, err, c.want)
		}
	}
}

func TestConvertFloat(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{`float(5)`, "5.0"},
		{`float("3.14")`, "3.14"},
		{`float(true)`, "1.0"},
		{`float(false)`, "0.0"},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use convert;
def r as float init `+c.expr+`;
printf("%f", $r);
`)
		if err != nil {
			t.Errorf("%s: err %v", c.expr, err)
			continue
		}
		if out != c.want {
			t.Errorf("%s: got %q, want %q", c.expr, out, c.want)
		}
	}
}

func TestConvertString(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{`string(42)`, "42"},
		{`string(3.14)`, "3.14"},
		{`string(true)`, "true"},
		{`string(null)`, "null"},
		{`string("hi")`, "hi"},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use convert;
def r as string init `+c.expr+`;
printf("%s", $r);
`)
		if err != nil {
			t.Errorf("%s: err %v", c.expr, err)
			continue
		}
		if out != c.want {
			t.Errorf("%s: got %q, want %q", c.expr, out, c.want)
		}
	}
}

func TestConvertBool(t *testing.T) {
	// Canonical-only: 0/1 for int, 0.0/1.0 for float, "true"/"false" for string.
	cases := []struct {
		expr string
		want string
	}{
		{`bool("true")`, "true"},
		{`bool("false")`, "false"},
		{`bool(0)`, "false"},
		{`bool(1)`, "true"},
		{`bool(0.0)`, "false"},
		{`bool(1.0)`, "true"},
		{`bool(true)`, "true"},   // identity
		{`bool(false)`, "false"}, // identity
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use convert;
def r as bool init `+c.expr+`;
printf("%t", $r);
`)
		if err != nil {
			t.Errorf("%s: err %v", c.expr, err)
			continue
		}
		if out != c.want {
			t.Errorf("%s: got %q, want %q", c.expr, out, c.want)
		}
	}
}

func TestConvertBoolErrors(t *testing.T) {
	// Non-canonical values for each kind should error, not silently coerce.
	cases := []struct {
		expr string
		want string // substring of error
	}{
		{`bool("maybe")`, `only "true" or "false"`},
		{`bool(5)`, "only 0 and 1"},
		{`bool(0 - 1)`, "only 0 and 1"}, // -1 must error too
		{`bool(123)`, "only 0 and 1"},
		{`bool(1.5)`, "only 0.0 and 1.0"},
		{`bool(2.0)`, "only 0.0 and 1.0"},
		{`bool(null)`, "cannot convert null"},
	}
	for _, c := range cases {
		_, err := run(t, `
use io;
use convert;
def r as bool init `+c.expr+`;
`)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want substring %q", c.expr, err, c.want)
		}
	}
}

func TestTypeof(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{`typeOf(5)`, "int"},
		{`typeOf(3.14)`, "float"},
		{`typeOf("hi")`, "string"},
		{`typeOf(true)`, "bool"},
		{`typeOf(null)`, "null"},
		{`typeOf(5 / 2)`, "float"},
		{`typeOf(5 div 2)`, "int"},
		{`typeOf(2.5 * 2)`, "float"},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use convert;
def r as string init `+c.expr+`;
printf("%s", $r);
`)
		if err != nil {
			t.Errorf("%s: err %v", c.expr, err)
			continue
		}
		if out != c.want {
			t.Errorf("%s: got %q, want %q", c.expr, out, c.want)
		}
	}
}

func TestConvertRequiresUse(t *testing.T) {
	_, err := run(t, `
use io;
def r as int init int("42");
`)
	if err == nil || !strings.Contains(err.Error(), "use convert") {
		t.Errorf("expected use-convert hint, got %v", err)
	}
}

func TestTypeNameAsBareReferenceErrors(t *testing.T) {
	// `int` alone (not followed by `(`) in expression position should error.
	_, err := run(t, `
use io;
use convert;
def r as int init int;
`)
	if err == nil || !strings.Contains(err.Error(), "called as a conversion") {
		t.Errorf("expected hint, got %v", err)
	}
}

// ---- M4 math library ----

func TestMathAbs(t *testing.T) {
	cases := []struct{ expr, want string }{
		{"abs(5)", "5"},
		{"abs(0 - 5)", "5"},
		{"abs(0)", "0"},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use math;
def r as int init `+c.expr+`;
printf("%d", $r);
`)
		if err != nil {
			t.Errorf("%s: %v", c.expr, err)
			continue
		}
		if out != c.want {
			t.Errorf("%s: got %q, want %q", c.expr, out, c.want)
		}
	}
}

func TestMathAbsFloat(t *testing.T) {
	out, err := run(t, `
use io;
use math;
def r as float init abs(-3.14);
printf("%f", $r);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "3.14" {
		t.Errorf("got %q", out)
	}
}

func TestMathMinMax(t *testing.T) {
	cases := []struct{ expr, want, typ string }{
		{"min(3, 7)", "3", "int"},
		{"min(7, 3)", "3", "int"},
		{"max(3, 7)", "7", "int"},
		{"max(7, 3)", "7", "int"},
		{"min(3, 2.5)", "2.5", "float"}, // mixed -> float
		{"max(3, 2.5)", "3.0", "float"}, // mixed -> float (int promoted)
		{"min(1.0, 2.0)", "1.0", "float"},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use math;
def r as `+c.typ+` init `+c.expr+`;
printf("%v", $r);
`)
		if err != nil {
			t.Errorf("%s: %v", c.expr, err)
			continue
		}
		if out != c.want {
			t.Errorf("%s: got %q, want %q", c.expr, out, c.want)
		}
	}
}

func TestMathSqrt(t *testing.T) {
	out, err := run(t, `
use io;
use math;
def r as float init sqrt(16);
printf("%f", $r);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "4.0" {
		t.Errorf("got %q, want %q", out, "4.0")
	}
}

func TestMathSqrtNegativeErrors(t *testing.T) {
	_, err := run(t, `
use io;
use math;
def r as float init sqrt(0 - 1);
`)
	if err == nil || !strings.Contains(err.Error(), "undefined for negative") {
		t.Errorf("got %v", err)
	}
}

func TestMathPow(t *testing.T) {
	out, err := run(t, `
use io;
use math;
def a as float init pow(2, 10);
def b as float init pow(2, 0.5);
printf("%f|%f", $a, $b);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// pow(2,10) = 1024.0; pow(2,0.5) = sqrt(2)
	if !strings.HasPrefix(out, "1024.0|1.41") {
		t.Errorf("got %q", out)
	}
}

func TestMathFloorCeilRound(t *testing.T) {
	cases := []struct{ expr, want string }{
		{"floor(3.7)", "3"},
		{"floor(3.2)", "3"},
		{"floor(0 - 3.2)", "-4"}, // toward -inf
		{"ceil(3.2)", "4"},
		{"ceil(3.7)", "4"},
		{"ceil(0 - 3.7)", "-3"},
		{"round(2.5)", "3"}, // half away from zero
		{"round(2.4)", "2"},
		{"round(0 - 2.5)", "-3"},
		{"floor(5)", "5"}, // int passes through
		{"ceil(5)", "5"},
		{"round(5)", "5"},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use math;
def r as int init `+c.expr+`;
printf("%d", $r);
`)
		if err != nil {
			t.Errorf("%s: %v", c.expr, err)
			continue
		}
		if out != c.want {
			t.Errorf("%s: got %q, want %q", c.expr, out, c.want)
		}
	}
}

func TestMathConstants(t *testing.T) {
	out, err := run(t, `
use io;
use math;
def pi as float init PI;
def e as float init E;
printf("%f|%f", $pi, $e);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.HasPrefix(out, "3.14159") || !strings.Contains(out, "|2.71828") {
		t.Errorf("got %q", out)
	}
}

func TestMathConstantRequiresUse(t *testing.T) {
	_, err := run(t, `
use io;
def r as float init PI;
`)
	if err == nil || !strings.Contains(err.Error(), "use math") {
		t.Errorf("got %v", err)
	}
}

func TestMathArityErrors(t *testing.T) {
	cases := []struct{ expr, want string }{
		{"abs()", "expects 1 argument"},
		{"abs(1, 2)", "expects 1 argument"},
		{"min(1)", "expects 2 arguments"},
		{"pow(1)", "expects 2 arguments"},
	}
	for _, c := range cases {
		_, err := run(t, `
use io;
use math;
def r as int init `+c.expr+`;
`)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want substring %q", c.expr, err, c.want)
		}
	}
}

// ---- M4 strings library ----

func TestStringsLen(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`len("")`, "0"},
		{`len("hello")`, "5"},
		{`len("héllo")`, "5"}, // rune count, not bytes
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use strings;
def r as int init `+c.expr+`;
printf("%d", $r);
`)
		if err != nil {
			t.Errorf("%s: %v", c.expr, err)
			continue
		}
		if out != c.want {
			t.Errorf("%s: got %q, want %q", c.expr, out, c.want)
		}
	}
}

func TestStringsCaseConversion(t *testing.T) {
	out, err := run(t, `
use io;
use strings;
def a as string init upper("hello");
def b as string init lower("HELLO");
printf("%s|%s", $a, $b);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "HELLO|hello" {
		t.Errorf("got %q", out)
	}
}

func TestStringsSearchPredicates(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`contains("hello world", "world")`, "true"},
		{`contains("hello", "z")`, "false"},
		{`startsWith("hello", "he")`, "true"},
		{`startsWith("hello", "lo")`, "false"},
		{`endsWith("hello", "lo")`, "true"},
		{`endsWith("hello", "he")`, "false"},
		{`contains("", "")`, "true"}, // empty is always contained
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use strings;
def r as bool init `+c.expr+`;
printf("%t", $r);
`)
		if err != nil {
			t.Errorf("%s: %v", c.expr, err)
			continue
		}
		if out != c.want {
			t.Errorf("%s: got %q, want %q", c.expr, out, c.want)
		}
	}
}

func TestStringsIndexOf(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`indexOf("hello", "l")`, "2"},
		{`indexOf("hello", "z")`, "-1"},
		{`indexOf("hello", "")`, "0"},
		{`indexOf("héllo", "l")`, "2"}, // rune index, not byte (é is 2 bytes)
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use strings;
def r as int init `+c.expr+`;
printf("%d", $r);
`)
		if err != nil {
			t.Errorf("%s: %v", c.expr, err)
			continue
		}
		if out != c.want {
			t.Errorf("%s: got %q, want %q", c.expr, out, c.want)
		}
	}
}

func TestStringsTrim(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`trim("   hello   ")`, "hello"},
		{`trim("\t\nhello\n")`, "hello"},
		{`trim("nothing")`, "nothing"},
		{`trimLeft("   hello   ")`, "hello   "},
		{`trimRight("   hello   ")`, "   hello"},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use strings;
def r as string init `+c.expr+`;
printf("[%s]", $r);
`)
		if err != nil {
			t.Errorf("%s: %v", c.expr, err)
			continue
		}
		if out != "["+c.want+"]" {
			t.Errorf("%s: got %q, want %q", c.expr, out, "["+c.want+"]")
		}
	}
}

func TestStringsReplace(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`replace("hello world", "world", "Jennifer")`, "hello Jennifer"},
		{`replace("a-b-c", "-", "/")`, "a/b/c"}, // replace all
		{`replace("xyz", "q", "?")`, "xyz"},     // no occurrence -> unchanged
		{`replace("", "x", "y")`, ""},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use strings;
def r as string init `+c.expr+`;
printf("%s", $r);
`)
		if err != nil {
			t.Errorf("%s: %v", c.expr, err)
			continue
		}
		if out != c.want {
			t.Errorf("%s: got %q, want %q", c.expr, out, c.want)
		}
	}
}

func TestStringsRepeat(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`repeat("ab", 3)`, "ababab"},
		{`repeat("x", 0)`, ""},
		{`repeat("", 5)`, ""},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use strings;
def r as string init `+c.expr+`;
printf("[%s]", $r);
`)
		if err != nil {
			t.Errorf("%s: %v", c.expr, err)
			continue
		}
		if out != "["+c.want+"]" {
			t.Errorf("%s: got %q, want %q", c.expr, out, "["+c.want+"]")
		}
	}
}

func TestStringsRepeatNegativeErrors(t *testing.T) {
	_, err := run(t, `
use io;
use strings;
def r as string init repeat("x", 0 - 1);
`)
	if err == nil || !strings.Contains(err.Error(), "negative count") {
		t.Errorf("got %v", err)
	}
}

func TestStringsSubstring(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`substring("hello", 0, 5)`, "hello"},
		{`substring("hello", 1, 4)`, "ell"},
		{`substring("hello", 0, 0)`, ""},
		{`substring("hello", 5, 5)`, ""},   // at end
		{`substring("héllo", 0, 2)`, "hé"}, // rune-indexed
		// Optional end - omit to mean "to the end of the string".
		{`substring("hello", 0)`, "hello"},
		{`substring("hello", 2)`, "llo"},
		{`substring("hello", 5)`, ""},
		{`substring("héllo", 2)`, "llo"}, // 2-arg form, rune-indexed
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use strings;
def r as string init `+c.expr+`;
printf("[%s]", $r);
`)
		if err != nil {
			t.Errorf("%s: %v", c.expr, err)
			continue
		}
		if out != "["+c.want+"]" {
			t.Errorf("%s: got %q, want %q", c.expr, out, "["+c.want+"]")
		}
	}
}

func TestStringsSubstringErrors(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`substring("hello", 0 - 1, 3)`, "is negative"},
		{`substring("hello", 0, 99)`, "out of range"},
		{`substring("hello", 4, 2)`, "before start"},
		{`substring("hello")`, "2 or 3 arguments"},          // arity: too few
		{`substring("hello", 1, 2, 3)`, "2 or 3 arguments"}, // arity: too many
		{`substring("hello", 99)`, "out of range"},          // 2-arg form, out of range
		{`substring("hello", 0 - 1)`, "is negative"},        // 2-arg form, negative
	}
	for _, c := range cases {
		_, err := run(t, `
use io;
use strings;
def r as string init `+c.expr+`;
`)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want substring %q", c.expr, err, c.want)
		}
	}
}

func TestStringsTypeErrors(t *testing.T) {
	// Non-string operands should error positionally.
	_, err := run(t, `
use io;
use strings;
def r as int init len(42);
`)
	if err == nil || !strings.Contains(err.Error(), "must be string") {
		t.Errorf("got %v", err)
	}
}

func TestStringsRequiresUse(t *testing.T) {
	_, err := run(t, `
use io;
def r as int init len("hello");
`)
	if err == nil || !strings.Contains(err.Error(), "use strings") {
		t.Errorf("got %v", err)
	}
}

func TestM2Comparisons(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{"1 < 2", "true"},
		{"2 < 1", "false"},
		{"3 <= 3", "true"},
		{"3 > 2", "true"},
		{"3 >= 3", "true"},
		{"3 == 3", "true"},
		{"3 == 4", "false"},
		{"1 == 1.0", "true"},   // int/float promotion in equality
		{`"a" == "a"`, "true"}, // string equality
		{`"a" == "b"`, "false"},
		{"true == true", "true"},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
func app() {
    def r as bool init `+c.expr+`;
    printf($r);
}`)
		if err != nil {
			t.Errorf("expr %s: err %v", c.expr, err)
			continue
		}
		if out != c.want {
			t.Errorf("expr %s: got %q, want %q", c.expr, out, c.want)
		}
	}
}

func TestM2IfElseifElse(t *testing.T) {
	// Note: M2 doesn't include unary minus, so we test buckets using only
	// non-negative literals.
	src := func(n int) string {
		return `
use io;
func app() {
    def n as int init ` + itoa(n) + `;
    if ($n == 0) {
        printf("zero");
    } elseif ($n < 10) {
        printf("small");
    } else {
        printf("large");
    }
}`
	}
	cases := map[int]string{0: "zero", 5: "small", 100: "large"}
	for n, want := range cases {
		out, err := run(t, src(n))
		if err != nil {
			t.Errorf("n=%d: err %v", n, err)
			continue
		}
		if out != want {
			t.Errorf("n=%d: got %q, want %q", n, out, want)
		}
	}
}

func TestM2While(t *testing.T) {
	out, err := run(t, `
use io;
func app() {
    def i as int init 0;
    def sum as int init 0;
    while ($i < 5) {
        $sum = $sum + $i;
        $i = $i + 1;
    }
    printf($sum);
}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "10" { // 0+1+2+3+4
		t.Errorf("got %q, want %q", out, "10")
	}
}

func TestM2For(t *testing.T) {
	out, err := run(t, `
use io;
func app() {
    def sum as int init 0;
    for (def i as int init 1; $i <= 5; $i = $i + 1) {
        $sum = $sum + $i;
    }
    printf($sum);
}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "15" { // 1+2+3+4+5
		t.Errorf("got %q, want %q", out, "15")
	}
}

func TestM2ForInitVarNotVisibleOutside(t *testing.T) {
	_, err := run(t, `
use io;
func app() {
    for (def i as int init 0; $i < 1; $i = $i + 1) { }
    printf($i);
}`)
	if err == nil || !strings.Contains(err.Error(), `undefined variable "i"`) {
		t.Errorf("expected $i undefined after loop, got %v", err)
	}
}

func TestM2ConstCannotBeReassigned(t *testing.T) {
	_, err := run(t, `
use io;
func app() {
    def const MAX as int init 10;
    MAX = 20;
}`)
	// This should be a parse error because constants don't have a $-prefix,
	// and we don't currently parse `IDENT = expr` as an assignment. Verify
	// the error is reasonable rather than the program running.
	if err == nil {
		t.Error("expected a parse/runtime error reassigning a constant")
	}
}

func TestM2AssignTypeCheck(t *testing.T) {
	_, err := run(t, `
use io;
func app() {
    def x as int init 1;
    $x = "string";
}`)
	if err == nil || !strings.Contains(err.Error(), "cannot assign") {
		t.Errorf("expected type-mismatch assignment error, got %v", err)
	}
}

func TestM2NoShadowing(t *testing.T) {
	_, err := run(t, `
use io;
func app() {
    def x as int init 1;
    if (true) {
        def x as int init 2;
    }
}`)
	if err == nil || !strings.Contains(err.Error(), "already defined") {
		t.Errorf("expected no-shadowing error, got %v", err)
	}
}

func TestM2BlockScopeDoesNotLeak(t *testing.T) {
	_, err := run(t, `
use io;
func app() {
    if (true) {
        def y as int init 1;
    }
    printf($y);
}`)
	if err == nil || !strings.Contains(err.Error(), `undefined variable "y"`) {
		t.Errorf("expected $y to be out of scope, got %v", err)
	}
}

func TestM2ConditionMustBeBool(t *testing.T) {
	_, err := run(t, `
use io;
func app() {
    if (1) { printf("x"); }
}`)
	if err == nil || !strings.Contains(err.Error(), "must be bool") {
		t.Errorf("expected bool-required error, got %v", err)
	}
}

// itoa avoids pulling strconv into the test file just for tiny ints.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}
