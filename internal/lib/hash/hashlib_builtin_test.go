// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

package hashlib

import (
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

func TestInstallRegistersEveryHashBuiltin(t *testing.T) {
	in := interpreter.New()
	Install(in)
	for _, name := range []string{"compute", "hmac", "equal", "stream", "update", "finalize", "discard"} {
		if in.LookupNamespacedBuiltin("hash", name) == nil {
			t.Errorf("hash.%s is not registered", name)
		}
	}
}

// TestDiscardStream covers hash.discard: a streaming handle can be dropped
// without finalizing, and reusing it (or discarding twice) is a catchable error.
func TestDiscardStream(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	s, err := streamFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("sha256")})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if _, err := discardFn(interpreter.BuiltinCtx{}, []interpreter.Value{s}); err != nil {
		t.Fatalf("discard: %v", err)
	}
	if _, err := discardFn(interpreter.BuiltinCtx{}, []interpreter.Value{s}); err == nil {
		t.Error("second discard should error")
	}
	if _, err := updateFn(interpreter.BuiltinCtx{}, []interpreter.Value{s, interpreter.BytesVal([]byte("x"))}); err == nil {
		t.Error("update after discard should error")
	}
	if _, err := discardFn(interpreter.BuiltinCtx{}, nil); err == nil {
		t.Error("discard arity error expected")
	}
	if _, err := discardFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(1)}); err == nil {
		t.Error("discard(non-stream) should error")
	}
}
