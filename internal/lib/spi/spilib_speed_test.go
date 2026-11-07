// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build linux && !tinygo

package spilib

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/devio"
)

// TestConfigureSpeedRange: speedHz is a u32 in the kernel ABI; 2^32 used to
// truncate to 0 and 2^32+1 to 1 Hz. The device is a regular file (no ioctl is
// reached for an out-of-range speed).
func TestConfigureSpeedRange(t *testing.T) {
	ResetForTest()
	f, err := os.Create(filepath.Join(t.TempDir(), "spidev"))
	if err != nil {
		t.Fatal(err)
	}
	devsMu.Lock()
	nextID++
	id := nextID
	devs[id] = &devState{f: f, speedHz: 500000}
	devsMu.Unlock()
	h := devio.Handle(LibraryName, "Device", id)
	for _, speed := range []int64{0, -1, 1 << 32, 1<<32 + 1} {
		_, err := configureFn(interpreter.BuiltinCtx{}, []Value{h, interpreter.IntVal(0), interpreter.IntVal(speed)})
		if err == nil || !strings.Contains(err.Error(), "speedHz must be in 1..4294967295") {
			t.Errorf("speed %d: err = %v", speed, err)
		}
	}
}
