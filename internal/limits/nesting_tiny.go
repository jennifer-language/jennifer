// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

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
