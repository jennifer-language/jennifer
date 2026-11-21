// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

package devio

import (
	"errors"
	"testing"

	"jennifer-lang.dev/jennifer/internal/limits"
)

func TestReadSize(t *testing.T) {
	if _, err := ReadSize("x.read", -1); err == nil {
		t.Error("negative length should error")
	}
	if _, err := ReadSize("x.read", MaxRead+1); err == nil {
		t.Error("over-cap length should error")
	}
	if sz, err := ReadSize("x.read", 64); err != nil || sz != 64 {
		t.Errorf("valid length: sz=%d err=%v", sz, err)
	}
	if sz, err := ReadSize("x.read", MaxRead); err != nil || sz != MaxRead {
		t.Errorf("boundary length: sz=%d err=%v", sz, err)
	}
}

// TestReadSizeCapIsKindLimit: an over-cap device read (serial.read, i2c.read,
// i2c.readReg) is kind "limit" (M29.1); a negative length stays ordinary.
func TestReadSizeCapIsKindLimit(t *testing.T) {
	var ce *limits.CapError
	if _, err := ReadSize("x.read", MaxRead+1); !errors.As(err, &ce) {
		t.Errorf("over the cap: %T %v, want a limits.CapError", err, err)
	}
	if _, err := ReadSize("x.read", -1); err == nil || errors.As(err, &ce) {
		t.Errorf("negative: %T %v, want a plain error", err, err)
	}
}
