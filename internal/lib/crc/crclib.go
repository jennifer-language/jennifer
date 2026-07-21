// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package crclib implements Jennifer's `crc` library: CRC-32 (IEEE
// polynomial) and CRC-64 (ECMA polynomial) over `bytes`, plus a
// streaming API matching `hash`'s shape. Output is the raw
// big-endian digest as `bytes` (4 bytes for CRC-32, 8 bytes for
// CRC-64) - hex / base64 encoding belongs to the future `encoding`
// library.
//
// CRCs are not cryptographic primitives; they live in their own
// library so the difference between "checksum for transport
// integrity" and "digest for content addressing" is obvious at the
// import line.
//
// Like `hash`, this library uses the codec-table shape: one verb
// per category with the algorithm as a string argument.
package crclib

import (
	"fmt"
	gohash "hash"
	"hash/crc32"
	"hash/crc64"
	"sync"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// LibraryName is the namespace prefix and `use` name.
const LibraryName = "crc"

// crc64Table is the singleton ECMA table shared by the one-shot and
// streaming paths.
var crc64Table = crc64.MakeTable(crc64.ECMA)

// algoSpec describes one supported algorithm: how to build a fresh
// stream and how wide the digest output is (so the one-shot path
// can emit big-endian bytes of the right size without consulting
// the live state).
type algoSpec struct {
	newStream func() gohash.Hash
	width     int
}

var algoSpecs = map[string]algoSpec{
	"crc32": {newStream: func() gohash.Hash { return crc32.NewIEEE() }, width: 4},
	"crc64": {newStream: func() gohash.Hash { return crc64.New(crc64Table) }, width: 8},
}

// algoList is the rendered "known algorithms" string used in error
// messages. Kept stable so the messages are deterministic.
const algoList = `"crc32", "crc64"`

// Live streaming state. Cleared per-entry on `crc.finalize`.
var (
	streamsMu sync.Mutex // guards streams + nextID (spawned tasks share the registry)
	streams   = map[int64]*streamEntry{}
	nextID    int64
)

// streamEntry carries its own mutex: the registry map is guarded by streamsMu,
// but the checksum state is mutable and two spawned tasks sharing one handle
// would race on the Go hash.Hash internals without a per-stream lock.
type streamEntry struct {
	mu    sync.Mutex
	h     gohash.Hash
	width int
}

// Install registers the crc surface.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespacedStruct(LibraryName, "Stream", []parser.StructField{
		{Name: "id", Type: parser.PrimitiveType(parser.TypeInt)},
	})

	in.RegisterNamespaced(LibraryName, "compute", computeFn)
	in.RegisterNamespaced(LibraryName, "stream", streamFn)
	in.RegisterNamespaced(LibraryName, "update", updateFn)
	in.RegisterNamespaced(LibraryName, "finalize", finalizeFn)
	in.RegisterNamespaced(LibraryName, "discard", discardFn)
}

func makeStream(id int64) interpreter.Value {
	return interpreter.NamespacedStructVal(LibraryName, "Stream", []interpreter.StructField{
		{Name: "id", Value: interpreter.IntVal(id)},
	})
}

func extractStreamID(fnName string, v interpreter.Value) (int64, error) {
	if v.Kind != interpreter.KindStruct || v.StructNS != LibraryName || v.StructName != "Stream" {
		return 0, fmt.Errorf("%s: argument must be a crc.Stream, got %s", fnName, v.Kind)
	}
	for _, f := range v.Fields {
		if f.Name == "id" {
			if f.Value.Kind != interpreter.KindInt {
				return 0, fmt.Errorf("%s: crc.Stream.id is not int (got %s)", fnName, f.Value.Kind)
			}
			return f.Value.Int, nil
		}
	}
	return 0, fmt.Errorf("%s: crc.Stream has no id field", fnName)
}

// computeFn implements `crc.compute(bytes, algo) -> bytes`. Renders
// the digest big-endian (network byte order); the alternative is to
// force the user to remember which.
func computeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("crc.compute expects 2 arguments (bytes, algo), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("crc.compute: first argument must be bytes, got %s", args[0].Kind)
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("crc.compute: algo must be string, got %s", args[1].Kind)
	}
	spec, ok := algoSpecs[args[1].Str]
	if !ok {
		return interpreter.Null(), fmt.Errorf("crc.compute: unknown algorithm %q; known: %s", args[1].Str, algoList)
	}
	h := spec.newStream()
	h.Write(args[0].Bytes)
	out := h.Sum(nil)
	// h.Sum already returns big-endian bytes of the natural width for
	// Go's crc32/crc64 helpers, but assert the width as a safety net.
	if len(out) != spec.width {
		out = padToWidth(out, spec.width)
	}
	return interpreter.BytesVal(out), nil
}

// padToWidth zero-pads `b` on the left to `width` bytes (big-endian
// safety net; Go's CRC types always return their natural width).
func padToWidth(b []byte, width int) []byte {
	if len(b) >= width {
		return b
	}
	out := make([]byte, width)
	copy(out[width-len(b):], b)
	return out
}

func streamFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("crc.stream expects 1 argument (algo), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("crc.stream: algo must be string, got %s", args[0].Kind)
	}
	spec, ok := algoSpecs[args[0].Str]
	if !ok {
		return interpreter.Null(), fmt.Errorf("crc.stream: unknown algorithm %q; known: %s", args[0].Str, algoList)
	}
	streamsMu.Lock()
	nextID++
	id := nextID
	streams[id] = &streamEntry{h: spec.newStream(), width: spec.width}
	streamsMu.Unlock()
	return makeStream(id), nil
}

func updateFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("crc.update expects 2 arguments (stream, bytes), got %d", len(args))
	}
	id, err := extractStreamID("crc.update", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("crc.update: second argument must be bytes, got %s", args[1].Kind)
	}
	streamsMu.Lock()
	entry, ok := streams[id]
	streamsMu.Unlock()
	if !ok {
		return interpreter.Null(), fmt.Errorf("crc.update: stream %d has already been finalized or never existed", id)
	}
	entry.mu.Lock()
	entry.h.Write(args[1].Bytes)
	entry.mu.Unlock()
	return interpreter.Null(), nil
}

func finalizeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("crc.finalize expects 1 argument (stream), got %d", len(args))
	}
	id, err := extractStreamID("crc.finalize", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	streamsMu.Lock()
	entry, ok := streams[id]
	if ok {
		delete(streams, id)
	}
	streamsMu.Unlock()
	if !ok {
		return interpreter.Null(), fmt.Errorf("crc.finalize: stream %d has already been finalized or never existed", id)
	}
	entry.mu.Lock()
	digest := entry.h.Sum(nil)
	entry.mu.Unlock()
	if len(digest) != entry.width {
		digest = padToWidth(digest, entry.width)
	}
	return interpreter.BytesVal(digest), nil
}

// discardFn drops a live stream without computing its checksum, releasing its
// state. The abort path for a stream opened but abandoned before finalize.
func discardFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("crc.discard expects 1 argument (stream), got %d", len(args))
	}
	id, err := extractStreamID("crc.discard", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	streamsMu.Lock()
	_, ok := streams[id]
	delete(streams, id)
	streamsMu.Unlock()
	if !ok {
		return interpreter.Null(), fmt.Errorf("crc.discard: stream %d has already been finalized or never existed", id)
	}
	return interpreter.Null(), nil
}

// resetForTest clears the live-stream map and id counter. Test-only.
func resetForTest() {
	streamsMu.Lock()
	streams = map[int64]*streamEntry{}
	nextID = 0
	streamsMu.Unlock()
}
