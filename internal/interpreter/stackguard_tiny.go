// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build tinygo

package interpreter

import _ "unsafe" // go:linkname

// stackPointer is TinyGo's runtime.getCurrentStackPointer (an LLVM stacksave),
// present on every architecture TinyGo targets.
//
//go:linkname stackPointer runtime.getCurrentStackPointer
func stackPointer() uintptr

// maxStackBytes is how much of jennifer-tiny's fixed 4 MB goroutine stack
// (-stack-size=4mb) a call chain may use before a call raises the catchable
// call-depth "limit" error. TinyGo's tree-walker frames are large (evalExpr and
// execStmtRaw take about 25 KB each, LLVM merging their single-call-site
// handlers into them), so a heavy recursive body passed 4 MB well below
// MaxCallDepth's 120 units and TinyGo, which does not trap an overflow, went
// on with corrupted memory (M28.72). The remaining 1 MB covers what runs
// between two call checks (nesting bounded by MaxNestingDepth).
const maxStackBytes = 3 << 20

// stackTooDeep reports whether the chain has used more than maxStackBytes of
// stack. The base is the stack pointer at the chain's first call check, taken
// on the goroutine that runs it; a pointer above the base re-anchors it, and a
// pointer more than a whole stack below it means another goroutine's stack, so
// it re-anchors too rather than trip.
func stackTooDeep(dc *CallChain) bool {
	sp := stackPointer()
	if dc.stackBase == 0 || sp > dc.stackBase || dc.stackBase-sp > 4<<20 {
		dc.stackBase = sp
		return false
	}
	return dc.stackBase-sp > maxStackBytes
}
