// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"bytes"
	"errors"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
)

// A top-level `errdefer` in the REPL is skipped on `exit` (a deliberate
// termination, not an error), while a plain top-level `defer` still runs - the
// REPL teardown must match finishFrame / the documented semantics.
func TestErrdeferReplSkippedOnExit(t *testing.T) {
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	_, err := evalRepl(t, in, "", `use io;
errdefer io.printf("ERRDEFER\n");
defer io.printf("DEFER\n");
exit 0;`)
	var ex *interpreter.ExitSignal
	if !errors.As(err, &ex) {
		t.Fatalf("expected ExitSignal, got %v", err)
	}
	if got := buf.String(); got != "DEFER\n" {
		t.Errorf("REPL exit: got %q, want just the plain defer (DEFER), errdefer must NOT fire", got)
	}
}

// A top-level `errdefer` in the REPL fires when the input exits with a real
// propagating error (a throw).
func TestErrdeferReplFiresOnThrow(t *testing.T) {
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	_, err := evalRepl(t, in, "", `use io;
errdefer io.printf("ERRDEFER\n");
throw "boom";`)
	if err == nil {
		t.Fatal("expected the throw to propagate")
	}
	if got := buf.String(); got != "ERRDEFER\n" {
		t.Errorf("REPL throw: got %q, want ERRDEFER (errdefer fires on a real error)", got)
	}
}

// An errdefer is skipped when the block exits normally (fall-through and
// return alike) - the acquired resource stays live for the caller.
func TestErrdeferSkippedOnSuccess(t *testing.T) {
	out := deferOut(t, `use io;
func f() { errdefer io.printf("undo\n"); io.printf("body\n"); }
func g() { errdefer io.printf("undo-g\n"); return; }
f();
g();`)
	if out != "body\n" {
		t.Errorf("got %q, want body only (errdefer skipped on success)", out)
	}
}

// An errdefer runs while a throw unwinds its block, before the catch.
func TestErrdeferRunsOnThrow(t *testing.T) {
	out := deferOut(t, `use io;
func f() { errdefer io.printf("undo\n"); io.printf("body\n"); throw "boom"; }
try { f(); } catch (e) { io.printf("caught %s\n", $e); }`)
	if out != "body\nundo\ncaught boom\n" {
		t.Errorf("got %q, want body/undo/caught", out)
	}
}

// A runtime error (not just an explicit throw) triggers an errdefer.
func TestErrdeferRunsOnRuntimeError(t *testing.T) {
	out := deferOut(t, `use io;
func f() {
	errdefer io.printf("undo\n");
	def m as map of string to int init {"a": 1};
	io.printf("%d\n", $m["missing"]);
}
try { f(); } catch (e) { io.printf("caught\n"); }`)
	if out != "undo\ncaught\n" {
		t.Errorf("got %q, want undo/caught", out)
	}
}

// break / continue are normal control flow, not errors: errdefer skipped.
func TestErrdeferSkippedOnBreakContinue(t *testing.T) {
	out := deferOut(t, `use io;
for (def i in [1, 2, 3]) {
	errdefer io.printf("undo %d\n", $i);
	if ($i == 1) { continue; }
	if ($i == 2) { break; }
	io.printf("unreached\n");
}
io.printf("after\n");`)
	if out != "after\n" {
		t.Errorf("got %q, want after only", out)
	}
}

// `exit` is a deliberate termination, not an error: plain defers run, errdefers
// do not, and the exit code survives.
func TestErrdeferSkippedOnExit(t *testing.T) {
	out, err := run(t, `use io;
defer io.printf("clean\n");
errdefer io.printf("undo\n");
io.printf("start\n");
exit 5;`)
	if out != "start\nclean\n" {
		t.Errorf("output = %q, want start/clean (no undo)", out)
	}
	var ex *interpreter.ExitSignal
	if !errors.As(err, &ex) {
		t.Fatalf("expected ExitSignal, got %v", err)
	}
	if ex.Code != 5 {
		t.Errorf("exit code = %d, want 5", ex.Code)
	}
}

// defer and errdefer interleave in one LIFO teardown: on an error exit both
// kinds run, in reverse registration order.
func TestErrdeferInterleavesLIFO(t *testing.T) {
	out := deferOut(t, `use io;
func f() {
	defer io.printf("d-one\n");
	errdefer io.printf("e-two\n");
	defer io.printf("d-three\n");
	throw "boom";
}
try { f(); } catch (e) { io.printf("caught\n"); }`)
	if out != "d-three\ne-two\nd-one\ncaught\n" {
		t.Errorf("got %q, want d-three/e-two/d-one/caught", out)
	}
}

// On a success exit the same stack runs only the plain defers, keeping order.
func TestErrdeferInterleaveSkipsOnSuccess(t *testing.T) {
	out := deferOut(t, `use io;
func f() {
	defer io.printf("d-one\n");
	errdefer io.printf("e-two\n");
	defer io.printf("d-three\n");
}
f();`)
	if out != "d-three\nd-one\n" {
		t.Errorf("got %q, want d-three/d-one", out)
	}
}

// A plain defer that throws during teardown turns the exit into an error exit:
// an earlier-registered errdefer (still to run, LIFO) now fires.
func TestDeferErrorActivatesEarlierErrdefer(t *testing.T) {
	out := deferOut(t, `use io;
func bad() { io.printf("bad\n"); throw "teardown-boom"; }
func f() {
	errdefer io.printf("undo\n");
	defer bad();
	io.printf("body\n");
}
try { f(); } catch (e) { io.printf("caught %s\n", $e); }`)
	if out != "body\nbad\nundo\ncaught teardown-boom\n" {
		t.Errorf("got %q, want body/bad/undo/caught", out)
	}
}

// Arguments are evaluated at the errdefer site, like defer.
func TestErrdeferArgsEvaluatedAtSite(t *testing.T) {
	out := deferOut(t, `use io;
func f() { def x as int init 1; errdefer io.printf("was %d\n", $x); $x = 99; throw "boom"; }
try { f(); } catch (e) { io.printf("caught\n"); }`)
	if out != "was 1\ncaught\n" {
		t.Errorf("got %q, want 'was 1'/caught", out)
	}
}

// The canonical acquire/undo pattern: on a failed "handshake" the errdefer
// releases the resource; on success the resource survives the function.
func TestErrdeferConnectPattern(t *testing.T) {
	out := deferOut(t, `use io;
func connect(ok as bool) {
	io.printf("open\n");
	errdefer io.printf("close\n");
	if (not $ok) { throw "handshake failed"; }
	io.printf("ready\n");
}
try { connect(false); } catch (e) { io.printf("caught\n"); }
connect(true);`)
	if out != "open\nclose\ncaught\nopen\nready\n" {
		t.Errorf("got %q", out)
	}
}
