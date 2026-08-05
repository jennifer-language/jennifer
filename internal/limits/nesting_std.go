// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build !tinygo

// Package limits holds the shared resource caps that every recursive-descent
// parser in the tree (the language parser and the json / toml / xml decoders)
// enforces so that deeply-nested untrusted input cannot exhaust the Go stack.
//
// The value is build-tag split because the two binaries have very different
// stacks. The interpreter has no recover(): a Go stack overflow is a fatal,
// uncatchable crash, so the cap must sit below the depth at which the
// tree-walker's per-level frames overflow the stack, not merely "beyond any
// real document".
package limits

// MaxNestingDepth caps structural nesting (containers, grouped expressions) in
// every recursive-descent parser. The default binary uses Go's growable stack
// (up to ~1 GB), so 1000 is far below any crash point and leaves ample room for
// even a pathologically deep serialized document.
const MaxNestingDepth = 1000

// MaxCallDepth caps the number of nested Jennifer method calls the interpreter
// will execute before raising a positioned, catchable runtime error - the
// analogue of Python's RecursionError. Unbounded recursion otherwise grows the
// Go goroutine stack until the runtime's ~1 GB ceiling triggers a fatal,
// uncatchable "stack overflow". The default binary crashes a heavy method body
// (many locals, nested blocks) near 50k nested calls and a minimal one near
// 100k; 10000 sits well below the tighter floor with room for even heavier
// frames, while still allowing an order of magnitude more recursion than a
// typical tree-walking language (CPython defaults to 1000).
const MaxCallDepth = 10000

// MaxRangeElements caps how many elements a range expression will materialise
// into a `list of int` in one evaluation - the value forms `0..n` and
// `$xs = 0..n`, not the lazy `for (def i in 0..n)` iteration, which allocates
// nothing and is unbounded. Like MaxCallDepth, its job is to convert a fatal,
// uncatchable failure into a positioned, catchable error: a bad or
// attacker-controlled bound would otherwise reach `make([]Value, 0, n)` with a
// huge (or, on int64 span overflow, negative) capacity and trigger Go's
// "makeslice: cap out of range" panic - which the interpreter, having no
// recover(), cannot catch - or a multi-gigabyte single allocation just below
// it. The default binary allows ~16.7M ints, far past any reasonable
// materialised range yet well below the allocation cliff; a larger span should
// iterate lazily.
const MaxRangeElements = 1 << 24

// MaxChannelCapacity caps the buffer a single channel.make can allocate, in the
// same spirit as MaxRangeElements: a buffered `chan Value` (Value is ~272 bytes)
// allocates its whole buffer eagerly, so an unbounded capacity gives program-
// (or attacker-)controlled input a fresh multi-gigabyte single-allocation path -
// which, below Go's makechan "size out of range" panic, is a real commit that
// OOM-kills the process (a fatal, unrecoverable failure the makeFn recover cannot
// catch). At 1<<20 elements a buffer is ~285 MiB on the default binary - far past
// any real channel use, yet well below the multi-GB allocation cliff. A larger
// capacity is a catchable error.
const MaxChannelCapacity = 1 << 20
