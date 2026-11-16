// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package oslib

import (
	"errors"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
	"jennifer-lang.dev/jennifer/internal/parser"
)

// TestSpawnHandleCapIsKindLimit: os.spawn past the live-child cap is kind
// "limit" (M28.54), not "runtime". The registry is filled with placeholders,
// so no process starts.
func TestSpawnHandleCapIsKindLimit(t *testing.T) {
	if !execSupported() {
		t.Skip("no exec on this build")
	}
	handlesMu.Lock()
	saved := handles
	handles = map[int64]*processState{}
	for i := int64(0); i < maxProcHandles; i++ {
		handles[-1-i] = nil
	}
	handlesMu.Unlock()
	defer func() {
		handlesMu.Lock()
		handles = saved
		handlesMu.Unlock()
	}()
	argv := interpreter.ListVal(parser.PrimitiveType(parser.TypeString), []interpreter.Value{interpreter.StringVal("true")})
	_, err := spawnFn(interpreter.BuiltinCtx{}, []interpreter.Value{argv})
	var ce *limits.CapError
	if !errors.As(err, &ce) {
		t.Fatalf("got %T %v, want a limits.CapError", err, err)
	}
}
