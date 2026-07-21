// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// ---- try / catch / throw ----

func TestTryCatchBasic(t *testing.T) {
	out, err := run(t, `
use io;
try {
    throw Error{kind: "demo", message: "hi", file: "", line: 0, col: 0};
    io.printf("unreachable\n");
} catch (e) {
    io.printf("kind=%s msg=%s\n", $e.kind, $e.message);
}
io.printf("after\n");
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "kind=demo msg=hi\nafter\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestTryNoThrowRunsBody(t *testing.T) {
	out, err := run(t, `
use io;
try {
    io.printf("body\n");
} catch (e) {
    io.printf("handler (BAD)\n");
}
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "body\n" {
		t.Errorf("got %q", out)
	}
}

func TestTryCatchesRuntimeError(t *testing.T) {
	// Out-of-bounds is a runtimeError; it should be wrapped into an
	// Error struct and bound to the catch variable.
	out, err := run(t, `
use io;
def xs as list of int init [10, 20];
try {
    def bad as int init $xs[5];
} catch (e) {
    io.printf("rt caught: kind=%s\n", $e.kind);
}
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(out, "rt caught: kind=runtime") {
		t.Errorf("got %q", out)
	}
}

func TestThrowAnyValue(t *testing.T) {
	// The spec allows throwing any value. A bare string still works -
	// it just won't have a .kind / .message field.
	out, err := run(t, `
use io;
use convert;
try {
    throw "boom";
} catch (e) {
    io.printf("got %s of kind %s\n", $e, convert.typeOf($e));
}
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "got boom of kind string\n" {
		t.Errorf("got %q", out)
	}
}

// The catch variable must survive catch-body defs. It occupies slot 0 of the
// handler frame; a name-only binding into a fresh env leaves the slot slice
// empty, so the first catch-body `def` (slot 1) grows the slice over slot 0
// and every later slot-resolved `$e` read hits a zeroed binding (null).
func TestCatchVariableSurvivesHandlerDefs(t *testing.T) {
	out, err := run(t, `
use io;
try {
    throw Error{kind: "demo", message: "boom", file: "", line: 0, col: 0};
} catch (e) {
    def x as int init 1;
    def y as string init "two";
    io.printf("msg=%s x=%d y=%s\n", $e.message, $x, $y);
}
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "msg=boom x=1 y=two\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// The try body is its own block scope: a `def` inside it is not visible after
// the try, so a `def` skipped by a throw reads as an undefined-variable error
// (at parse time) rather than a silent null. A def used within the body works.
func TestTryBodyDefIsBlockScoped(t *testing.T) {
	// Referencing a try-body def after the try is a parse-time undefined error.
	if _, err := run(t, `
use io;
try { throw Error{kind: "x", message: "m", file: "", line: 0, col: 0}; def x as int init 5; } catch (e) {}
io.printf("%v", $x);
`); err == nil {
		t.Error("expected an undefined-variable error for a try-body def read after the try")
	}
	// A def used within the try body works normally.
	out, err := run(t, `
use io;
try { def x as int init 5; io.printf("%d", $x); } catch (e) { io.printf("bad"); }
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "5" {
		t.Errorf("try-body def use: got %q, want 5", out)
	}
}

func TestCatchDispatchOnKind(t *testing.T) {
	out, err := run(t, `
use io;
func raise(k as string) {
    throw Error{kind: $k, message: "x", file: "", line: 0, col: 0};
}
try {
    raise("parse_error");
} catch (e) {
    if ($e.kind == "parse_error") {
        io.printf("parse\n");
    } else {
        io.printf("other\n");
    }
}
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "parse\n" {
		t.Errorf("got %q", out)
	}
}

func TestReThrowPropagates(t *testing.T) {
	// `throw $err;` inside a catch re-raises to the next enclosing
	// `try`.
	out, err := run(t, `
use io;
try {
    try {
        throw Error{kind: "inner", message: "x", file: "", line: 0, col: 0};
    } catch (e) {
        throw $e;
    }
} catch (e) {
    io.printf("outer %s\n", $e.kind);
}
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "outer inner\n" {
		t.Errorf("got %q", out)
	}
}

func TestUncaughtThrowReachesProgramBoundary(t *testing.T) {
	_, err := run(t, `
throw Error{kind: "escape", message: "x", file: "", line: 0, col: 0};
`)
	if err == nil {
		t.Fatal("expected uncaught error")
	}
	if _, ok := err.(*interpreter.ErrorSignal); !ok {
		t.Fatalf("got %T, want *ErrorSignal", err)
	}
}

func TestExitInsideTryIsUncatchable(t *testing.T) {
	// `exit EXPR;` is the program-level escape hatch; the catch block
	// must not run, and the ExitSignal must propagate.
	_, err := run(t, `
try {
    exit 7;
} catch (e) {
    return;
}
`)
	if err == nil {
		t.Fatal("expected ExitSignal")
	}
	ex, ok := err.(*interpreter.ExitSignal)
	if !ok {
		t.Fatalf("got %T", err)
	}
	if ex.Code != 7 {
		t.Errorf("got exit code %d, want 7", ex.Code)
	}
}

func TestReturnInsideTryPropagates(t *testing.T) {
	out, err := run(t, `
use io;
func f() {
    try {
        return 42;
    } catch (e) {
        return -1;
    }
}
io.printf("%d\n", f());
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "42\n" {
		t.Errorf("got %q", out)
	}
}

func TestBreakInsideTryPropagates(t *testing.T) {
	out, err := run(t, `
use io;
for (def i as int init 0; $i < 5; $i = $i + 1) {
    try {
        if ($i == 2) { break; }
        io.printf("%d ", $i);
    } catch (e) {
        io.printf("caught\n");
    }
}
io.printf("\n");
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "0 1 \n" {
		t.Errorf("got %q", out)
	}
}

func TestCatchScopeDoesNotLeak(t *testing.T) {
	_, err := run(t, `
use io;
try {
    throw "x";
} catch (e) {
    io.printf("ok\n");
}
io.printf("%s\n", $e);
`)
	if err == nil || !strings.Contains(err.Error(), `undefined variable "e"`) {
		t.Fatalf("got %v", err)
	}
}

func TestUserCannotRedefineErrorStruct(t *testing.T) {
	_, err := run(t, `
def struct Error { foo as int };
`)
	if err == nil || !strings.Contains(err.Error(), `"Error" is defined more than once`) {
		t.Fatalf("got %v", err)
	}
}

func TestThrowValueIsCopied(t *testing.T) {
	// Mutating the catch binding must not change the thrown source
	// (value semantics, like every other binding boundary).
	out, err := run(t, `
use io;
def err as Error init Error{kind: "k", message: "m", file: "", line: 0, col: 0};
try {
    throw $err;
} catch (caught) {
    $caught.kind = "mutated";
}
io.printf("source=%s\n", $err.kind);
`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "source=k\n" {
		t.Errorf("got %q", out)
	}
}
