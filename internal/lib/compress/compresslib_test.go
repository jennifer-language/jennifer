// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package compresslib

import (
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func b(s string) interpreter.Value   { return interpreter.BytesVal([]byte(s)) }
func str(s string) interpreter.Value { return interpreter.StringVal(s) }

// pack / unpack call the builtins the way the interpreter would.
func pack(t *testing.T, args ...interpreter.Value) interpreter.Value {
	t.Helper()
	v, err := packFn(interpreter.BuiltinCtx{}, args)
	if err != nil {
		t.Fatalf("pack%v: %v", args, err)
	}
	return v
}
func unpack(t *testing.T, args ...interpreter.Value) interpreter.Value {
	t.Helper()
	v, err := unpackFn(interpreter.BuiltinCtx{}, args)
	if err != nil {
		t.Fatalf("unpack%v: %v", args, err)
	}
	return v
}

func TestRoundTripAll(t *testing.T) {
	data := "the quick brown fox the quick brown fox the quick brown fox"
	for _, algo := range []string{"gzip", "zlib", "deflate"} {
		comp := pack(t, b(data), str(algo))
		if comp.Kind != interpreter.KindBytes {
			t.Fatalf("%s: pack result is %s, want bytes", algo, comp.Kind)
		}
		dec := unpack(t, comp, str(algo))
		if string(dec.Bytes) != data {
			t.Errorf("%s round-trip: got %q", algo, dec.Bytes)
		}
	}
}

func TestGzipMagicAndShrinks(t *testing.T) {
	data := strings.Repeat("abcabcabc", 300)
	out := pack(t, b(data), str("gzip"))
	if len(out.Bytes) < 2 || out.Bytes[0] != 0x1f || out.Bytes[1] != 0x8b {
		t.Errorf("gzip output missing magic 1f 8b: %x", out.Bytes)
	}
	if len(out.Bytes) >= len(data) {
		t.Errorf("compressible data did not shrink: %d -> %d", len(data), len(out.Bytes))
	}
}

func TestLevels(t *testing.T) {
	data := strings.Repeat("abcabc", 400)
	fast := pack(t, b(data), str("gzip"), str("fast"))
	best := pack(t, b(data), str("gzip"), str("best"))
	if len(best.Bytes) > len(fast.Bytes) {
		t.Errorf("best (%d) should not exceed fast (%d)", len(best.Bytes), len(fast.Bytes))
	}
	if _, err := packFn(interpreter.BuiltinCtx{}, []interpreter.Value{b(data), str("gzip"), str("turbo")}); err == nil {
		t.Error("expected an error for an unknown level")
	}
}

func TestStreamingMatchesOneShot(t *testing.T) {
	s, err := streamFn(interpreter.BuiltinCtx{}, []interpreter.Value{str("gzip")})
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range []string{"hello ", "world"} {
		if _, err := updateFn(interpreter.BuiltinCtx{}, []interpreter.Value{s, b(chunk)}); err != nil {
			t.Fatal(err)
		}
	}
	out, err := finalizeFn(interpreter.BuiltinCtx{}, []interpreter.Value{s})
	if err != nil {
		t.Fatal(err)
	}
	dec := unpack(t, out, str("gzip"))
	if string(dec.Bytes) != "hello world" {
		t.Errorf("streamed content = %q, want %q", dec.Bytes, "hello world")
	}
	// finalize consumes the handle: a second finalize / update errors.
	if _, err := finalizeFn(interpreter.BuiltinCtx{}, []interpreter.Value{s}); err == nil {
		t.Error("expected an error finalizing an already-finalized stream")
	}
	if _, err := updateFn(interpreter.BuiltinCtx{}, []interpreter.Value{s, b("x")}); err == nil {
		t.Error("expected an error updating a finalized stream")
	}
}

func TestErrors(t *testing.T) {
	// unknown algorithm in each entry point
	if _, err := packFn(interpreter.BuiltinCtx{}, []interpreter.Value{b("x"), str("lzma")}); err == nil {
		t.Error("pack: unknown algorithm should error")
	}
	if _, err := unpackFn(interpreter.BuiltinCtx{}, []interpreter.Value{b("x"), str("lzma")}); err == nil {
		t.Error("unpack: unknown algorithm should error")
	}
	if _, err := streamFn(interpreter.BuiltinCtx{}, []interpreter.Value{str("lzma")}); err == nil {
		t.Error("stream: unknown algorithm should error")
	}
	// decompressing garbage
	if _, err := unpackFn(interpreter.BuiltinCtx{}, []interpreter.Value{b("not gzip at all"), str("gzip")}); err == nil {
		t.Error("invalid gzip input should error")
	}
	// wrong kinds / arity
	if _, err := packFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(5), str("gzip")}); err == nil {
		t.Error("non-bytes input should error")
	}
	if _, err := packFn(interpreter.BuiltinCtx{}, []interpreter.Value{b("x")}); err == nil {
		t.Error("missing algo should error")
	}
	// stream-handle validation
	if _, err := extractStreamID("compress.update", interpreter.IntVal(1)); err == nil {
		t.Error("non-Stream handle should error")
	}
}
