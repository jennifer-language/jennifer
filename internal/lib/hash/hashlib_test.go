// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package hashlib

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/mplx/jennifer-lang/internal/interpreter"
)

// compute is a Go-side convenience wrapper around computeFn so the
// table-driven tests below stay tidy.
func compute(t *testing.T, in []byte, algo string) []byte {
	t.Helper()
	v, err := computeFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		interpreter.BytesVal(in),
		interpreter.StringVal(algo),
	})
	if err != nil {
		t.Fatalf("compute(%q): %v", algo, err)
	}
	if v.Kind != interpreter.KindBytes {
		t.Fatalf("compute(%q): expected KindBytes, got %s", algo, v.Kind)
	}
	return v.Bytes
}

// TestComputeKnownVectors pins each algorithm against its canonical
// published digest so a regression in the underlying Go crypto
// package or in the wrapping shows up immediately.
func TestComputeKnownVectors(t *testing.T) {
	vectors := []struct {
		algo, in, hex string
	}{
		{"md5", "", "d41d8cd98f00b204e9800998ecf8427e"},
		{"md5", "abc", "900150983cd24fb0d6963f7d28e17f72"},
		{"sha1", "", "da39a3ee5e6b4b0d3255bfef95601890afd80709"},
		{"sha1", "abc", "a9993e364706816aba3e25717850c26c9cd0d89d"},
		{"sha256", "", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"sha256", "abc", "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
	}
	for _, v := range vectors {
		got := compute(t, []byte(v.in), v.algo)
		want, err := hex.DecodeString(v.hex)
		if err != nil {
			t.Fatalf("%s/%q: bad fixture hex: %v", v.algo, v.in, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s/%q: got %x, want %s", v.algo, v.in, got, v.hex)
		}
	}
}

// TestComputeUnknownAlgo lists the supported algorithms in the
// error message.
func TestComputeUnknownAlgo(t *testing.T) {
	_, err := computeFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		interpreter.BytesVal(nil),
		interpreter.StringVal("md4"),
	})
	if err == nil {
		t.Fatal("expected unknown-algo error")
	}
	if !strings.Contains(err.Error(), `unknown digest algorithm "md4"`) {
		t.Errorf("error doesn't quote the unknown algo: %v", err)
	}
	if !strings.Contains(err.Error(), "sha256") {
		t.Errorf("error doesn't list known algos: %v", err)
	}
}

// TestComputeRejectsNonBytes confirms the bytes-only first argument.
func TestComputeRejectsNonBytes(t *testing.T) {
	_, err := computeFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		interpreter.StringVal("abc"),
		interpreter.StringVal("md5"),
	})
	if err == nil {
		t.Fatal("expected bytes-required error")
	}
	if !strings.Contains(err.Error(), "must be bytes") {
		t.Errorf("error doesn't mention bytes requirement: %v", err)
	}
}

// TestStreamMatchesOneShot: streaming chunks gives the same digest
// as the one-shot call.
func TestStreamMatchesOneShot(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	input := []byte("the quick brown fox jumps over the lazy dog")
	for _, algo := range []string{"md5", "sha1", "sha256"} {
		s, err := streamFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal(algo)})
		if err != nil {
			t.Fatalf("%s: stream: %v", algo, err)
		}
		// Three chunks to exercise multi-update.
		for _, slice := range [][]byte{input[:10], input[10:25], input[25:]} {
			if _, err := updateFn(interpreter.BuiltinCtx{}, []interpreter.Value{s, interpreter.BytesVal(slice)}); err != nil {
				t.Fatalf("%s: update: %v", algo, err)
			}
		}
		got, err := finalizeFn(interpreter.BuiltinCtx{}, []interpreter.Value{s})
		if err != nil {
			t.Fatalf("%s: finalize: %v", algo, err)
		}
		want := compute(t, input, algo)
		if !bytes.Equal(got.Bytes, want) {
			t.Errorf("%s: streamed %x, one-shot %x", algo, got.Bytes, want)
		}
	}
}

// TestStreamUnknownAlgo: stream constructor rejects unknown algos.
func TestStreamUnknownAlgo(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)
	_, err := streamFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("rot13")})
	if err == nil {
		t.Fatal("expected unknown-algo error")
	}
}

// TestFinalizeConsumes: a stream can't be reused after finalize.
func TestFinalizeConsumes(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	s, _ := streamFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("md5")})
	_, _ = finalizeFn(interpreter.BuiltinCtx{}, []interpreter.Value{s})

	if _, err := updateFn(interpreter.BuiltinCtx{}, []interpreter.Value{s, interpreter.BytesVal([]byte("x"))}); err == nil {
		t.Fatal("expected error updating finalized stream")
	}
	if _, err := finalizeFn(interpreter.BuiltinCtx{}, []interpreter.Value{s}); err == nil {
		t.Fatal("expected error finalizing twice")
	}
}

// TestStreamWrongStruct rejects a struct from a different library.
func TestStreamWrongStruct(t *testing.T) {
	bogus := interpreter.NamespacedStructVal("os", "Process",
		[]interpreter.StructField{{Name: "pid", Value: interpreter.IntVal(1)}})
	_, err := updateFn(interpreter.BuiltinCtx{}, []interpreter.Value{bogus, interpreter.BytesVal(nil)})
	if err == nil {
		t.Fatal("expected struct-type error")
	}
	if !strings.Contains(err.Error(), "hash.Stream") {
		t.Errorf("error doesn't mention hash.Stream: %v", err)
	}
}
