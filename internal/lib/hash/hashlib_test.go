// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

package hashlib

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
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
		{"sha512", "", "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e"},
		{"sha512", "abc", "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f"},
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

// hmac is a Go-side wrapper around hmacFn for the table tests.
func hmacHelper(t *testing.T, key, msg []byte, algo string) []byte {
	t.Helper()
	v, err := hmacFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		interpreter.BytesVal(key),
		interpreter.BytesVal(msg),
		interpreter.StringVal(algo),
	})
	if err != nil {
		t.Fatalf("hmac(%q): %v", algo, err)
	}
	if v.Kind != interpreter.KindBytes {
		t.Fatalf("hmac(%q): expected KindBytes, got %s", algo, v.Kind)
	}
	return v.Bytes
}

// TestHmacKnownVectors pins hash.hmac against the canonical RFC 2202 / RFC 4231
// vectors (key "Jefe", data "what do ya want for nothing?") for each algorithm.
func TestHmacKnownVectors(t *testing.T) {
	key := []byte("Jefe")
	msg := []byte("what do ya want for nothing?")
	vectors := []struct {
		algo, hexWant string
	}{
		{"md5", "750c783e6ab0b503eaa86e310a5db738"},
		{"sha1", "effcdf6ae5eb2fa2d27416d5f184df9c259a7c79"},
		{"sha256", "5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843"},
	}
	for _, v := range vectors {
		got := hex.EncodeToString(hmacHelper(t, key, msg, v.algo))
		if got != v.hexWant {
			t.Errorf("hmac(%q) = %s, want %s", v.algo, got, v.hexWant)
		}
	}
}

// TestHmacErrors covers the boundary checks.
func TestHmacErrors(t *testing.T) {
	good := interpreter.BytesVal([]byte("k"))
	cases := [][]interpreter.Value{
		{good, good}, // too few args
		{interpreter.StringVal("k"), good, interpreter.StringVal("sha256")}, // key not bytes
		{good, interpreter.StringVal("m"), interpreter.StringVal("sha256")}, // message not bytes
		{good, good, interpreter.StringVal("sha3")},                         // unknown algo
	}
	for i, args := range cases {
		if _, err := hmacFn(interpreter.BuiltinCtx{}, args); err == nil {
			t.Errorf("case %d: expected an error, got nil", i)
		}
	}
}
