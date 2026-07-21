// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	tasklib "jennifer-lang.dev/jennifer/internal/lib/task"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// The call-depth guard turns unbounded Jennifer recursion - which would
// otherwise grow the Go goroutine stack until a fatal, uncatchable "stack
// overflow" - into a positioned, catchable runtime error before the stack is
// exhausted. These tests pin that it fires, that it is catchable, that it
// counts across methods (not per-method), that legitimate recursion just below
// the cap still runs, and that a spawn body is guarded on its own goroutine.

// TestCallDepthOverLimitRaisesCatchableError proves runaway self-recursion
// surfaces as an ordinary returned error (not a crash) carrying the
// call-stack-too-deep message, positioned at the recursive call site.
func TestCallDepthOverLimitRaisesCatchableError(t *testing.T) {
	_, err := run(t, `func rec(n as int) { return rec($n + 1); }
def r as int init rec(1);`)
	if err == nil {
		t.Fatal("expected a call-stack-too-deep error, got nil")
	}
	if !strings.Contains(err.Error(), "call stack too deep") {
		t.Fatalf("error is not the depth guard: %v", err)
	}
}

// TestCallDepthIsCatchable proves try/catch catches the depth error so a
// program can recover from runaway recursion instead of dying.
func TestCallDepthIsCatchable(t *testing.T) {
	out, err := run(t, `use io;
func rec(n as int) { return rec($n + 1); }
def caught as bool init false;
try { def r as int init rec(1); } catch (e) {
    $caught = true;
    io.printf("kind=%s\n", $e.kind);
}
io.printf("survived=%t\n", $caught);`)
	if err != nil {
		t.Fatalf("program should survive via catch, got error: %v", err)
	}
	if !strings.Contains(out, "survived=true") {
		t.Fatalf("expected the catch to run, got: %q", out)
	}
	if !strings.Contains(out, "kind=runtime") {
		t.Fatalf("depth error should present as kind=runtime, got: %q", out)
	}
}

// TestCallDepthCountsAcrossMethods proves the counter is a true call-stack
// depth, not a per-method recursion count: a ping/pong mutual recursion trips
// the same guard.
func TestCallDepthCountsAcrossMethods(t *testing.T) {
	_, err := run(t, `func ping(n as int) { return pong($n + 1); }
func pong(n as int) { return ping($n + 1); }
def r as int init ping(1);`)
	if err == nil || !strings.Contains(err.Error(), "call stack too deep") {
		t.Fatalf("mutual recursion should trip the depth guard, got: %v", err)
	}
}

// TestCallDepthUnderLimitRuns proves recursion whose depth stays at or below
// the cap completes normally - the guard fires strictly above the limit.
func TestCallDepthUnderLimitRuns(t *testing.T) {
	// rec(N) nests N+1 calls (rec(N)..rec(0)); N = MaxCallDepth-1 peaks at
	// exactly MaxCallDepth, which is allowed (the guard fires only above it).
	src := fmt.Sprintf(`use io;
func rec(n as int) { if ($n <= 0) { return 0; } return rec($n - 1); }
def r as int init rec(%d);
io.printf("ok=%%d\n", $r);`, limits.MaxCallDepth-1)
	out, err := run(t, src)
	if err != nil {
		t.Fatalf("recursion at the cap should run, got error: %v", err)
	}
	if !strings.Contains(out, "ok=0") {
		t.Fatalf("expected ok=0, got: %q", out)
	}
}

// TestCallDepthGuardsSpawnBody proves a spawn body is depth-guarded on its own
// goroutine: runaway recursion inside spawn is re-raised at task.wait and is
// catchable there, rather than segfaulting the goroutine.
func TestCallDepthGuardsSpawnBody(t *testing.T) {
	src := `use io;
use task;
func rec(n as int) { return rec($n + 1); }
def t as task of int init spawn { return rec(1); };
def caught as bool init false;
try { def r as int init task.wait($t); } catch (e) { $caught = true; }
io.printf("caught=%t\n", $caught);`
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	tasklib.Install(in)
	if err := in.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(buf.String(), "caught=true") {
		t.Fatalf("spawn-body depth error should be catchable at wait, got: %q", buf.String())
	}
}
