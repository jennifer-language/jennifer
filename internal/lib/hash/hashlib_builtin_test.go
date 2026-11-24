// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

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
// without finalizing; a second discard does nothing and any other use is kind
// "closed" (M29.46).
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
	if _, err := discardFn(interpreter.BuiltinCtx{}, []interpreter.Value{s}); err != nil {
		t.Errorf("second discard: %v", err)
	}
	_, err = updateFn(interpreter.BuiltinCtx{}, []interpreter.Value{s, interpreter.BytesVal([]byte("x"))})
	if kind, _, _, _, _ := interpreter.ClassifyError(err); kind != "closed" {
		t.Errorf("update after discard: kind %q (%v)", kind, err)
	}
	_, err = finalizeFn(interpreter.BuiltinCtx{}, []interpreter.Value{s})
	if kind, _, _, _, _ := interpreter.ClassifyError(err); kind != "closed" {
		t.Errorf("finalize after discard: kind %q (%v)", kind, err)
	}
	if _, err := discardFn(interpreter.BuiltinCtx{}, nil); err == nil {
		t.Error("discard arity error expected")
	}
	if _, err := discardFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.IntVal(1)}); err == nil {
		t.Error("discard(non-stream) should error")
	}
}
