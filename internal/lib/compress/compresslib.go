// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

// Package compresslib is the `compress` library: byte-stream compression
// (gzip / zlib / raw DEFLATE), `bytes` in / `bytes` out, plus streaming
// compression via the integer-handle pattern `hash` uses (`compress.Stream
// {id}`). The algorithm is a string first argument, matching
// `hash.compute("sha256", b)` / `crc.compute("crc32", b)`; the `pack` /
// `unpack` verbs pair with `archive`'s (streams here, file bundles there).
// Distinct from `encoding` (reversible representation like base64); this is
// entropy-based size reduction. Backed by Go's TinyGo-clean compress/*
// packages.
package compresslib

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// maxDecompressed is unpack's default output cap, so a small "zip bomb" input
// cannot expand to gigabytes in memory: limits.MaxAllocBytes (256 MiB, 32 MiB
// on jennifer-tiny). A caller lowers (or raises) it with unpack's optional
// maxBytes argument.
const maxDecompressed = limits.MaxAllocBytes

// maxDeflateRatio bounds how far DEFLATE can expand its input (about 1032:1), so
// a forged gzip size trailer cannot make unpack preallocate more than the input
// could produce.
const maxDeflateRatio = 1032

// capError marks an output-cap breach so unpack can raise it as
// Error{kind: "limit"}, as archive does, rather than a generic runtime error.
type capError struct{ limit int64 }

func (e *capError) Error() string {
	return fmt.Sprintf("decompressed size exceeds the %d-byte limit", e.limit)
}

// readCapped reads r fully but stops one byte past limit and errors rather
// than allocating without bound.
// A limit of MaxInt64 reads to EOF (limits.ReadAll saturates the +1). hint is
// the expected output size (0 = unknown): the first min(hint, limit) bytes go
// into one buffer of that size, avoiding io.ReadAll's doubling copies; a wrong
// hint costs only that allocation.
func readCapped(r io.Reader, limit, hint int64) ([]byte, error) {
	var head []byte
	if hint > 0 {
		head = make([]byte, min(hint, limit))
		k := 0
		for k < len(head) {
			m, err := r.Read(head[k:])
			k += m
			if err == io.EOF {
				return head[:k], nil
			}
			if err != nil {
				return nil, err
			}
		}
		limit -= int64(k)
	}
	data, err := limits.ReadAll(r, limit)
	if errors.Is(err, limits.ErrTooLarge) {
		return nil, &capError{limit + int64(len(head))}
	}
	if err != nil {
		return nil, err
	}
	if head == nil {
		return data, nil
	}
	return append(head, data...), nil
}

// gzipSizeHint reads the ISIZE trailer of a gzip member (the uncompressed size
// mod 2^32) as a preallocation hint, bounded by what the input can expand to.
// 0 when unknown.
func gzipSizeHint(b []byte) int64 {
	if len(b) < 18 { // 10-byte header + 8-byte trailer
		return 0
	}
	n := int64(binary.LittleEndian.Uint32(b[len(b)-4:]))
	return min(n, int64(len(b))*maxDeflateRatio, limits.MaxAllocBytes)
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
// maxStreams bounds the live stream registry so a script that leaks streams
// (opens without a matching finalize / discard) surfaces a catchable error
// instead of growing the in-memory registry without limit. Each stream holds a
// DEFLATE compressor of up to about streamCost bytes, so the cap keeps leaked
// compressors within limits.MaxAllocBytes: 256 streams, 32 on jennifer-tiny.
const (
	streamCost = 1 << 20
	maxStreams = limits.MaxAllocBytes / streamCost
)

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

// readerPools holds idle decompressors for unpack, one pool per algorithm:
// a flate decompressor carries ~40 KB of state that Reset keeps.
var readerPools = map[string]*sync.Pool{"gzip": {}, "zlib": {}, "deflate": {}}

// acquireReader returns a decompressor for algo over r, reusing an idle one
// from its pool when there is one.
func acquireReader(algo string, r io.Reader) (io.ReadCloser, error) {
	p := readerPools[algo]
	if p == nil {
		return nil, fmt.Errorf("unknown algorithm %q; known: %s", algo, algoList)
	}
	if pooled := p.Get(); pooled != nil {
		var err error
		switch d := pooled.(type) {
		case *gzip.Reader:
			err = d.Reset(r)
		case flate.Resetter:
			err = d.Reset(r, nil)
		}
		if err != nil {
			return nil, err
		}
		return pooled.(io.ReadCloser), nil
	}
	switch algo {
	case "gzip":
		return gzip.NewReader(r)
	case "zlib":
		return zlib.NewReader(r)
	default:
		return flate.NewReader(r), nil
	}
}

// releaseReader closes a decompressor and returns it to its pool, pointed at an
// empty reader so the pool does not keep the caller's input alive.
func releaseReader(algo string, rc io.ReadCloser) {
	_ = rc.Close()
	switch d := rc.(type) {
	case *gzip.Reader:
		_ = d.Reset(bytes.NewReader(nil))
	case flate.Resetter:
		_ = d.Reset(bytes.NewReader(nil), nil)
	}
	if p := readerPools[algo]; p != nil {
		p.Put(rc)
	}
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

// packFn implements `compress.pack(algo, bytes [, level]) -> bytes`.
func packFn(_ interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return interpreter.Null(), fmt.Errorf("compress.pack expects 2 or 3 arguments (algo, bytes[, level]), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("compress.pack: algo must be string, got %s", args[0].Kind)
	}
	if args[1].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("compress.pack: second argument must be bytes, got %s", args[1].Kind)
	}
	level, err := levelFor("compress.pack", args, 2)
	if err != nil {
		return interpreter.Null(), err
	}
	var buf bytes.Buffer
	algo := args[0].Str
	w, err := acquireWriter(algo, &buf, level)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("compress.pack: %v", err)
	}
	if _, err := w.Write(args[1].Bytes); err != nil {
		w.Close()
		return interpreter.Null(), fmt.Errorf("compress.pack: %v", err)
	}
	if err := w.Close(); err != nil {
		return interpreter.Null(), fmt.Errorf("compress.pack: %v", err)
	}
	releaseWriter(algo, level, w)
	return interpreter.BytesVal(buf.Bytes()), nil
}

