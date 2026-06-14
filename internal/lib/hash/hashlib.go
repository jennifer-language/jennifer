// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

// Package hashlib implements Jennifer's `hash` library: MD5, SHA-1,
// and SHA-256 digests over `bytes`, plus a streaming API for inputs
// that don't fit in memory. Output is raw `bytes` - hex / base64
// encoding lives in the future `encoding` library (M15.7) so the
// verb names don't multiply across libraries (stance #1).
//
// The library follows the codec-table shape already used by
// `convert.bytesFromString(s, "utf-8")` and the planned
// `encoding.encode(s, codec)`: one function per category, with the
// algorithm passed as a string. This dodges Jennifer's letters-only
// identifier rule (which would reject `hash.md5` because `5` is a
// digit) and keeps the public verb count small.
//
// Streaming uses the integer-handle pattern from `oslib` (M15.3):
// the Jennifer side sees an opaque `hash.Stream {id as int}` struct
// while the real Go `hash.Hash` state lives in a package-scope map
// keyed by `id`. `hash.finalize($stream)` removes the entry so
// further calls error cleanly.
//
// The Go package is named hashlib to avoid colliding with Go's
// standard `hash` package, which this implementation depends on.
package hashlib

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	gohash "hash"

	"github.com/mplx/jennifer-lang/internal/interpreter"
	"github.com/mplx/jennifer-lang/internal/parser"
)

// LibraryName is the namespace prefix and `use` name.
const LibraryName = "hash"

// algoCtor maps a Jennifer-side algorithm string to a `hash.Hash`
// constructor. The set of known algorithms is the keys.
var algoCtor = map[string]func() gohash.Hash{
	"md5":    md5.New,
	"sha1":   sha1.New,
	"sha256": sha256.New,
}

// algoList is the rendered "known algorithms" string used in error
// messages. Kept in a known order so the message stays stable.
const algoList = `"md5", "sha1", "sha256"`

// streams holds live streaming hash state keyed by integer handle.
// `hash.finalize` removes the entry so further calls error.
var (
	streams = map[int64]gohash.Hash{}
	nextID  int64
)

// Install registers the M15.6 hash surface.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespacedStruct(LibraryName, "Stream", []parser.StructField{
		{Name: "id", Type: parser.PrimitiveType(parser.TypeInt)},
	})

	in.RegisterNamespaced(LibraryName, "compute", computeFn)
	in.RegisterNamespaced(LibraryName, "stream", streamFn)
	in.RegisterNamespaced(LibraryName, "update", updateFn)
	in.RegisterNamespaced(LibraryName, "finalize", finalizeFn)
}

// makeStream builds the Jennifer-side `hash.Stream{id}` value.
func makeStream(id int64) interpreter.Value {
	return interpreter.NamespacedStructVal(LibraryName, "Stream", []interpreter.StructField{
		{Name: "id", Value: interpreter.IntVal(id)},
	})
}

// extractStreamID pulls the id field out of a `hash.Stream`.
func extractStreamID(fnName string, v interpreter.Value) (int64, error) {
	if v.Kind != interpreter.KindStruct || v.StructNS != LibraryName || v.StructName != "Stream" {
		return 0, fmt.Errorf("%s: argument must be a hash.Stream, got %s", fnName, v.Kind)
	}
	for _, f := range v.Fields {
		if f.Name == "id" {
			if f.Value.Kind != interpreter.KindInt {
				return 0, fmt.Errorf("%s: hash.Stream.id is not int (got %s)", fnName, f.Value.Kind)
			}
			return f.Value.Int, nil
		}
	}
	return 0, fmt.Errorf("%s: hash.Stream has no id field", fnName)
}

// computeFn implements `hash.compute(bytes, algo) -> bytes`. One-shot
// digest of the full input.
func computeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("hash.compute expects 2 arguments (bytes, algo), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("hash.compute: first argument must be bytes, got %s", args[0].Kind)
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("hash.compute: algo must be string, got %s", args[1].Kind)
	}
	ctor, ok := algoCtor[args[1].Str]
	if !ok {
		return interpreter.Null(), fmt.Errorf("hash.compute: unknown digest algorithm %q; known: %s", args[1].Str, algoList)
	}
	h := ctor()
	h.Write(args[0].Bytes)
	return interpreter.BytesVal(h.Sum(nil)), nil
}

// streamFn implements `hash.stream(algo) -> hash.Stream`.
func streamFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("hash.stream expects 1 argument (algo), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("hash.stream: algo must be string, got %s", args[0].Kind)
	}
	ctor, ok := algoCtor[args[0].Str]
	if !ok {
		return interpreter.Null(), fmt.Errorf("hash.stream: unknown digest algorithm %q; known: %s", args[0].Str, algoList)
	}
	nextID++
	id := nextID
	streams[id] = ctor()
	return makeStream(id), nil
}

// updateFn feeds one chunk of bytes into a live stream. Returns null;
// the stream's state is mutated by side effect, and the Jennifer
// struct only carries the id so the caller keeps using the same
// handle.
func updateFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("hash.update expects 2 arguments (stream, bytes), got %d", len(args))
	}
	id, err := extractStreamID("hash.update", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("hash.update: second argument must be bytes, got %s", args[1].Kind)
	}
	h, ok := streams[id]
	if !ok {
		return interpreter.Null(), fmt.Errorf("hash.update: stream %d has already been finalized or never existed", id)
	}
	h.Write(args[1].Bytes)
	return interpreter.Null(), nil
}

// finalizeFn computes the final digest, removes the handle from the
// live map, and returns the digest as bytes.
func finalizeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("hash.finalize expects 1 argument (stream), got %d", len(args))
	}
	id, err := extractStreamID("hash.finalize", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	h, ok := streams[id]
	if !ok {
		return interpreter.Null(), fmt.Errorf("hash.finalize: stream %d has already been finalized or never existed", id)
	}
	digest := h.Sum(nil)
	delete(streams, id)
	return interpreter.BytesVal(digest), nil
}

// resetForTest clears the live-stream map and id counter so tests
// run in isolation. Test-only.
func resetForTest() {
	streams = map[int64]gohash.Hash{}
	nextID = 0
}
