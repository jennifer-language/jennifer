// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

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

// TestPackReusesWritersCleanly packs different inputs in turn so pooled
// compressors are reused, and checks each output is the same as from a fresh
// writer: no state from the previous input may leak into the next.
func TestPackReusesWritersCleanly(t *testing.T) {
	inputs := []string{strings.Repeat("first input ", 200), "x", "", strings.Repeat("abc", 1000)}
	for _, algo := range []string{"gzip", "zlib", "deflate"} {
		for _, level := range []string{"fast", "default", "best"} {
			want := make([][]byte, len(inputs))
			for i, in := range inputs {
				want[i] = pack(t, b(in), str(algo), str(level)).Bytes
			}
			for round := 0; round < 3; round++ {
				for i, in := range inputs {
					got := pack(t, b(in), str(algo), str(level))
					if string(got.Bytes) != string(want[i]) {
						t.Fatalf("%s/%s round %d input %d: output differs from the first pack", algo, level, round, i)
					}
					if dec := unpack(t, got, str(algo)); string(dec.Bytes) != in {
						t.Fatalf("%s/%s round %d input %d: round-trip got %q", algo, level, round, i, dec.Bytes)
					}
				}
			}
		}
	}
}

// TestPackConcurrent packs from several goroutines at once (run under -race).
func TestPackConcurrent(t *testing.T) {
	data := strings.Repeat("concurrent ", 500)
	done := make(chan string, 8)
	for g := 0; g < 8; g++ {
		go func() {
			for i := 0; i < 50; i++ {
				v, err := packFn(interpreter.BuiltinCtx{}, []interpreter.Value{b(data), str("deflate")})
				if err != nil {
					done <- err.Error()
					return
				}
				out, err := unpackFn(interpreter.BuiltinCtx{}, []interpreter.Value{v, str("deflate")})
				if err != nil || string(out.Bytes) != data {
					done <- "round-trip mismatch"
					return
				}
			}
			done <- ""
		}()
	}
	for g := 0; g < 8; g++ {
		if msg := <-done; msg != "" {
			t.Fatal(msg)
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

// unpack's optional maxBytes stops decompression one byte past the cap and
// raises a catchable Error{kind: "limit"} naming it; output that fits is
// returned whole, and a bad maxBytes is an argument error.
func TestUnpackMaxBytes(t *testing.T) {
	data := strings.Repeat("a", 1000)
	for _, algo := range []string{"gzip", "zlib", "deflate"} {
		comp := pack(t, b(data), str(algo))
		if got := unpack(t, comp, str(algo), interpreter.IntVal(1000)); string(got.Bytes) != data {
			t.Errorf("%s: output at exactly maxBytes should be returned whole", algo)
		}
		_, err := unpackFn(interpreter.BuiltinCtx{}, []interpreter.Value{comp, str(algo), interpreter.IntVal(999)})
		sig, ok := err.(*interpreter.ErrorSignal)
		if !ok {
			t.Fatalf("%s: over the cap want *ErrorSignal, got %T %v", algo, err, err)
		}
		if k := errorField(sig.Value, "kind"); k != "limit" {
			t.Errorf("%s: kind = %q, want limit", algo, k)
		}
		if m := errorField(sig.Value, "message"); !strings.Contains(m, "999-byte limit") {
			t.Errorf("%s: message %q should name the caller's limit", algo, m)
		}
	}
	comp := pack(t, b(data), str("zlib"))
	for _, bad := range []interpreter.Value{interpreter.IntVal(0), interpreter.IntVal(-1), str("10")} {
		if _, err := unpackFn(interpreter.BuiltinCtx{}, []interpreter.Value{comp, str("zlib"), bad}); err == nil {
			t.Errorf("maxBytes %v should be rejected", bad)
		}
	}
}

func errorField(v interpreter.Value, name string) string {
	for _, f := range v.Fields {
		if f.Name == name {
			return f.Value.Str
		}
	}
	return ""
}