// resetWriter is a compressor that can be pointed at a new destination and
// reused: gzip.Writer, zlib.Writer and flate.Writer all are.
type resetWriter interface {
	io.WriteCloser
	Reset(w io.Writer)
}

// poolKey selects one writer pool: compressor state depends on the algorithm and
// the level, so writers are only reused for the same pair.
type poolKey struct {
	algo  string
	level int
}

// writerPools holds idle compressors for pack. Building a DEFLATE compressor
// allocates its whole window and hash tables (several hundred KiB), which
// dominated a small pack; Reset keeps those buffers. Every (algo, level) pair
// writerFor accepts has a pool, so the map is read-only after init.
var writerPools = func() map[poolKey]*sync.Pool {
	m := map[poolKey]*sync.Pool{}
	for _, algo := range []string{"gzip", "zlib", "deflate"} {
		for _, level := range []int{flate.BestSpeed, flate.DefaultCompression, flate.BestCompression} {
			m[poolKey{algo, level}] = &sync.Pool{}
		}
	}
	return m
}()

// acquireWriter returns a compressor for algo at level writing to w, reusing an
// idle one from its pool when there is one.
func acquireWriter(algo string, w io.Writer, level int) (resetWriter, error) {
	if p := writerPools[poolKey{algo, level}]; p != nil {
		if rw, ok := p.Get().(resetWriter); ok {
			rw.Reset(w)
			return rw, nil
		}
	}
	wc, err := writerFor(algo, w, level)
	if err != nil {
		return nil, err
	}
	return wc.(resetWriter), nil
}

// releaseWriter returns a compressor to its pool after a successful Close. A
// writer that failed is dropped instead, since its state is not known to reset.
func releaseWriter(algo string, level int, rw resetWriter) {
	rw.Reset(io.Discard)
	if p := writerPools[poolKey{algo, level}]; p != nil {
		p.Put(rw)
	}
}

// unpackFn implements `compress.unpack(algo, bytes[, maxBytes]) -> bytes`.
func unpackFn(ctx interpreter.BuiltinCtx, args []interpreter.Value) (interpreter.Value, error) {
	if len(args) != 2 && len(args) != 3 {
		return interpreter.Null(), fmt.Errorf("compress.unpack expects 2 or 3 arguments (algo, bytes[, maxBytes]), got %d", len(args))
	}
	if args[0].Kind != interpreter.KindString {
		return interpreter.Null(), fmt.Errorf("compress.unpack: algo must be string, got %s", args[0].Kind)
	}
	if args[1].Kind != interpreter.KindBytes {
		return interpreter.Null(), fmt.Errorf("compress.unpack: second argument must be bytes, got %s", args[1].Kind)
	}
	limit := int64(maxDecompressed)
	if len(args) == 3 {
		if args[2].Kind != interpreter.KindInt {
			return interpreter.Null(), fmt.Errorf("compress.unpack: maxBytes must be int, got %s", args[2].Kind)
		}
		if args[2].Int < 1 {
			return interpreter.Null(), fmt.Errorf("compress.unpack: maxBytes must be at least 1, got %d", args[2].Int)
		}
		limit = args[2].Int
	}
	algo := args[0].Str
	r, err := acquireReader(algo, bytes.NewReader(args[1].Bytes))
	if err != nil {
		return interpreter.Null(), fmt.Errorf("compress.unpack: %v", err)
	}
	var hint int64
	if algo == "gzip" {
		hint = gzipSizeHint(args[1].Bytes)
	}
	out, err := readCapped(r, limit, hint)
	releaseReader(algo, r)
	if err != nil {
		var ce *capError
		if errors.As(err, &ce) {
			return interpreter.Null(), interpreter.RaiseError("limit", "compress.unpack: "+ce.Error(), ctx.File, ctx.Line, ctx.Col)
		}
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
	if len(streams) >= maxStreams {
		streamsMu.Unlock()
		return interpreter.Null(), fmt.Errorf("compress.stream: too many open streams (limit %d); each needs a matching compress.finalize or discard", maxStreams)
	}
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
