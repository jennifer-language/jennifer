// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

// Package compresslib is the `compress` library: byte-stream compression
// (gzip / zlib / raw DEFLATE), `bytes` in / `bytes` out, plus streaming
// compression via the integer-handle pattern `hash` uses (`compress.Stream
// {id}`). The algorithm is a string argument, matching `hash.compute(b,
// "sha-256")` / `crc.compute(b, "crc32")`; the `pack` / `unpack` verbs pair
// with `archive`'s (streams here, file bundles there). Distinct from
// `encoding` (reversible representation like base64); this is entropy-based
// size reduction. Backed by Go's TinyGo-clean compress/* packages.
package compresslib

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"sync"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// maxDecompressed caps a decompressed stream so a small "zip bomb" input cannot
// expand to gigabytes in memory. Fixed default (configurable later).
const maxDecompressed = 256 << 20

// readCapped reads r fully but errors past maxDecompressed rather than
// allocating without bound.
func readCapped(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxDecompressed+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxDecompressed {
		return nil, fmt.Errorf("decompressed size exceeds the %d-byte limit", maxDecompressed)
	}
	return data, nil
}

// LibraryName is the namespace prefix (`compress.`) and the `use` name.
const LibraryName = "compress"

// levelList / algoList are the rendered known-value strings for error messages.
const (
	levelList = `"fast", "default", "best"`
	algoList  = `"gzip", "zlib", "deflate"`
)

// compStream is one live streaming compressor: the algorithm's writer feeding
// an in-memory buffer. finalize closes the writer (flushing the trailer) and
// returns the buffer.
type compStream struct {
	mu  sync.Mutex // guards w/buf: spawned tasks sharing one handle must not race
	w   io.WriteCloser
	buf *bytes.Buffer
}

// streams holds live streaming state keyed by integer handle; finalize removes
// the entry so a later call errors. Mirrors hash's registry; a mutex guards it
// so two spawned tasks opening streams don't corrupt the map.
var (
	streamsMu sync.Mutex
	streams   = map[int64]*compStream{}
	nextID    int64
)

// Install registers the compress surface.
func Install(in *interpreter.Interpreter) {
	in.RegisterNamespacedStruct(LibraryName, "Stream", []parser.StructField{
		{Name: "id", Type: parser.PrimitiveType(parser.TypeInt)},
	})
	in.RegisterNamespaced(LibraryName, "pack", packFn)
	in.RegisterNamespaced(LibraryName, "unpack", unpackFn)
	in.RegisterNamespaced(LibraryName, "stream", streamFn)
	in.RegisterNamespaced(LibraryName, "update", updateFn)
	in.RegisterNamespaced(LibraryName, "finalize", finalizeFn)
	in.RegisterNamespaced(LibraryName, "discard", discardFn)
}

// writerFor builds a leveled compressor for algo over w.
func writerFor(algo string, w io.Writer, level int) (io.WriteCloser, error) {
	switch algo {
	case "gzip":
		return gzip.NewWriterLevel(w, level)
	case "zlib":
		return zlib.NewWriterLevel(w, level)
	case "deflate":
		return flate.NewWriter(w, level)
	}
	return nil, fmt.Errorf("unknown algorithm %q; known: %s", algo, algoList)
}

// readerFor builds a decompressor for algo over r.
func readerFor(algo string, r io.Reader) (io.ReadCloser, error) {
	switch algo {
	case "gzip":
		return gzip.NewReader(r)
	case "zlib":
		return zlib.NewReader(r)
	case "deflate":
		return flate.NewReader(r), nil
	}
	return nil, fmt.Errorf("unknown algorithm %q; known: %s", algo, algoList)
}

// levelFor reads the optional level argument at args[idx]. Absent -> default
// compression; otherwise a string in {fast, default, best}.
func levelFor(fnName string, args []interpreter.Value, idx int) (int, error) {
	if len(args) <= idx {
		return flate.DefaultCompression, nil
	}
	if args[idx].Kind != interpreter.KindString {
		return 0, fmt.Errorf("%s: level must be string, got %s", fnName, args[idx].Kind)
	}
	switch args[idx].Str {
	case "fast":
		return flate.BestSpeed, nil
	case "default":
		return flate.DefaultCompression, nil
	case "best":
		return flate.BestCompression, nil
	}
	return 0, fmt.Errorf("%s: unknown level %q; known: %s", fnName, args[idx].Str, levelList)
}

// packFn implements `compress.pack(bytes, algo [, level]) -> bytes`.
func packFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return interpreter.Null(), fmt.Errorf("compress.pack expects 2 or 3 arguments (bytes, algo[, level]), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("compress.pack: first argument must be bytes, got %s", args[0].Kind)
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("compress.pack: algo must be string, got %s", args[1].Kind)
	}
	level, err := levelFor("compress.pack", args, 2)
	if err != nil {
		return interpreter.Null(), err
	}
	var buf bytes.Buffer
	w, err := writerFor(args[1].Str, &buf, level)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("compress.pack: %v", err)
	}
	if _, err := w.Write(args[0].Bytes); err != nil {
		w.Close()
		return interpreter.Null(), fmt.Errorf("compress.pack: %v", err)
	}
	if err := w.Close(); err != nil {
		return interpreter.Null(), fmt.Errorf("compress.pack: %v", err)
	}
	return interpreter.BytesVal(buf.Bytes()), nil
}

