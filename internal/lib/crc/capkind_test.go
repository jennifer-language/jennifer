// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package crclib

import (
	"errors"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// TestStreamCapIsKindLimit: opening a stream past the open-stream cap is kind
// "limit", not "runtime".
func TestStreamCapIsKindLimit(t *testing.T) {
	var opened []interpreter.Value
	defer func() {
		for _, s := range opened {
			_, _ = discardFn(interpreter.BuiltinCtx{}, []interpreter.Value{s})
		}
	}()
	var err error
	for i := 0; i <= maxStreams; i++ {
		var s interpreter.Value
		if s, err = streamFn(interpreter.BuiltinCtx{}, []interpreter.Value{interpreter.StringVal("crc32")}); err != nil {
			break
		}
		opened = append(opened, s)
	}
	var ce *limits.CapError
	if !errors.As(err, &ce) {
		t.Fatalf("after %d streams: %T %v, want a limits.CapError", len(opened), err, err)
	}
}
