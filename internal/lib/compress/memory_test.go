// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package compresslib

import (
	"runtime"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// allocBytes reports the bytes allocated while fn runs.
func allocBytes(fn func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// TestStreamCapBoundsMemory: each open stream holds a DEFLATE compressor of up
// to ~1 MiB, so the open-stream cap keeps leaked streams within MaxAllocBytes
// (it was 4096, about 4 GB of compressors).
func TestStreamCapBoundsMemory(t *testing.T) {
	var opened []interpreter.Value
	defer func() {
		for _, s := range opened {
			_, _ = discardFn(interpreter.BuiltinCtx{}, []interpreter.Value{s})
		}
	}()
	var err error
	for i := 0; i <= maxStreams; i++ {
		var s interpreter.Value
		if s, err = streamFn(interpreter.BuiltinCtx{}, []interpreter.Value{str("deflate"), str("fast")}); err != nil {
			break
		}
		opened = append(opened, s)
	}
	if err == nil || !strings.Contains(err.Error(), "too many open streams") {
		t.Fatalf("opened %d streams without the cap firing (err %v)", len(opened), err)
	}
	if len(opened) > 256 { // ~1 MiB per compressor
		t.Errorf("cap allows %d streams, over 256 MiB of compressors", len(opened))
	}
}

// TestGzipUnpackPresized: gzip unpack sizes its output from the size trailer,
// so a large result costs about one copy, not io.ReadAll's growth copies.
func TestGzipUnpackPresized(t *testing.T) {
	const size = 16 << 20
	p := pack(t, interpreter.BytesVal(make([]byte, size)), str("gzip"))
	var got interpreter.Value
	n := allocBytes(func() { got = unpack(t, p, str("gzip")) })
	if len(got.Bytes) != size {
		t.Fatalf("unpacked %d bytes, want %d", len(got.Bytes), size)
	}
	if n > size*3/2 {
		t.Errorf("unpack allocated %d bytes for a %d-byte result, want under 1.5x", n, size)
	}
	// A forged trailer only sizes the buffer: the data still decides.
	forged := append([]byte(nil), pack(t, b("short"), str("gzip")).Bytes...)
	forged[len(forged)-4], forged[len(forged)-3] = 0xff, 0xff
	if _, err := unpackFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.BytesVal(forged), str("gzip")}); err == nil {
		t.Error("gzip with a forged size trailer: want gzip's size-mismatch error")
	}
}

// TestUnpackReusesDecompressors: unpack takes decompressors from a pool rather
// than building a ~40 KB one per call.
func TestUnpackReusesDecompressors(t *testing.T) {
	for _, algo := range []string{"gzip", "zlib", "deflate"} {
		p := pack(t, b("a small payload, a small payload"), str(algo))
		unpack(t, p, str(algo)) // warm the pool
		const calls = 50
		n := allocBytes(func() {
			for i := 0; i < calls; i++ {
				unpack(t, p, str(algo))
			}
		})
		if per := n / calls; per > 8<<10 {
			t.Errorf("%s: unpack allocated %d bytes per call, want under 8 KiB", algo, per)
		}
	}
}