// unpackFn implements `compress.unpack(bytes, algo) -> bytes`.
func unpackFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("compress.unpack expects 2 arguments (bytes, algo), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("compress.unpack: first argument must be bytes, got %s", args[0].Kind)
	}
	if args[1].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("compress.unpack: algo must be string, got %s", args[1].Kind)
	}
	r, err := readerFor(args[1].Str, bytes.NewReader(args[0].Bytes))
	if err != nil {
		return interpreter.Null(), fmt.Errorf("compress.unpack: %v", err)
	}
	out, err := readCapped(r)
	r.Close()
	if err != nil {
		return interpreter.Null(), fmt.Errorf("compress.unpack: %v", err)
	}
	return interpreter.BytesVal(out), nil
}

// makeStream / extractStreamID mirror hash's handle helpers.
func makeStream(id int64) interpreter.Value {
	return interpreter.NamespacedStructVal(LibraryName, "Stream", []interpreter.StructField{
		{Name: "id", Value: interpreter.IntVal(id)},
	})
}

func extractStreamID(fnName string, v interpreter.Value) (int64, error) {
	if v.Kind != interpreter.KindStruct || v.StructNS != LibraryName || v.StructName != "Stream" {
		return 0, fmt.Errorf("%s: argument must be a compress.Stream, got %s", fnName, v.Kind)
	}
	for _, f := range v.Fields {
		if f.Name == "id" {
			if f.Value.Kind != interpreter.KindInt {
				return 0, fmt.Errorf("%s: compress.Stream.id is not int (got %s)", fnName, f.Value.Kind)
			}
			return f.Value.Int, nil
		}
	}
	return 0, fmt.Errorf("%s: compress.Stream has no id field", fnName)
}

// streamFn implements `compress.stream(algo[, level]) -> compress.Stream`.
func streamFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return interpreter.Null(), fmt.Errorf("compress.stream expects 1 or 2 arguments (algo[, level]), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("compress.stream: algo must be string, got %s", args[0].Kind)
	}
	level, err := levelFor("compress.stream", args, 1)
	if err != nil {
		return interpreter.Null(), err
	}
	var buf bytes.Buffer
	w, err := writerFor(args[0].Str, &buf, level)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("compress.stream: %v", err)
	}
	streamsMu.Lock()
	nextID++
	id := nextID
	streams[id] = &compStream{w: w, buf: &buf}
	streamsMu.Unlock()
	return makeStream(id), nil
}

// updateFn feeds one chunk of uncompressed bytes into a live stream.
func updateFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 {
		return interpreter.Null(), fmt.Errorf("compress.update expects 2 arguments (stream, bytes), got %d", len(args))
	}
	id, err := extractStreamID("compress.update", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	if args[1].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("compress.update: second argument must be bytes, got %s", args[1].Kind)
	}
	streamsMu.Lock()
	st, ok := streams[id]
	streamsMu.Unlock()
	if !ok {
		return interpreter.Null(), fmt.Errorf("compress.update: stream %d has already been finalized or never existed", id)
	}
	st.mu.Lock()
	_, werr := st.w.Write(args[1].Bytes)
	st.mu.Unlock()
	if werr != nil {
		return interpreter.Null(), fmt.Errorf("compress.update: %v", werr)
	}
	return interpreter.Null(), nil
}

// finalizeFn closes the compressor (flushing the trailer), returns the full
// compressed output, and removes the handle.
func finalizeFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("compress.finalize expects 1 argument (stream), got %d", len(args))
	}
	id, err := extractStreamID("compress.finalize", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	streamsMu.Lock()
	st, ok := streams[id]
	if ok {
		delete(streams, id)
	}
	streamsMu.Unlock()
	if !ok {
		return interpreter.Null(), fmt.Errorf("compress.finalize: stream %d has already been finalized or never existed", id)
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if err := st.w.Close(); err != nil {
		return interpreter.Null(), fmt.Errorf("compress.finalize: %v", err)
	}
	out := st.buf.Bytes()
	return interpreter.BytesVal(out), nil
}

// discardFn drops a live stream without flushing/returning its output, closing
// the underlying writer to release its state. The abort path for a stream
// opened but abandoned before finalize.
func discardFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 1 {
		return interpreter.Null(), fmt.Errorf("compress.discard expects 1 argument (stream), got %d", len(args))
	}
	id, err := extractStreamID("compress.discard", args[0])
	if err != nil {
		return interpreter.Null(), err
	}
	streamsMu.Lock()
	st, ok := streams[id]
	if ok {
		delete(streams, id)
	}
	streamsMu.Unlock()
	if !ok {
		return interpreter.Null(), fmt.Errorf("compress.discard: stream %d has already been finalized or never existed", id)
	}
	st.mu.Lock()
	st.w.Close() // release the writer's buffers; the output is intentionally dropped
	st.mu.Unlock()
	return interpreter.Null(), nil
}
