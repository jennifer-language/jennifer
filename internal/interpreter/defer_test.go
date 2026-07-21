// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"errors"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// deferOut runs src and asserts no error, returning the captured output.
func deferOut(t *testing.T, src string) string {
	t.Helper()
	out, err := run(t, src)
	if err != nil {
		t.Fatalf("run: %v\noutput so far: %q", err, out)
	}
	return out
}

// Deferred calls run LIFO at block exit, after the body.
func TestDeferLIFOAtBlockExit(t *testing.T) {
	out := deferOut(t, `use io;
func f() { defer io.printf("a\n"); defer io.printf("b\n"); io.printf("body\n"); }
f();`)
	if out != "body\nb\na\n" {
		t.Errorf("got %q, want body/b/a", out)
	}
}

// A deferred call runs on an early return.
func TestDeferRunsOnReturn(t *testing.T) {
	out := deferOut(t, `use io;
func f() { defer io.printf("clean\n"); io.printf("x\n"); return; io.printf("unreached\n"); }
f();`)
	if out != "x\nclean\n" {
		t.Errorf("got %q", out)
	}
}

// Deferred calls run on break and continue, per loop-body iteration.
func TestDeferRunsOnBreakContinue(t *testing.T) {
	out := deferOut(t, `use io;
for (def i in [1, 2, 3]) {
	defer io.printf("d%d\n", $i);
	if ($i == 1) { continue; }
	if ($i == 2) { break; }
	io.printf("u%d\n", $i);
}`)
	// i=1: defer d1, continue -> d1. i=2: defer d2, break -> d2. i=3 never runs.
	if out != "d1\nd2\n" {
		t.Errorf("got %q, want d1/d2", out)
	}
}

// Arguments are evaluated at the defer site, not when the call runs.
func TestDeferArgsEvaluatedAtDeferSite(t *testing.T) {
	out := deferOut(t, `use io;
func f() { def x as int init 1; defer io.printf("was %d\n", $x); $x = 99; }
f();`)
	if out != "was 1\n" {
		t.Errorf("got %q, want 'was 1'", out)
	}
}

// A defer is block-scoped: it runs at the end of each loop iteration, not
// piled up to the method's end (contrast Go's function-scoped defer).
func TestDeferBlockScopedPerIteration(t *testing.T) {
	out := deferOut(t, `use io;
for (def i in [1, 2]) { io.printf("open %d\n", $i); defer io.printf("close %d\n", $i); }`)
	if out != "open 1\nclose 1\nopen 2\nclose 2\n" {
		t.Errorf("got %q", out)
	}
}

// A method's defers run at the method's exit and do not leak into the caller.
func TestDeferDoesNotCrossMethodBoundary(t *testing.T) {
	out := deferOut(t, `use io;
func inner() { defer io.printf("inner-clean\n"); io.printf("inner\n"); }
func outer() { inner(); io.printf("after\n"); }
outer();`)
	if out != "inner\ninner-clean\nafter\n" {
		t.Errorf("got %q", out)
	}
}

// A defer runs while a throw unwinds the frame, and the thrown value is caught.
func TestDeferRunsWhileThrowUnwinds(t *testing.T) {
	out := deferOut(t, `use io;
func f() { defer io.printf("clean\n"); throw "boom"; }
try { f(); } catch (e) { io.printf("caught %s\n", $e); }`)
	if out != "clean\ncaught boom\n" {
		t.Errorf("got %q", out)
	}
}

// An error from a deferred call propagates (and is catchable).
func TestDeferredCallErrorPropagates(t *testing.T) {
	out := deferOut(t, `use io;
func bad() { throw "defer-failed"; }
func f() { defer bad(); io.printf("body\n"); }
try { f(); } catch (e) { io.printf("caught %s\n", $e); }`)
	if out != "body\ncaught defer-failed\n" {
		t.Errorf("got %q", out)
	}
}

// A deferred call's error supersedes an error the body was already unwinding
// (Go's panic-in-defer rule).
func TestDeferredErrorSupersedesBodyError(t *testing.T) {
	out := deferOut(t, `use io;
func bad() { throw "F-defer"; }
func f() { defer bad(); throw "E-body"; }
try { f(); } catch (e) { io.printf("caught %s\n", $e); }`)
	if out != "caught F-defer\n" {
		t.Errorf("got %q, want the deferred error to win", out)
	}
}

// A top-level defer runs at program end, including when the program exits;
// the exit code is preserved (not superseded by the defer).
func TestDeferTopLevelRunsOnExit(t *testing.T) {
	out, err := run(t, `use io;
defer io.printf("top-clean\n");
io.printf("start\n");
exit 5;`)
	if out != "start\ntop-clean\n" {
		t.Errorf("output = %q, want start/top-clean", out)
	}
	var ex *interpreter.ExitSignal
	if !errors.As(err, &ex) {
		t.Fatalf("expected ExitSignal, got %v", err)
	}
	if ex.Code != 5 {
		t.Errorf("exit code = %d, want 5 (defer must not change it)", ex.Code)
	}
}

// Deferred calls across two resources release in reverse acquisition order.
func TestDeferMultiResourceLIFO(t *testing.T) {
	out := deferOut(t, `use io;
func closeR(n as string) { io.printf("close %s\n", $n); }
func work() { defer closeR("db"); defer closeR("file"); io.printf("work\n"); }
work();`)
	if out != "work\nclose file\nclose db\n" {
		t.Errorf("got %q", out)
	}
}

// A nested block's defer runs when that inner block exits, before the outer
// block's own defers.
func TestDeferNestedBlocks(t *testing.T) {
	out := deferOut(t, `use io;
func f() {
	defer io.printf("outer\n");
	if (true) { defer io.printf("inner\n"); io.printf("in-if\n"); }
	io.printf("after-if\n");
}
f();`)
	if out != "in-if\ninner\nafter-if\nouter\n" {
		t.Errorf("got %q", out)
	}
}

// `defer` requires a call; other expression forms are a parse error.
func TestDeferRequiresCall(t *testing.T) {
	for _, src := range []string{
		`func f() { defer 1 + 2; }`,
		`func f() { defer $x; }`,
		`func f() { defer "hi"; }`,
		`func f() { defer $xs[0]; }`,
	} {
		_, err := run(t, src)
		if err == nil {
			t.Errorf("expected a parse error for %q", src)
			continue
		}
		if !strings.Contains(err.Error(), "requires a function call") {
			t.Errorf("%q: error should explain defer needs a call, got %v", src, err)
		}
	}
}
