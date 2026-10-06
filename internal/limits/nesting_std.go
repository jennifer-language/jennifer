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

// MaxCallDepth caps interpreter recursion before a positioned, catchable runtime
// error is raised - the analogue of Python's RecursionError. Unbounded recursion
// otherwise grows the Go goroutine stack to the runtime's ~1 GB ceiling and
// triggers a fatal, uncatchable "stack overflow". The budget counts every major
// tree-walker recursion step - a method call, a block entry (execBlock), and an
// expression-operand descent (evalExprDeep) - so the cap bounds real Go-stack
// depth regardless of how a frame mixes recursion with per-frame nesting; a
// call-only count let ~10 nested blocks, or a call wrapped in many parens,
// overflow well below it. Block and expression steps increment without their own
// check (bounded by the parser's stmtDepth / exprDepth caps, they only run away
// through a checked call). A simple recursion reaches a few thousand levels here
// and peaks near 120 MB of stack. Bounds the stack; the heap a recursion holds
// in per-frame value copies is bounded separately by MaxCopyChainBytes.
const MaxCallDepth = 10000

// MaxCopyChainBytes caps the estimated heap held by the value copies live along
// one call chain - the companion to MaxCallDepth for the heap, not the stack.
// The depth cap bounds frame *count*, but a frame's cost is unbounded: a
// recursion that copies a large value per frame (a parser carrying its token
// list, a tree-walker holding a node list) exhausts memory long before the
// frame count reaches MaxCallDepth, and a Go OOM is a fatal, uncatchable crash
// the interpreter has no recover() to trap. Each store that allocates a new
// compound backing (a copying parameter bind, a copying local `def`) adds its
// estimated size (shallowValueBytes) to a goroutine-local counter shared by
// every live frame, and the counter drops as each frame is released; crossing
// this budget at a call site raises the same catchable "limit" error as the
// depth cap. The accounting is deliberately crude and conservative (it
// undercounts nested backing and ignores non-recursive single-frame growth), so
// it never stops a legal program early; its job is only to convert the
// uncatchable kill into the catchable error that already exists. At 1 GiB the
// budget is far above any reasonable recursive working set yet below the OOM
// cliff on a typical host; a deeper machine can carry more real memory than this
// in globals, which is why a larger single value or a non-recursive loop is not
// bounded here (that is the host's own memory limit's job).
const MaxCopyChainBytes = 1 << 30

// MaxDecodedNodes caps how many value nodes one decode of untrusted text may
// materialise, shared by the hand-rolled json / xml / toml decoders and the
// yaml converter. A node is an interpreter.Value (~272 bytes) plus container
// slice-growth transients, so it costs closer to ~1 KB at the decode peak; a
// few-MB document would otherwise amplify into gigabytes and an uncatchable OOM
// (only nesting depth was bounded before). At 1<<19 (~524k nodes) the decode
// peaks at a few hundred MB - far past any real document yet well below the OOM
// cliff - and exceeding it is a catchable "too many nodes" error. (asn1 keeps
// its own lower 200k cap.)
const MaxDecodedNodes = 1 << 19

// MaxRangeElements caps how many elements a range expression will materialise
// into a `list of int` in one evaluation - the value forms `0..n` and
// `$xs = 0..n`, not the lazy `for (def i in 0..n)` iteration, which allocates
// nothing and is unbounded. Like MaxCallDepth, its job is to convert a fatal,
// uncatchable failure into a positioned, catchable error: a bad or
// attacker-controlled bound would otherwise reach `make([]Value, 0, n)` with a
// huge (or, on int64 span overflow, negative) capacity and trigger Go's
// "makeslice: cap out of range" panic - which the interpreter, having no
// recover(), cannot catch - or a multi-gigabyte single allocation just below
// it. At 1<<22 (~4.2M ints) the materialised list is ~1.1 GB of Value (272 bytes
// each), far past any reasonable materialised range yet bounded and catchable;
// a larger span should iterate lazily with `for (def i in lo..hi)`, which
// allocates nothing. lists.range shares this cap.
const MaxRangeElements = 1 << 22

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

// MaxMatrixElements caps how many elements a single `linalg` vector or matrix
// may hold - enforced both where a constructor (`identity(n)`, `zeros(rows,
// cols)`) sizes its allocation from an integer argument and where the `vector` /
// `matrix` readers accept an operation's input, so every entry point is bounded.
// Its sibling is MaxChannelCapacity, not MaxRangeElements: a `linalg` value is
// *always fully materialised* as a tree of interpreter Values (each 272 bytes),
// exactly like a channel buffer and unlike a range, whose common form is lazy and
// allocates nothing. So the budget is sized in Value cells, not raw float64s:
// `identity(100000)` would otherwise reach `make` for a ~10^10-cell matrix (~2.7
// TB of Value, a fatal uncatchable OOM the interpreter has no recover() to trap),
// and even a 4096x4096 matrix is ~4.5 GiB of Value (~9 GiB after the value-
// semantics store-boundary copy), not the ~128 MiB a float64-per-cell reading
// would suggest. At 1<<20 the materialised tree is ~285 MiB (matching
// MaxChannelCapacity's budget), a 1024x1024 matrix sits at the ceiling - far past
// any real dense-matrix use in a tree-walker - and a larger vector or matrix is a
// catchable error. The cap also transitively bounds the O(n^3) routines (matmul /
// inverse / solve): with n <= 1024 the worst case is ~1e9 operations, seconds of
// CPU, not an unbounded stall.
const MaxMatrixElements = 1 << 20

// MaxTokens caps how many tokens a single source file lexes into, bounding the
// front of the pipeline against a "token bomb": a small, dense file (a few MB of
// `1+1+1+1;`) tokenises to millions of ~72-byte Token structs, ~100x the input
// byte count, before the parser then walks that same array and multiplies the
// footprint again. The sibling caps (MaxNestingDepth, MaxRangeElements) bound
// depth and single allocations; this bounds the token *stream*. It complements
// the preprocessor's 2M spliced-token budget, which counts tokens across
// `include` splices but does NOT count the entry file's own tokens - so a giant
// root program with no includes would otherwise be lexed (and parsed) unbounded.
// 1<<22 (~4.2M tokens, ~300 MiB of Token) is generous for any real or generated
// program (a 1 MB source is well under 1M tokens) yet well below the multi-GB
// territory a bomb reaches; a larger file is a positioned, catchable lex error.
const MaxTokens = 1 << 22

// MaxSourceBytes caps how many bytes a single source file the preprocessor reads
// (an `include`d file) may hold, checked with a stat before the whole file is
// read into memory. MaxTokens bounds the token stream, but a file that is huge
// yet not token-dense (megabytes of one string literal or comment) would still be
// slurped whole by os.ReadFile before tokenising; this bounds that raw read. At
// 64 MiB it never rejects a real source file (a `.j` file is KB to low MB) yet
// stops an accidental gigabyte include from committing its bytes; a larger file
// is a positioned, catchable preprocess error.
const MaxSourceBytes = 64 << 20
