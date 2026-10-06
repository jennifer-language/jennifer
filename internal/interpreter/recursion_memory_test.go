// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter

import (
	"bytes"
	"strings"
	"sync/atomic"
	"testing"

	"jennifer-lang.dev/jennifer/internal/parser"
)

// The recursion memory guard (MaxCopyChainBytes) converts an otherwise
// uncatchable OOM - a large value copied on each of many recursive frames,
// which exhausts the heap long before the frame-count depth cap fires - into the
// same catchable "limit" runtime error as plain deep recursion. These tests
// lower the byte budget so the guard fires without allocating a real gigabyte.

func runMem(t *testing.T, src string) error {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	in := New()
	var buf bytes.Buffer
	in.Out = &buf
	return in.Run(prog)
}

func withBudget(t *testing.T, n int64) {
	t.Helper()
	saved := copyChainBudget
	copyChainBudget = n
	t.Cleanup(func() { copyChainBudget = saved })
}

// A recursion that copies a large value on every frame tops the budget and
// raises a catchable "limit" (kind), rather than crashing the process. `mine` is
// written, so it is a real per-frame copy (not a borrowed alias); depth stays
// far under MaxCallDepth, so only the memory guard can stop it.
func TestRecursionMemoryGuardTripsCatchably(t *testing.T) {
	withBudget(t, 1<<20) // 1 MiB
	src := `
		func down(n as int, held as list of int) {
			if ($n <= 0) { return 0; }
			def mine as list of int init $held;
			$mine[0] = $n;
			return down($n - 1, $mine);
		}
		def big as list of int init 0..2000;
		def r as int init down(500, $big);
	`
	err := runMem(t, src)
	if err == nil {
		t.Fatal("expected a limit error, got nil (guard did not fire)")
	}
	re, ok := err.(*runtimeError)
	if !ok {
		t.Fatalf("expected *runtimeError, got %T: %v", err, err)
	}
	if re.Kind != "limit" {
		t.Fatalf("error kind = %q, want %q", re.Kind, "limit")
	}
	if !strings.Contains(re.Msg, "too much memory") {
		t.Fatalf("message = %q, want it to mention memory", re.Msg)
	}
}

// The same shape with a read-only `mine` (never written) borrows its
// initializer instead of copying, so it adds no per-frame heap and does NOT trip
// the memory guard even at a tiny budget - proving the accounting counts real
// copies, not aliases.
func TestRecursionMemoryGuardIgnoresBorrowedReads(t *testing.T) {
	withBudget(t, 1<<20) // 1 MiB
	src := `
		func down(n as int, held as list of int) {
			if ($n <= 0) { return 0; }
			def mine as list of int init $held;
			return len($mine) + down($n - 1, $held);
		}
		def big as list of int init 0..2000;
		def r as int init down(500, $big);
	`
	if err := runMem(t, src); err != nil {
		t.Fatalf("borrowed-read recursion must not trip the memory guard: %v", err)
	}
}

// The counter tracks live memory, not cumulative: after a copy-recursion that
// completes well under the budget, every frame has been released, so the
// goroutine-root counter is back to just the top-level globals' contribution (no
// drift that would falsely trip a later call).
func TestRecursionMemoryCounterBalances(t *testing.T) {
	withBudget(t, 1<<30) // generous: the recursion completes
	src := `
		func down(n as int, held as list of int) {
			if ($n <= 0) { return 0; }
			def mine as list of int init $held;
			$mine[0] = $n;
			return down($n - 1, $mine);
		}
		def big as list of int init 0..100;
		def r as int init down(200, $big);
	`
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	in := New()
	var buf bytes.Buffer
	in.Out = &buf
	if err := in.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	// Only the two top-level globals (`big`, the 100-element list; `r`, a scalar)
	// remain live on the root; the 200 recursive frames each copied a 100-element
	// list and all have been released. The residual must be a single list's worth,
	// not 200 of them.
	live := atomic.LoadInt64(in.global.chainBytes)
	oneList := int64(100) * valueCellBytes
	if live > 3*oneList {
		t.Fatalf("counter did not balance: live=%d bytes, want <= %d (a drift of ~%d frames)",
			live, 3*oneList, live/oneList)
	}
}
