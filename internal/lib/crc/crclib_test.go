// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

package crclib

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/mplx/jennifer-lang/internal/interpreter"
)

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

// TestCRC32IEEEVector pins "123456789" CRC-32 IEEE against the
// canonical 0xcbf43926.
func TestCRC32IEEEVector(t *testing.T) {
	got := compute(t, []byte("123456789"), "crc32")
	want, _ := hex.DecodeString("cbf43926")
	if !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
	if len(got) != 4 {
		t.Errorf("CRC-32 width = %d bytes, want 4", len(got))
	}
}

// TestCRC64ECMAVector pins "123456789" CRC-64 against the polynomial
// Go's `crc64.ECMA` table uses (0xC96C5795D7870F42). The other
// commonly-published "CRC-64/XZ" vector (0x6c40df5f...) uses a
// different polynomial; we ship Go's stdlib choice.
func TestCRC64ECMAVector(t *testing.T) {
	got := compute(t, []byte("123456789"), "crc64")
	want, _ := hex.DecodeString("995dc9bbdf1939fa")
	if !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
	if len(got) != 8 {
		t.Errorf("CRC-64 width = %d bytes, want 8", len(got))
	}
}

// TestEmptyInput: empty input yields the algorithm's zero state.
func TestEmptyInput(t *testing.T) {
	if got := compute(t, nil, "crc32"); !bytes.Equal(got, []byte{0, 0, 0, 0}) {
		t.Errorf("crc32 empty = %x, want 00000000", got)
	}
	if got := compute(t, nil, "crc64"); !bytes.Equal(got, []byte{0, 0, 0, 0, 0, 0, 0, 0}) {
		t.Errorf("crc64 empty = %x, want 0000000000000000", got)
	}
}

// TestUnknownAlgo: rejects with a positioned message.
func TestUnknownAlgo(t *testing.T) {
	_, err := computeFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		interpreter.BytesVal(nil),
		interpreter.StringVal("adler32"),
	})
	if err == nil {
		t.Fatal("expected unknown-algo error")
	}
	if !strings.Contains(err.Error(), `unknown algorithm "adler32"`) {
		t.Errorf("error doesn't quote the algo: %v", err)
	}
}

// TestRejectsNonBytes covers the bytes-only first argument.
func TestRejectsNonBytes(t *testing.T) {
	_, err := computeFn(interpreter.BuiltinCtx{}, []interpreter.Value{
		interpreter.StringVal("abc"),
		interpreter.StringVal("crc32"),
	})
	if err == nil {
		t.Fatal("expected bytes-required error")
	}
}

// TestStreamMatchesOneShot: chunked update + finalize equals
// the one-shot result for both widths.
func TestStreamMatchesOneShot(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	input := []byte("the quick brown fox jumps over the lazy dog")
	for _, algo := range []string{"crc32", "crc64"} {
		s, err := streamFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal(algo)})
		if err != nil {
			t.Fatalf("%s: stream: %v", algo, err)
		}
		for _, slice := range [][]byte{input[:15], input[15:]} {
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

// TestFinalizeConsumes: a stream is gone after finalize.
func TestFinalizeConsumes(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	s, _ := streamFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("crc32")})
	_, _ = finalizeFn(interpreter.BuiltinCtx{}, []interpreter.Value{s})

	if _, err := updateFn(interpreter.BuiltinCtx{}, []interpreter.Value{s, interpreter.BytesVal([]byte("x"))}); err == nil {
		t.Fatal("expected error updating finalized stream")
	}
}
