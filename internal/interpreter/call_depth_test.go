// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

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
	if !strings.Contains(out, "kind=limit") {
		t.Fatalf("depth error should present as kind=limit (resource exhaustion), got: %q", out)
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

// TestCallDepthUnderLimitRuns proves recursion whose depth stays comfortably
// below the cap completes normally. The budget counts block and expression
// nesting as well as calls (so a `rec` body spends several budget units per
// level, not one); this recurses to a depth whose total budget use stays well
// under MaxCallDepth so it must succeed.
func TestCallDepthUnderLimitRuns(t *testing.T) {
	// Each rec level costs a handful of budget units (the call, the body block,
	// the `if` block, the operand descents); MaxCallDepth/8 levels stays safely
	// under the cap for any realistic per-level cost.
	depth := limits.MaxCallDepth / 8
	src := fmt.Sprintf(`use io;
func rec(n as int) { if ($n <= 0) { return 0; } return rec($n - 1); }
def r as int init rec(%d);
io.printf("ok=%%d\n", $r);`, depth)
	out, err := run(t, src)
	if err != nil {
		t.Fatalf("recursion well under the cap should run, got error: %v", err)
	}
	if !strings.Contains(out, "ok=0") {
		t.Fatalf("expected ok=0, got: %q", out)
	}
}

// TestCallDepthCountsBlockNesting proves recursion through a body with several
// nested blocks trips the catchable guard instead of overflowing the Go stack:
// the depth budget counts block entry, not just the call.
func TestCallDepthCountsBlockNesting(t *testing.T) {
	out, err := run(t, `use io;
func f(n as int) {
    if (true) { if (true) { if (true) { if (true) { if (true) {
    if (true) { if (true) { if (true) { if (true) { if (true) {
        return f($n + 1);
    } } } } } } } } } }
    return 0;
}
def caught as bool init false;
try { def r as int init f(0); } catch (e) { $caught = true; io.printf("kind=%s\n", $e.kind); }
io.printf("survived=%t\n", $caught);`)
	if err != nil {
		t.Fatalf("deeply-nested-block recursion should be catchable, got error: %v", err)
	}
	if !strings.Contains(out, "survived=true") || !strings.Contains(out, "kind=limit") {
		t.Fatalf("expected catchable kind=limit, got: %q", out)
	}
}

// TestCallDepthCountsExpressionNesting proves a recursive call wrapped in many
// parentheses trips the catchable guard: the budget counts expression-operand
// descent, so the stack cannot overflow at a call depth far below the call cap.
func TestCallDepthCountsExpressionNesting(t *testing.T) {
	// 200 nested `(1 + ...)` around the recursive call; parses (under the
	// expression-nesting cap) but each frame carries 200 expression frames, so
	// unbounded recursion would overflow the Go stack without expression counting.
	open := strings.Repeat("(1 + ", 200)
	closeP := strings.Repeat(")", 200)
	src := `use io;
func f(n as int) { return ` + open + `f($n + 1)` + closeP + `; }
def caught as bool init false;
try { def r as int init f(0); } catch (e) { $caught = true; io.printf("kind=%s\n", $e.kind); }
io.printf("survived=%t\n", $caught);`
	out, err := run(t, src)
	if err != nil {
		t.Fatalf("paren-wrapped recursion should be catchable, got error: %v", err)
	}
	if !strings.Contains(out, "survived=true") || !strings.Contains(out, "kind=limit") {
		t.Fatalf("expected catchable kind=limit, got: %q", out)
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
