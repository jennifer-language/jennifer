// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build linux && !tinygo

package gpiolib

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// TestChipHandle (M29.49): gpio.open returns a chip handle; setup / read /
// write / release take it, lines of two chips with the same pin are separate,
// close releases the chip's lines, a second close does nothing, and any other
// call on a closed chip is kind "closed". No gpiochip on the test machine: a
// regular file stands in for the chip and a stub for the ioctls, which hand
// out pipe fds as line fds and keep each line's value.
func TestChipHandle(t *testing.T) {
	ResetForTest()
	values := map[int]uint64{}
	var lineFds []int
	old := ioctlPtr
	ioctlPtr = func(fd int, req uintptr, p unsafe.Pointer) error {
		switch req {
		case gpioGetInfo:
		case gpioGetLine:
			r, w, err := os.Pipe()
			if err != nil {
				return err
			}
			defer r.Close()
			defer w.Close()
			nfd, err := unix.Dup(int(r.Fd()))
			if err != nil {
				return err
			}
			lineFds = append(lineFds, nfd)
			(*gpioV2LineRequest)(p).fd = int32(nfd)
		case gpioSetVals:
			values[fd] = (*gpioV2LineValues)(p).bits
		case gpioGetVals:
			(*gpioV2LineValues)(p).bits = values[fd]
		}
		return nil
	}
	t.Cleanup(func() { ioctlPtr = old; ResetForTest() })
	path := filepath.Join(t.TempDir(), "gpiochip")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := interpreter.BuiltinCtx{}
	iv, sv := interpreter.IntVal, interpreter.StringVal
	call := func(fn interpreter.Builtin, args ...Value) Value {
		t.Helper()
		v, err := fn(ctx, args)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	a := call(openFn, sv(path))
	b := call(openFn, sv(path))
	call(setupFn, a, iv(17), sv("out"))
	call(setupFn, b, iv(17), sv("out"))
	call(writeFn, a, iv(17), iv(1))
	if v := call(readFn, a, iv(17)); v.Int() != 1 {
		t.Errorf("read a/17 = %d, want 1", v.Int())
	}
	if v := call(readFn, b, iv(17)); v.Int() != 0 {
		t.Errorf("read b/17 = %d, want 0 (a separate line)", v.Int())
	}
	call(releaseFn, b, iv(17))
	if _, err := readFn(ctx, []Value{b, iv(17)}); err == nil || !strings.Contains(err.Error(), "not set up") {
		t.Errorf("read after release: %v", err)
	}
	call(closeFn, a)
	call(closeFn, a)
	if _, err := unix.FcntlInt(uintptr(lineFds[0]), unix.F_GETFD, 0); err == nil {
		t.Error("close left the chip's line fd open")
	}
	for name, fn := range map[string]func() error{
		"setup":   func() error { _, err := setupFn(ctx, []Value{a, iv(4), sv("in")}); return err },
		"read":    func() error { _, err := readFn(ctx, []Value{a, iv(17)}); return err },
		"write":   func() error { _, err := writeFn(ctx, []Value{a, iv(17), iv(0)}); return err },
		"release": func() error { _, err := releaseFn(ctx, []Value{a, iv(17)}); return err },
	} {
		if kind, _, _, _, _ := interpreter.ClassifyError(fn()); kind != "closed" {
			t.Errorf("%s on a closed chip: kind %q", name, kind)
		}
	}
	call(closeFn, b)
}

// TestOpenNotAChip: gpio.open checks the device is a gpiochip.
func TestOpenNotAChip(t *testing.T) {
	ResetForTest()
	_, err := openFn(interpreter.BuiltinCtx{}, []Value{interpreter.StringVal("/dev/null")})
	if err == nil || !strings.Contains(err.Error(), "gpio.open: /dev/null: inappropriate ioctl for device (not a gpiochip device)") {
		t.Errorf("open /dev/null: %v", err)
	}
	_, err = openFn(interpreter.BuiltinCtx{}, []Value{interpreter.StringVal("/dev/gpiochip-nope")})
	if err == nil || !strings.Contains(err.Error(), "gpio.open: /dev/gpiochip-nope: no such file or directory") {
		t.Errorf("open missing: %v", err)
	}
}
