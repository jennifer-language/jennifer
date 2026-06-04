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
func run(t *testing.T, src string) (string, error) {
	t.Helper()
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
import stdlib;
def app() {
    define $x as int init 21;
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
import stdlib;
def app() {
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
import stdlib;
def app() {
    define $r as int init 2 + 3 * 4;
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
import stdlib;
def app() {
    define $a as int init 17 / 5;
    define $b as int init 17 % 5;
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

func TestErrorOnMissingApp(t *testing.T) {
	_, err := run(t, `import stdlib;`)
	if err == nil || !strings.Contains(err.Error(), "no `app()`") {
		t.Errorf("expected missing-app error, got %v", err)
	}
}

func TestErrorOnPrintfWithoutImport(t *testing.T) {
	_, err := run(t, `def app() { printf(1); }`)
	if err == nil || !strings.Contains(err.Error(), "import stdlib") {
		t.Errorf("expected import-stdlib error, got %v", err)
	}
}

func TestErrorOnDivisionByZero(t *testing.T) {
	_, err := run(t, `
import stdlib;
def app() { define $x as int init 1 / 0; }`)
	if err == nil || !strings.Contains(err.Error(), "division by zero") {
		t.Errorf("expected division-by-zero error, got %v", err)
	}
}

func TestErrorOnTypeMismatch(t *testing.T) {
	_, err := run(t, `
import stdlib;
def app() { define $x as int init "nope"; }`)
	if err == nil || !strings.Contains(err.Error(), "cannot initialize int") {
		t.Errorf("expected type-mismatch error, got %v", err)
	}
}

func TestErrorOnUndefinedVar(t *testing.T) {
	_, err := run(t, `
import stdlib;
def app() { printf($missing); }`)
	if err == nil || !strings.Contains(err.Error(), `undefined variable "missing"`) {
		t.Errorf("expected undefined-var error, got %v", err)
	}
}

func TestErrorOnUnknownFunction(t *testing.T) {
	_, err := run(t, `
import stdlib;
def app() { nope(1); }`)
	if err == nil || !strings.Contains(err.Error(), "unknown function") {
		t.Errorf("expected unknown-function error, got %v", err)
	}
}

func TestErrorOnDuplicateMethod(t *testing.T) {
	_, err := run(t, `
def app() {}
def app() {}`)
	if err == nil || !strings.Contains(err.Error(), "defined more than once") {
		t.Errorf("expected duplicate-method error, got %v", err)
	}
}
