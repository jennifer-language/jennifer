// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package interpreter_test

import (
	"io"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	convertlib "jennifer-lang.dev/jennifer/internal/lib/convert"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	listslib "jennifer-lang.dev/jennifer/internal/lib/lists"
	mathlib "jennifer-lang.dev/jennifer/internal/lib/math"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// benchRun parses once, then re-runs the program on a fresh interpreter each
// iteration. b.ReportAllocs() surfaces allocations/op, which - unlike wall
// clock - is machine-independent, so it is the fair proxy for the GC pressure
// M21.12 targets. The workloads are call-heavy (recursive method frames) and
// block-heavy (per-iteration block frames), the two shapes that borrow and
// release Environment frames in the hot path.
func benchRun(b *testing.B, src string) {
	b.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		b.Fatalf("parse: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		in := interpreter.New()
		in.Out = io.Discard
		iolib.Install(in)
		convertlib.Install(in)
		mathlib.Install(in)
		listslib.Install(in)
		if err := in.Run(prog); err != nil {
			b.Fatalf("run: %v", err)
		}
	}
}

// TestFrameAllocationsStayLow is the CI guard for the per-frame allocation work:
// a call frame binds parameters and locals through the slot slice without
// touching the name map, and evalCall binds args without an intermediate slice,
// so a deeply recursive run allocates a near-constant handful of objects instead
// of ~2 per call. fib(20) makes ~13.5k calls; before this work that was ~27k
// allocs, so a 2000 ceiling catches any reintroduction of per-binding /
// per-call heap traffic with wide margin for one-time setup noise.
func TestFrameAllocationsStayLow(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts are dominated by race-detector shadow state under -race")
	}
	src := `
func fib(n as int) { if ($n < 2) { return $n; } return fib($n - 1) + fib($n - 2); }
def r as int init fib(20);
`
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	avg := testing.AllocsPerRun(5, func() {
		in := interpreter.New()
		in.Out = io.Discard
		iolib.Install(in)
		convertlib.Install(in)
		if err := in.Run(prog); err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	if avg > 2000 {
		t.Fatalf("fib(20) allocated %.0f objects/run; expected < 2000 (per-frame allocation regressed - a map write or per-call slice is back in the hot path)", avg)
	}
}

// Deeply recursive method calls: exercises borrowBlockEnv / releaseBlockEnv and
// per-frame parameter binding (DefineAt) at high volume.
func BenchmarkCallHeavyFib(b *testing.B) {
	benchRun(b, `
func fib(n as int) {
	if ($n < 2) { return $n; }
	return fib($n - 1) + fib($n - 2);
}
def r as int init fib(24);
`)
}

// A tight loop whose body opens a nested block each iteration: exercises block
// frame borrow/release without method-call overhead.
func BenchmarkBlockHeavyLoop(b *testing.B) {
	benchRun(b, `
def acc as int init 0;
for (def i as int init 0; $i < 200000; $i = $i + 1) {
	if ($i > 0) {
		def step as int init $i;
		$acc = $acc + $step;
	}
}
`)
}

// A method called in a tight loop: isolates the per-call frame cost (parameter
// binding + frame borrow/release) from recursion depth.
func BenchmarkCallInLoop(b *testing.B) {
	benchRun(b, `
func add(a as int, b as int) {
	def s as int init $a + $b;
	return $s;
}
def acc as int init 0;
for (def i as int init 0; $i < 200000; $i = $i + 1) {
	$acc = add($acc, $i);
}
`)
}
