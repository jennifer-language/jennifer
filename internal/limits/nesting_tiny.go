// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build tinygo

package limits

// MaxNestingDepth caps structural nesting for jennifer-tiny, whose goroutine
// stack is a fixed 4 MB (see the Makefile's -stack-size and the TinyGo notes in
// docs/technical/tinygo.md). At the earlier 2 MB the tree-walker overflowed on
// the heaviest shape - a deeply-nested map literal - between depth 96 (survives)
// and 128 (segfaults); the 4 MB stack roughly doubles that floor, so 64 sits far
// below it with ample margin for shape variance, and is still an order of
// magnitude past the point where nesting is a code smell.
const MaxNestingDepth = 64

// MaxCallDepth caps nested Jennifer method calls for jennifer-tiny. A method
// call is far heavier per level than a single nesting step - the tree-walker
// stacks many Go frames per Jennifer call - so the stack overflows much sooner
// than for structural nesting. On the 4 MB stack a minimal recursive body
// segfaults near depth 100, a fib-shaped or heavy one (several locals, nested
// blocks) near depth 75. 48 sits well below that floor while clearing the
// deepest recursion a shipped example reaches with room to spare
// (examples/benchmark.j's serial fib(23) peaks at depth 24), turning what is
// otherwise an uncatchable SIGSEGV into a positioned, catchable error. It is
// deliberately lower than MaxNestingDepth: call frames cost more stack than the
// single deep expression chain the nesting cap governs.
const MaxCallDepth = 48

// MaxRangeElements caps range-literal materialisation for jennifer-tiny, whose
// heap is far smaller than the default binary's. It turns the same otherwise
// uncatchable "makeslice: cap out of range" panic (or an oversized allocation)
// on `0..n` into a positioned, catchable error. 1<<20 (~1M ints) stays modest
// on a constrained target while covering any realistic materialised range;
// larger spans should use the lazy `for (def i in 0..n)` form, which allocates
// nothing. See the std-build note for the full rationale.
const MaxRangeElements = 1 << 20

// MaxChannelCapacity caps a single channel.make buffer on jennifer-tiny, whose
// heap is far smaller than the default binary's, so the ceiling is lower: 1<<16
// elements of a ~272-byte Value is ~17 MiB, generous for a constrained target yet
// well below an allocation that would exhaust it. A larger capacity is a catchable
// error. See the std-build note for the full rationale.
const MaxChannelCapacity = 1 << 16

// MaxMatrixElements caps how many elements a single `linalg` vector or matrix may
// hold on jennifer-tiny, whose heap is far smaller than the default binary's, so
// the ceiling is lower. Like its std sibling it is enforced at both the
// constructors and the readers, and sized in 272-byte Value cells (a `linalg`
// value is always fully materialised, like a channel buffer): 1<<16 elements is
// ~17 MiB of Value, matching MaxChannelCapacity's tiny budget, generous for a
// constrained target yet well below an allocation that would exhaust it. A larger
// vector or matrix is a catchable error. See the std-build note for the full
// rationale.
const MaxMatrixElements = 1 << 16
