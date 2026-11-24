// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build linux && !tinygo

package i2clib

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/devio"
)

// addBus registers f as an open bus without the I2C_SLAVE ioctl.
func addBus(f *os.File) Value {
	busesMu.Lock()
	defer busesMu.Unlock()
	nextID++
	buses[nextID] = &busState{f: f, addr: 0x76}
	return devio.Handle(LibraryName, "Bus", nextID)
}

// TestMessageLimit (M29.49): i2c-dev moves at most 8192 bytes per message, so
// a longer read / readReg / write / writeReg throws kind "limit" instead of
// transferring a shortened message; 8192 itself goes through. A regular file
// stands in for the bus.
func TestMessageLimit(t *testing.T) {
	ResetForTest()
	path := filepath.Join(t.TempDir(), "bus")
	if err := os.WriteFile(path, make([]byte, 20000), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	b := addBus(f)
	ctx := interpreter.BuiltinCtx{}
	iv, bv := interpreter.IntVal, interpreter.BytesVal
	v, err := readFn(ctx, []Value{b, iv(8192)})
	if err != nil || len(v.Bytes()) != 8192 {
		t.Fatalf("read 8192: %d bytes, %v", len(v.Bytes()), err)
	}
	if _, err := writeRegFn(ctx, []Value{b, iv(1), bv(make([]byte, 8191))}); err != nil {
		t.Fatalf("writeReg of 8191 data bytes: %v", err)
	}
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	for name, call := range map[string]func() error{
		"read":     func() error { _, err := readFn(ctx, []Value{b, iv(8193)}); return err },
		"read big": func() error { _, err := readFn(ctx, []Value{b, iv(1 << 28)}); return err },
		"readReg":  func() error { _, err := readRegFn(ctx, []Value{b, iv(1), iv(8193)}); return err },
		"write":    func() error { _, err := writeFn(ctx, []Value{b, bv(make([]byte, 8193))}); return err },
		"writeReg": func() error { _, err := writeRegFn(ctx, []Value{b, iv(1), bv(make([]byte, 8192))}); return err },
	} {
		if kind, _, _, _, _ := interpreter.ClassifyError(call()); kind != "limit" {
			t.Errorf("%s: kind %q, want limit", name, kind)
		}
	}
	runtime.ReadMemStats(&m1)
	if got := m1.TotalAlloc - m0.TotalAlloc; got > 1<<20 {
		t.Errorf("refused reads allocated %d bytes", got)
	}
}

// TestClosedBus (M29.46): any call on a closed bus is kind "closed"; a second
// close does nothing.
func TestClosedBus(t *testing.T) {
	ResetForTest()
	f, err := os.Create(filepath.Join(t.TempDir(), "bus"))
	if err != nil {
		t.Fatal(err)
	}
	b := addBus(f)
	ctx := interpreter.BuiltinCtx{}
	for i := 0; i < 2; i++ {
		if _, err := closeFn(ctx, []Value{b}); err != nil {
			t.Fatalf("close %d: %v", i+1, err)
		}
	}
	_, err = readFn(ctx, []Value{b, interpreter.IntVal(1)})
	if kind, _, _, _, _ := interpreter.ClassifyError(err); kind != "closed" {
		t.Errorf("read after close: kind %q (%v)", kind, err)
	}
}

// TestReadRegIsOneUnit: two tasks calling readReg on one bus must not
// interleave the register-pointer write of one between the write and read of
// the other. A socketpair stands in for the bus; the peer answers only after
// checking no second pointer byte arrived.
func TestReadRegIsOneUnit(t *testing.T) {
	ResetForTest()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Non-blocking so os.File puts the peer on the poller (read deadlines).
	if err := unix.SetNonblock(fds[1], true); err != nil {
		t.Fatal(err)
	}
	peer := os.NewFile(uintptr(fds[1]), "peer")
	defer peer.Close()
	b := addBus(os.NewFile(uintptr(fds[0]), "bus"))

	done := make(chan error, 2)
	for _, reg := range []int64{1, 2} {
		go func() {
			_, err := readRegFn(interpreter.BuiltinCtx{}, []Value{b, interpreter.IntVal(reg), interpreter.IntVal(1)})
			done <- err
		}()
	}
	buf := make([]byte, 4)
	for i := 0; i < 2; i++ {
		if err := peer.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			t.Fatal(err)
		}
		n, err := peer.Read(buf)
		if err != nil || n != 1 {
			t.Fatalf("transaction %d: read %d bytes, %v", i, n, err)
		}
		// Another pointer write must not arrive before this one is answered.
		_ = peer.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		if n, err := peer.Read(buf); err == nil {
			t.Fatalf("transaction %d: %d more pointer byte(s) before the reply", i, n)
		}
		if _, err := peer.Write([]byte{0xaa}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}
