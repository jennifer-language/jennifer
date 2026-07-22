// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"bytes"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/convert"
	"jennifer-lang.dev/jennifer/internal/lib/io"
	"jennifer-lang.dev/jennifer/internal/lib/lists"
	"jennifer-lang.dev/jennifer/internal/lib/maps"
	"jennifer-lang.dev/jennifer/internal/lib/math"
	"jennifer-lang.dev/jennifer/internal/lib/os"
	"jennifer-lang.dev/jennifer/internal/lib/strings"
	"jennifer-lang.dev/jennifer/internal/parser"
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
	listslib.Install(in)
	mapslib.Install(in)
	oslib.Install(in)
	if err := in.Run(prog); err != nil {
		return buf.String(), err
	}
	return buf.String(), nil
}

// runWithStdin is the input-capable variant of run: stdin is fed from
// the given string, captured stdout is returned. Used by the readLine /
// eof end-to-end tests.
func runWithStdin(t *testing.T, src, stdin string) (string, error) {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	in.In = strings.NewReader(stdin)
	iolib.Install(in)
	convert.Install(in)
	mathlib.Install(in)
	stringslib.Install(in)
	listslib.Install(in)
	mapslib.Install(in)
	oslib.Install(in)
	if err := in.Run(prog); err != nil {
		return buf.String(), err
	}
	return buf.String(), nil
}

func TestReadLineEofLoopEndToEnd(t *testing.T) {
	out, err := runWithStdin(t, `
use io;
while (not io.eof()) {
    def line as string init io.readLine();
    io.printf("%s\n", $line);
}
`, "alpha\nbeta\ngamma\n")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "alpha\nbeta\ngamma\n" {
		t.Errorf("got %q", out)
	}
}

func TestReadLineWithPromptEndToEnd(t *testing.T) {
	out, err := runWithStdin(t, `
use io;
def name as string init io.readLine("name: ");
io.printf("hi, %s\n", $name);
`, "Jennifer\n")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "name: hi, Jennifer\n" {
		t.Errorf("got %q", out)
	}
}

