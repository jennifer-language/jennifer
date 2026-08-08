// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package crclib

import (
	"bytes"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func TestInstallRegistersEveryCrcBuiltin(t *testing.T) {
	in := interpreter.New()
	Install(in)
	for _, name := range []string{"compute", "stream", "update", "finalize", "discard"} {
		if in.LookupNamespacedBuiltin("crc", name) == nil {
			t.Errorf("crc.%s is not registered", name)
		}
	}
}

// TestDiscardStream covers crc.discard: a stream handle can be dropped without
// finalizing, and using it afterward (or discarding twice) is a catchable error.
func TestDiscardStream(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	s, err := streamFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("crc32")})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if _, err := discardFn(interpreter.BuiltinCtx{}, []interpreter.Value{s}); err != nil {
		t.Fatalf("discard: %v", err)
	}
	// A second discard errors (the handle is already gone).
	if _, err := discardFn(interpreter.BuiltinCtx{}, []interpreter.Value{s}); err == nil {
		t.Error("second discard should error")
	}
	// Using the discarded handle also errors.
	if _, err := updateFn(interpreter.BuiltinCtx{}, []interpreter.Value{s, interpreter.BytesVal([]byte("x"))}); err == nil {
		t.Error("update after discard should error")
	}
	// arity + non-stream errors.
	if _, err := discardFn(interpreter.BuiltinCtx{}, nil); err == nil {
		t.Error("discard arity error expected")
	}
	if _, err := discardFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(1)}); err == nil {
		t.Error("discard(non-stream) should error")
	}
}

// TestPadToWidth pins the left-zero-pad helper directly (the one-shot path never
// exercises the short-input branch, since a CRC sum is already full-width).
func TestPadToWidth(t *testing.T) {
	if got := padToWidth([]byte{0xAB}, 4); !bytes.Equal(got, []byte{0, 0, 0, 0xAB}) {
		t.Errorf("padToWidth({AB}, 4) = % x, want 00 00 00 AB", got)
	}
	if got := padToWidth([]byte{1, 2}, 2); !bytes.Equal(got, []byte{1, 2}) { // exactly width
		t.Errorf("padToWidth(len2, 2) = % x, want unchanged", got)
	}
	in := []byte{1, 2, 3, 4, 5}
	if got := padToWidth(in, 4); !bytes.Equal(got, in) { // already wider than width
		t.Errorf("padToWidth(len5, 4) = % x, want unchanged", got)
	}
}
