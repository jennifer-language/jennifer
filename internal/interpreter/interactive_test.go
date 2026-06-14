// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package interpreter_test

import (
	"bytes"
	"testing"

	"github.com/mplx/jennifer-lang/internal/interpreter"
	"github.com/mplx/jennifer-lang/internal/lib/convert"
	iolib "github.com/mplx/jennifer-lang/internal/lib/io"
	listslib "github.com/mplx/jennifer-lang/internal/lib/lists"
	mapslib "github.com/mplx/jennifer-lang/internal/lib/maps"
	mathlib "github.com/mplx/jennifer-lang/internal/lib/math"
	oslib "github.com/mplx/jennifer-lang/internal/lib/os"
	stringslib "github.com/mplx/jennifer-lang/internal/lib/strings"
	"github.com/mplx/jennifer-lang/internal/parser"
)

// newReplInterp builds an interpreter wired with the standard libraries and
// stdout redirected to a buffer the caller can inspect.
func newReplInterp() (*interpreter.Interpreter, *bytes.Buffer) {
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
	return in, &buf
}

// evalLine parses a single REPL input and feeds it to EvalInteractive.
func evalLine(t *testing.T, in *interpreter.Interpreter, src string) interpreter.Value {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	v, err := in.EvalInteractive(prog)
	if err != nil {
		t.Fatalf("eval %q: %v", src, err)
	}
	return v
}

func TestEvalInteractivePersistsGlobals(t *testing.T) {
	in, _ := newReplInterp()
	evalLine(t, in, "def x as int init 21;")
	got := evalLine(t, in, "$x + $x;")
	if got.Kind != interpreter.KindInt || got.Int != 42 {
		t.Errorf("expected int 42, got %v %v", got.Kind, got)
	}
}

func TestEvalInteractiveReturnsFinalExprValue(t *testing.T) {
	in, _ := newReplInterp()
	got := evalLine(t, in, `"hello";`)
	if got.Kind != interpreter.KindString || got.Str != "hello" {
		t.Errorf("expected string 'hello', got %v %v", got.Kind, got)
	}
	// Multi-statement: only the last ExprStmt's value is returned.
	got = evalLine(t, in, "def y as int init 5; $y * 3;")
	if got.Kind != interpreter.KindInt || got.Int != 15 {
		t.Errorf("expected int 15, got %v %v", got.Kind, got)
	}
}

func TestEvalInteractiveNullWhenLastStmtIsNotExpr(t *testing.T) {
	in, _ := newReplInterp()
	got := evalLine(t, in, "def z as int init 7;")
	if got.Kind != interpreter.KindNull {
		t.Errorf("expected null, got %v %v", got.Kind, got)
	}
}

func TestEvalInteractiveMethodRedefineAllowed(t *testing.T) {
	in, _ := newReplInterp()
	evalLine(t, in, "func f() { return 1; }")
	got := evalLine(t, in, "f();")
	if got.Kind != interpreter.KindInt || got.Int != 1 {
		t.Errorf("first f(): expected 1, got %v %v", got.Kind, got)
	}
	// In Run() mode this would error with "defined more than once". The REPL
	// silently overwrites so the user can iterate on a method.
	evalLine(t, in, "func f() { return 2; }")
	got = evalLine(t, in, "f();")
	if got.Kind != interpreter.KindInt || got.Int != 2 {
		t.Errorf("second f(): expected 2 after redefine, got %v %v", got.Kind, got)
	}
}

func TestEvalInteractiveImportPersists(t *testing.T) {
	in, buf := newReplInterp()
	// First input enables io. Second input - in a separate call - should still
	// be able to call printf because the import survives across calls.
	evalLine(t, in, "use io;")
	evalLine(t, in, `io.printf("ok\n");`)
	if got := buf.String(); got != "ok\n" {
		t.Errorf("expected printf output to persist, got %q", got)
	}
	// Re-issuing the same `use` is a silent no-op (not a redefinition error).
	evalLine(t, in, "use io;")
}

func TestEvalInteractiveBuiltinShadowStillRejected(t *testing.T) {
	// `len` is a language keyword (M15.4 promoted it from a `core`
	// global to a language built-in primary), so `func len()` is a
	// parse-time rejection rather than the M5-era "shadows builtin"
	// runtime check.
	_, err := parser.Parse("func len() { return 1; }")
	if err == nil {
		t.Fatal("expected parse error rejecting `func len`, got nil")
	}
}

func TestEvalInteractiveQualifiedCall(t *testing.T) {
	in, buf := newReplInterp()
	evalLine(t, in, "use io;")
	evalLine(t, in, "use os;")
	evalLine(t, in, `io.printf("%s\n", os.PLATFORM);`)
	if got := buf.String(); got == "" || got[len(got)-1] != '\n' {
		t.Errorf("expected platform output ending in newline, got %q", got)
	}
}

func TestEvalInteractiveAliasedQualifiedCall(t *testing.T) {
	in, buf := newReplInterp()
	evalLine(t, in, "use io;")
	evalLine(t, in, "use os as o;")
	evalLine(t, in, `io.printf("%s", o.EOL);`)
	if got := buf.String(); got != "\n" && got != "\r\n" {
		t.Errorf("expected platform line ending, got %q", got)
	}
}
