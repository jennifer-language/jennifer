// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package interpreter_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mplx/jennifer-lang/internal/interpreter"
	"github.com/mplx/jennifer-lang/internal/parser"
	"github.com/mplx/jennifer-lang/internal/stdlib"
)

// run lexes/parses/installs stdlib/runs a program and returns captured stdout.
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
	stdlib.Install(in)
	if err := in.Run(prog); err != nil {
		return buf.String(), err
	}
	return buf.String(), nil
}

func TestHelloProgramPrints42(t *testing.T) {
	out, err := run(t, `
use stdlib;
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
use stdlib;
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
use stdlib;
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
use stdlib;
func app() {
    def a as int init 17 / 5;
    def b as int init 17 % 5;
    printf($a);
    printf(" ");
    printf($b);
}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "3 2" {
		t.Errorf("got %q, want %q", out, "3 2")
	}
}

func TestEmptyProgramRunsCleanly(t *testing.T) {
	// app() is no longer required. An empty program (or one with only imports
	// and method defs that are never called) is valid and produces no output.
	out, err := run(t, `use stdlib;`)
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
use stdlib;
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
use stdlib;
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
use stdlib;
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
	if err == nil || !strings.Contains(err.Error(), "use stdlib") {
		t.Errorf("expected use-stdlib error, got %v", err)
	}
}

func TestErrorOnDivisionByZero(t *testing.T) {
	_, err := run(t, `
use stdlib;
func app() { def x as int init 1 / 0; }`)
	if err == nil || !strings.Contains(err.Error(), "division by zero") {
		t.Errorf("expected division-by-zero error, got %v", err)
	}
}

func TestErrorOnTypeMismatch(t *testing.T) {
	_, err := run(t, `
use stdlib;
func app() { def x as int init "nope"; }`)
	if err == nil || !strings.Contains(err.Error(), "cannot initialize int") {
		t.Errorf("expected type-mismatch error, got %v", err)
	}
}

func TestErrorOnUndefinedVar(t *testing.T) {
	_, err := run(t, `
use stdlib;
func app() { printf($missing); }`)
	if err == nil || !strings.Contains(err.Error(), `undefined variable "missing"`) {
		t.Errorf("expected undefined-var error, got %v", err)
	}
}

func TestErrorOnUnknownFunction(t *testing.T) {
	_, err := run(t, `
use stdlib;
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

func TestUserMethodCannotShadowStdlibBuiltin(t *testing.T) {
	// With `use stdlib;`, defining `func printf()` is a shadowing error.
	_, err := run(t, `
use stdlib;
func printf() {}
printf();
`)
	if err == nil || !strings.Contains(err.Error(), "shadows a builtin") {
		t.Errorf("expected shadowing error, got %v", err)
	}
}

func TestReturnValue(t *testing.T) {
	out, err := run(t, `
use stdlib;
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
use stdlib;
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
use stdlib;
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
use stdlib;
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
use stdlib;
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
use stdlib;
func add(a as int, b as int) { return $a + $b; }
add(3, "four");
`)
	if err == nil || !strings.Contains(err.Error(), "argument 2") {
		t.Errorf("expected per-arg type error, got %v", err)
	}
}

func TestParamArityMismatch(t *testing.T) {
	_, err := run(t, `
use stdlib;
func add(a as int, b as int) { return $a + $b; }
add(3);
`)
	if err == nil || !strings.Contains(err.Error(), "takes 2") {
		t.Errorf("expected arity error, got %v", err)
	}
}

func TestRecursionFactorial(t *testing.T) {
	out, err := run(t, `
use stdlib;
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
use stdlib;
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
use stdlib;
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

func TestConstInExpression(t *testing.T) {
	out, err := run(t, `
use stdlib;
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
use stdlib;
def x as int init 5;
printf("%d", x);
`)
	if err == nil || !strings.Contains(err.Error(), "use `$x`") {
		t.Errorf("expected $-prefix hint, got %v", err)
	}
}

func TestBareUndefinedNameError(t *testing.T) {
	_, err := run(t, `
use stdlib;
printf("%d", NOPE);
`)
	if err == nil || !strings.Contains(err.Error(), "undefined name") {
		t.Errorf("expected undefined-name error, got %v", err)
	}
}

func TestParamRejectsDollarAtDefSite(t *testing.T) {
	_, err := run(t, `
use stdlib;
func bad($x as int) { return $x; }
`)
	if err == nil || !strings.Contains(err.Error(), "no `$`") {
		t.Errorf("expected $-at-param error, got %v", err)
	}
}

func TestUserMethodCanReuseBuiltinNameWithoutStdlib(t *testing.T) {
	// Without `use stdlib;`, the name is free - the user's printf is the only one.
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
use stdlib;
func app() {
    def a as float init 1.5;
    def b as float init 2.5;
    printf($a + $b);
}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "4" {
		t.Errorf("got %q, want %q", out, "4")
	}
}

func TestM2IntFloatPromotion(t *testing.T) {
	out, err := run(t, `
use stdlib;
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
use stdlib;
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
use stdlib;
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
use stdlib;
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
use stdlib;
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
	if out != "0 0  false" {
		t.Errorf("got %q, want %q", out, "0 0  false")
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
		{"1 == 1.0", "true"},      // int/float promotion in equality
		{`"a" == "a"`, "true"},    // string equality
		{`"a" == "b"`, "false"},
		{"true == true", "true"},
	}
	for _, c := range cases {
		out, err := run(t, `
use stdlib;
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
use stdlib;
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
use stdlib;
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
use stdlib;
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
use stdlib;
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
use stdlib;
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
use stdlib;
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
use stdlib;
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
use stdlib;
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
use stdlib;
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