func TestHelloProgramPrints42(t *testing.T) {
	out, err := run(t, `
use io;
func app() {
    def x as int init 21;
    io.printf($x + $x);
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
    io.printf("hello, jennifer\n");
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
    io.printf($r);
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
    def a as int init 17  // 5;
    def b as int init 17 % 5;
    def c as float init 17 / 5;
    io.printf($a);
    io.printf(" ");
    io.printf($b);
    io.printf(" ");
    io.printf($c);
}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "3 2 3.4" {
		t.Errorf("got %q, want %q", out, "3 2 3.4")
	}
}

// `%` is floored, matching `//`, so the div/mod identity holds for negative
// operands: (a // b) * b + (a % b) == a.
func TestFlooredModulo(t *testing.T) {
	prog := `use io;
def a as int init (0 - 7) % 3;
def b as int init 7 % (0 - 3);
def c as int init (0 - 7) % (0 - 3);
io.printf("%d %d %d", $a, $b, $c);`
	out, err := run(t, prog)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// floored: -7 % 3 = 2, 7 % -3 = -2, -7 % -3 = -1
	if out != "2 -2 -1" {
		t.Errorf("floored %% got %q, want %q", out, "2 -2 -1")
	}
	// The div/mod identity holds: (a // b) * b + (a % b) == a for a=-7, b=3.
	id, err := run(t, `use io;
def q as int init (0 - 7) // 3;
def r as int init (0 - 7) % 3;
io.printf("%d", $q * 3 + $r);`)
	if err != nil {
		t.Fatalf("identity err: %v", err)
	}
	if id != "-7" {
		t.Errorf("div/mod identity got %q, want -7", id)
	}
}

// Integer arithmetic that overflows int64 is a positioned runtime error, not a
// silent wrap - consistent with the language's "undefined results error" stance.
func TestIntegerOverflowErrors(t *testing.T) {
	overflowing := []string{
		`def x as int init 9223372036854775807 + 1;`,
		`def x as int init 9223372036854775807 * 2;`,
		`def x as int init (0 - 9223372036854775807) - 2;`,
		`def x as int init (0 - 9223372036854775807) * 3;`,
	}
	for _, src := range overflowing {
		if _, err := run(t, src); err == nil {
			t.Errorf("expected an overflow error for %q", src)
		}
	}
	// In-range arithmetic still works, including the exact boundaries.
	ok, err := run(t, `use io;
def hi as int init 9223372036854775806 + 1;
io.printf("%d", $hi);`)
	if err != nil || ok != "9223372036854775807" {
		t.Errorf("boundary add: out=%q err=%v", ok, err)
	}
}

// A mixed int/float comparison must be exact: promoting the int to float64
// loses precision above 2^53, which would make 9007199254740993 (int) compare
// equal to 9007199254740992.0 (float).
func TestMixedComparisonIsExact(t *testing.T) {
	out, err := run(t, `use io;
def n as int init 9007199254740993;
def f as float init 9007199254740992.0;
io.printf("%t %t %t %t", $n == $f, $n > $f, $f < $n, $n <= $f);`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// n is exactly one greater than f: not equal, n > f, f < n, not (n <= f).
	if out != "false true true false" {
		t.Errorf("exact mixed comparison got %q, want %q", out, "false true true false")
	}
	// A genuinely-equal mixed pair still compares equal.
	eq, err := run(t, `use io; def n as int init 5; def f as float init 5.0; io.printf("%t", $n == $f);`)
	if err != nil || eq != "true" {
		t.Errorf("5 == 5.0: out=%q err=%v", eq, err)
	}
	// `!=` is the exact negation of `==`, including the large-int case. Uses
	// variables so the literal folder doesn't pre-compute it (folder leaves mixed
	// int/float comparisons to the runtime for exactly this reason).
	ne, err := run(t, `use io;
def n as int init 9007199254740993;
def f as float init 9007199254740992.0;
io.printf("%t %t", $n != $f, $n != $n);`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ne != "true false" {
		t.Errorf("!= exact got %q, want %q", ne, "true false")
	}
}

// `!=` is a full comparison operator across every comparable kind, exactly the
// negation of `==`.
func TestNotEqualOperator(t *testing.T) {
	out, err := run(t, `use io;
io.printf("%t %t %t %t %t\n", 1 != 2, 1 != 1, "a" != "b", true != false, null != null);
def a as list of int init [1, 2]; def b as list of int init [1, 3];
io.printf("%t\n", $a != $b);`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "true false true true false\ntrue\n" {
		t.Errorf("got %q", out)
	}
	// Precedence matches `==`: comparison binds tighter than `and`, and `!=`
	// sits at the same rung as `==` (below the bit ops). `2 & 3 != 2` parses as
	// `(2 & 3) != 2` -> `2 != 2` -> false.
	p, err := run(t, `use io; io.printf("%t %t", 3 != 4 and 1 != 2, 2 & 3 != 2);`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if p != "true false" {
		t.Errorf("precedence got %q, want %q", p, "true false")
	}
}

// A method may not share its name with a top-level variable or constant (the
// no-shadowing rule applies both directions).
func TestMethodGlobalNameClashRejected(t *testing.T) {
	for _, src := range []string{
		`def foo as int init 1; func foo() { return 2; }`,
		`func bar() { return 1; } def bar as int init 2;`,
		`def const BAZ as int init 1; func BAZ() { return 2; }`,
	} {
		if _, err := run(t, src); err == nil {
			t.Errorf("expected a method/global clash error for %q", src)
		}
	}
	// A method and a *local* (method-body) name that don't collide at top
	// level are fine.
	if _, err := run(t, `use io; func f() { def x as int init 1; return $x; } io.printf("%d", f());`); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// The `$` sigil is for mutable variables; a constant read with `$` is rejected,
// while a bare constant read and a `$CONST[...]` mutation attempt (which the
// runtime rejects as a const mutation) are handled elsewhere.
func TestConstSigilRejectedOnRead(t *testing.T) {
	if _, err := run(t, `use io; def const MAX as int init 10; io.printf("%d", $MAX);`); err == nil {
		t.Error("expected a $CONST read to be rejected")
	}
	// Bare constant read works.
	out, err := run(t, `use io; def const MAX as int init 10; io.printf("%d", MAX);`)
	if err != nil || out != "10" {
		t.Errorf("bare const read: out=%q err=%v", out, err)
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
io.printf($x + $x);
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
func show() { io.printf($greeting); }
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
	_, err := run(t, `func app() { io.printf(1); }`)
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
func app() { io.printf($missing); }`)
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

func TestLenIsKeyword(t *testing.T) {
	// `len` is a language built-in primary, so `func len()` is a
	// parse-time rejection rather than a "shadows builtin" runtime check.
	_, err := run(t, `
func len() {}
len();
`)
	if err == nil || !strings.Contains(err.Error(), "expected IDENT") {
		t.Errorf("expected parse error rejecting `func len`, got %v", err)
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
	if err == nil || !strings.Contains(err.Error(), "available: convert, io, lists, maps, math, os, strings") {
		t.Errorf("expected error to list available libraries, got %v", err)
	}
}

func TestReturnValue(t *testing.T) {
	out, err := run(t, `
use io;
func answer() { return 42; }
io.printf(answer());
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
io.printf(nothing());
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
    io.printf("a");
    return 1;
    io.printf("b");
}
io.printf(early());
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
io.printf(find());
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "3" {
		t.Errorf("got %q, want %q", out, "3")
	}
}

// ---- parameters ----

func TestParamsAddTwoInts(t *testing.T) {
	out, err := run(t, `
use io;
func add(a as int, b as int) { return $a + $b; }
io.printf(add(3, 4));
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
io.printf(fact(7));
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
    io.printf($greeting + $name);
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
io.printf("%d", MAX);
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
io.printf("%d %d", MAX_RETRIES, HTTP_OK);
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
io.printf("%d", $y);
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
io.printf("%d", x);
`)
	if err == nil || !strings.Contains(err.Error(), "use `$x`") {
		t.Errorf("expected $-prefix hint, got %v", err)
	}
}

func TestBareUndefinedNameError(t *testing.T) {
	_, err := run(t, `
use io;
io.printf("%d", NOPE);
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
	// Without `use os;`, the name `platform` (which `os` would expose as a
	// namespaced builtin) is free for ordinary use. Domain
	// libraries do not expose bare-name globals, so the case
	// "`func printf()` allowed when io isn't imported" is
	// trivially true; this test exercises the namespace-prefix path
	// instead.
	out, err := run(t, `
func platform() {}
platform();
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "" {
		t.Errorf("got %q, want empty (user platform is a no-op)", out)
	}
}

// ---- tests ----

func TestM2FloatArithmetic(t *testing.T) {
	out, err := run(t, `
use io;
func app() {
    def a as float init 1.5;
    def b as float init 2.5;
    io.printf($a + $b);
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
    io.printf($r);
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
    io.printf($a + $b);
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
    io.printf($t);
    io.printf(" ");
    io.printf($f);
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
    io.printf($n);
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
    io.printf($i);
    io.printf(" ");
    io.printf($f);
    io.printf(" ");
    io.printf($s);
    io.printf(" ");
    io.printf($b);
}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "0 0.0  false" {
		t.Errorf("got %q, want %q", out, "0 0.0  false")
	}
}

// ---- logical operators + unary minus ----

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
io.printf("%t", $r);
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
io.printf("%t", $r);
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
    io.printf("BOOM");
    return true;
}
def a as bool init false and boom();
def b as bool init true or boom();
io.printf("|%t %t", $a, $b);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "|false true" {
		t.Errorf("got %q, want %q", out, "|false true")
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
io.printf("%d %f %d %d", $i, $f, $doubleNeg, $neg);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "-5 -0.25 7 -10" {
		t.Errorf("got %q", out)
	}
}

func TestUnaryMinusPrecedence(t *testing.T) {
	// -3 + 10 -> (-3) + 10 -> 7
	// -3 * 2 -> (-3) * 2 -> -6
	out, err := run(t, `
use io;
io.printf("%d %d", -3 + 10, -3 * 2);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "7 -6" {
		t.Errorf("got %q, want %q", out, "7 -6")
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

// ---- division semantics ----

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
io.printf("%f", $r);
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
		{"5  // 2", "2"},
		{"6  // 2", "3"},
		// Python-style floor division: -5  // 2 = -3 (toward -inf)
		// We don't have unary minus on literals via prefix in numeric ctx
		// without space, but `0 - 5` works.
		{"(0 - 5)  // 2", "-3"},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
def r as int init `+c.expr+`;
io.printf("%d", $r);
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
def r as float init 5.7  // 2.0;
io.printf("%f", $r);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "2.0" {
		t.Errorf("got %q, want %q", out, "2.0")
	}
}

// ---- float display ----

func TestFloatDisplayAlwaysHasDot(t *testing.T) {
	// 5.0 should print as "5.0", not "5".
	out, err := run(t, `
use io;
def x as float init 5.0;
io.printf("%f", $x);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "5.0" {
		t.Errorf("got %q, want %q", out, "5.0")
	}
}

// ---- convert library ----

func TestConvertInt(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{`convert.toInt("42")`, "42"},
		{`convert.toInt(3.7)`, "3"}, // truncate
		{`convert.toInt(true)`, "1"},
		{`convert.toInt(false)`, "0"},
		{`convert.toInt(99)`, "99"}, // identity
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use convert;
def r as int init `+c.expr+`;
io.printf("%d", $r);
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
		{`convert.toInt("abc")`, "not a valid integer"},
		{`convert.toInt(null)`, "cannot convert null"},
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
		{`convert.toFloat(5)`, "5.0"},
		{`convert.toFloat("3.14")`, "3.14"},
		{`convert.toFloat(true)`, "1.0"},
		{`convert.toFloat(false)`, "0.0"},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use convert;
def r as float init `+c.expr+`;
io.printf("%f", $r);
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
		{`convert.toString(42)`, "42"},
		{`convert.toString(3.14)`, "3.14"},
		{`convert.toString(true)`, "true"},
		{`convert.toString(null)`, "null"},
		{`convert.toString("hi")`, "hi"},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use convert;
def r as string init `+c.expr+`;
io.printf("%s", $r);
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
		{`convert.toBool("true")`, "true"},
		{`convert.toBool("false")`, "false"},
		{`convert.toBool(0)`, "false"},
		{`convert.toBool(1)`, "true"},
		{`convert.toBool(0.0)`, "false"},
		{`convert.toBool(1.0)`, "true"},
		{`convert.toBool(true)`, "true"},   // identity
		{`convert.toBool(false)`, "false"}, // identity
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use convert;
def r as bool init `+c.expr+`;
io.printf("%t", $r);
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
		{`convert.toBool("maybe")`, `only "true" or "false"`},
		{`convert.toBool(5)`, "only 0 and 1"},
		{`convert.toBool(0 - 1)`, "only 0 and 1"}, // -1 must error too
		{`convert.toBool(123)`, "only 0 and 1"},
		{`convert.toBool(1.5)`, "only 0.0 and 1.0"},
		{`convert.toBool(2.0)`, "only 0.0 and 1.0"},
		{`convert.toBool(null)`, "cannot convert null"},
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
		{`convert.typeOf(5)`, "int"},
		{`convert.typeOf(3.14)`, "float"},
		{`convert.typeOf("hi")`, "string"},
		{`convert.typeOf(true)`, "bool"},
		{`convert.typeOf(null)`, "null"},
		{`convert.typeOf(5 / 2)`, "float"},
		{`convert.typeOf(5  // 2)`, "int"},
		{`convert.typeOf(2.5 * 2)`, "float"},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use convert;
def r as string init `+c.expr+`;
io.printf("%s", $r);
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
def r as int init convert.toInt("42");
`)
	if err == nil || !strings.Contains(err.Error(), "use convert") {
		t.Errorf("expected use-convert hint, got %v", err)
	}
}

func TestTypeNameAsBareReferenceErrors(t *testing.T) {
	// Type keywords have no expression-position meaning - bare
	// or call form. The parser points the user at the namespaced
	// convert call.
	_, err := run(t, `
use io;
use convert;
def r as int init int;
`)
	if err == nil || !strings.Contains(err.Error(), "type names belong after `as`") {
		t.Errorf("expected hint, got %v", err)
	}
}

// ---- math library ----

func TestMathAbs(t *testing.T) {
	cases := []struct{ expr, want string }{
		{"math.abs(5)", "5"},
		{"math.abs(0 - 5)", "5"},
		{"math.abs(0)", "0"},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use math;
def r as int init `+c.expr+`;
io.printf("%d", $r);
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
def r as float init math.abs(-3.14);
io.printf("%f", $r);
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
		{"math.min(3, 7)", "3", "int"},
		{"math.min(7, 3)", "3", "int"},
		{"math.max(3, 7)", "7", "int"},
		{"math.max(7, 3)", "7", "int"},
		{"math.min(3, 2.5)", "2.5", "float"}, // mixed -> float
		{"math.max(3, 2.5)", "3.0", "float"}, // mixed -> float (int promoted)
		{"math.min(1.0, 2.0)", "1.0", "float"},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use math;
def r as `+c.typ+` init `+c.expr+`;
io.printf("%v", $r);
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
def r as float init math.sqrt(16);
io.printf("%f", $r);
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
def r as float init math.sqrt(0 - 1);
`)
	if err == nil || !strings.Contains(err.Error(), "undefined for negative") {
		t.Errorf("got %v", err)
	}
}

func TestMathPow(t *testing.T) {
	out, err := run(t, `
use io;
use math;
def a as float init math.pow(2, 10);
def b as float init math.pow(2, 0.5);
io.printf("%f %f", $a, $b);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// math.pow(2,10) = 1024.0; math.pow(2,0.5) = math.sqrt(2)
	if !strings.HasPrefix(out, "1024.0 1.41") {
		t.Errorf("got %q", out)
	}
}

func TestMathFloorCeilRound(t *testing.T) {
	cases := []struct{ expr, want string }{
		{"math.floor(3.7)", "3"},
		{"math.floor(3.2)", "3"},
		{"math.floor(0 - 3.2)", "-4"}, // toward -inf
		{"math.ceil(3.2)", "4"},
		{"math.ceil(3.7)", "4"},
		{"math.ceil(0 - 3.7)", "-3"},
		{"math.round(2.5)", "3"}, // half away from zero
		{"math.round(2.4)", "2"},
		{"math.round(0 - 2.5)", "-3"},
		{"math.floor(5)", "5"}, // int passes through
		{"math.ceil(5)", "5"},
		{"math.round(5)", "5"},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use math;
def r as int init `+c.expr+`;
io.printf("%d", $r);
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
def pi as float init math.PI;
def e as float init math.E;
io.printf("%f %f", $pi, $e);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.HasPrefix(out, "3.14159") || !strings.Contains(out, " 2.71828") {
		t.Errorf("got %q", out)
	}
}

func TestMathConstantRequiresUse(t *testing.T) {
	// Math is namespaced; without `use math;` the `math` prefix
	// is unknown.
	_, err := run(t, `
use io;
def r as float init math.PI;
`)
	if err == nil || !strings.Contains(err.Error(), "math") {
		t.Errorf("got %v", err)
	}
}

func TestMathArityErrors(t *testing.T) {
	cases := []struct{ expr, want string }{
		{"math.abs()", "expects 1 argument"},
		{"math.abs(1, 2)", "expects 1 argument"},
		{"math.min(1)", "expects 2 arguments"},
		{"math.pow(1)", "expects 2 arguments"},
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

// ---- strings library ----

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
io.printf("%d", $r);
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
def a as string init strings.upper("hello");
def b as string init strings.lower("HELLO");
io.printf("%s %s", $a, $b);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "HELLO hello" {
		t.Errorf("got %q", out)
	}
}

func TestStringsSearchPredicates(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`strings.contains("hello world", "world")`, "true"},
		{`strings.contains("hello", "z")`, "false"},
		{`strings.startsWith("hello", "he")`, "true"},
		{`strings.startsWith("hello", "lo")`, "false"},
		{`strings.endsWith("hello", "lo")`, "true"},
		{`strings.endsWith("hello", "he")`, "false"},
		{`strings.contains("", "")`, "true"}, // empty is always contained
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use strings;
def r as bool init `+c.expr+`;
io.printf("%t", $r);
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
		{`strings.indexOf("hello", "l")`, "2"},
		{`strings.indexOf("hello", "z")`, "-1"},
		{`strings.indexOf("hello", "")`, "0"},
		{`strings.indexOf("héllo", "l")`, "2"}, // rune index, not byte (é is 2 bytes)
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use strings;
def r as int init `+c.expr+`;
io.printf("%d", $r);
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
		{`strings.trim("   hello   ")`, "hello"},
		{`strings.trim("\t\nhello\n")`, "hello"},
		{`strings.trim("nothing")`, "nothing"},
		{`strings.trimLeft("   hello   ")`, "hello   "},
		{`strings.trimRight("   hello   ")`, "   hello"},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use strings;
def r as string init `+c.expr+`;
io.printf("[%s]", $r);
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
		{`strings.replace("hello world", "world", "Jennifer")`, "hello Jennifer"},
		{`strings.replace("a-b-c", "-", "/")`, "a/b/c"}, // replace all
		{`strings.replace("xyz", "q", "?")`, "xyz"},     // no occurrence -> unchanged
		{`strings.replace("", "x", "y")`, ""},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use strings;
def r as string init `+c.expr+`;
io.printf("%s", $r);
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
		{`strings.repeat("ab", 3)`, "ababab"},
		{`strings.repeat("x", 0)`, ""},
		{`strings.repeat("", 5)`, ""},
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use strings;
def r as string init `+c.expr+`;
io.printf("[%s]", $r);
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
def r as string init strings.repeat("x", 0 - 1);
`)
	if err == nil || !strings.Contains(err.Error(), "negative count") {
		t.Errorf("got %v", err)
	}
}

func TestStringsSubstring(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`strings.substring("hello", 0, 5)`, "hello"},
		{`strings.substring("hello", 1, 4)`, "ell"},
		{`strings.substring("hello", 0, 0)`, ""},
		{`strings.substring("hello", 5, 5)`, ""},   // at end
		{`strings.substring("héllo", 0, 2)`, "hé"}, // rune-indexed
		// Optional end - omit to mean "to the end of the string".
		{`strings.substring("hello", 0)`, "hello"},
		{`strings.substring("hello", 2)`, "llo"},
		{`strings.substring("hello", 5)`, ""},
		{`strings.substring("héllo", 2)`, "llo"}, // 2-arg form, rune-indexed
	}
	for _, c := range cases {
		out, err := run(t, `
use io;
use strings;
def r as string init `+c.expr+`;
io.printf("[%s]", $r);
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
		{`strings.substring("hello", 0 - 1, 3)`, "is negative"},
		{`strings.substring("hello", 0, 99)`, "out of range"},
		{`strings.substring("hello", 4, 2)`, "before start"},
		{`strings.substring("hello")`, "2 or 3 arguments"},          // arity: too few
		{`strings.substring("hello", 1, 2, 3)`, "2 or 3 arguments"}, // arity: too many
		{`strings.substring("hello", 99)`, "out of range"},          // 2-arg form, out of range
		{`strings.substring("hello", 0 - 1)`, "is negative"},        // 2-arg form, negative
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
	// `len` lives in the auto-loaded `core` library now (moved from
	// `strings`), so it's available without `use strings;`. A non-string
	// argument still errors - just with a different message that names
	// the polymorphic intent. `len` also accepts lists/maps.
	_, err := run(t, `
def r as int init len(42);
`)
	if err == nil || !strings.Contains(err.Error(), "len()") {
		t.Errorf("got %v", err)
	}
}

func TestLenIsAutoLoaded(t *testing.T) {
	// `len` is in the `core` library, which is auto-loaded. No `use`
	// statement is needed - calling `len("hello")` from a bare program
	// must succeed.
	out, err := run(t, `
use io;
io.printf("%d", len("hello"));
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "5" {
		t.Errorf("got %q, want %q", out, "5")
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
    io.printf($r);
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
	// Note: this test avoids unary minus, so we test buckets using only
	// non-negative literals.
	src := func(n int) string {
		return `
use io;
func app() {
    def n as int init ` + itoa(n) + `;
    if ($n == 0) {
        io.printf("zero");
    } elseif ($n < 10) {
        io.printf("small");
    } else {
        io.printf("large");
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
    io.printf($sum);
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
    io.printf($sum);
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
    io.printf($i);
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
    io.printf($y);
}`)
	if err == nil || !strings.Contains(err.Error(), `undefined variable "y"`) {
		t.Errorf("expected $y to be out of scope, got %v", err)
	}
}

func TestM2ConditionMustBeBool(t *testing.T) {
	_, err := run(t, `
use io;
func app() {
    if (1) { io.printf("x"); }
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

// evalCall binds each argument into the borrowed call frame as it evaluates
// (no intermediate []Value since the slot-backed-bindings work). These pin the
// observable semantics that interleaving must preserve: strict left-to-right
// argument evaluation with side effects visible to later args, and correct
// binding under deep nested calls in argument position (frames borrowed from
// the pool while the callee frame is held).
func TestCallArgSideEffectOrdering(t *testing.T) {
	out, err := run(t, `use io;
def g as int init 0;
func bump() { $g = $g + 10; return $g; }
func pair(a as int, b as int) { io.printf("%d,%d,%d", $a, $b, $g); }
pair(bump(), $g + bump());`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// bump() -> 10 (g=10); $g + bump() -> 10 + 20 = 30 (g=20); callee sees g=20.
	if out != "10,30,20" {
		t.Fatalf("got %q, want 10,30,20", out)
	}
}

func TestCallNestedInArgPosition(t *testing.T) {
	out, err := run(t, `use io;
func fib(n as int) { if ($n < 2) { return $n; } return fib($n - 1) + fib($n - 2); }
func add(a as int, b as int, c as int) { return $a + $b + $c; }
io.printf("%d", add(fib(10), fib(11), fib(12)));`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// 55 + 89 + 144 = 288
	if out != "288" {
		t.Fatalf("got %q, want 288", out)
	}
}
