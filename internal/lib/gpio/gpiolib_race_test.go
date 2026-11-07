// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build linux && !tinygo

package gpiolib

import (
	"os"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// TestReleaseWaitsForIoctl: a release from another task must not close a line
// fd while a read or write ioctl on it is in flight (the number could be
// reused by a later open). The line fd here is a pipe; the ioctl is stubbed.
func TestReleaseWaitsForIoctl(t *testing.T) {
	for _, fn := range []struct {
		name string
		b    interpreter.Builtin
		args []Value
	}{
		{"read", readFn, []Value{interpreter.IntVal(5)}},
		{"write", writeFn, []Value{interpreter.IntVal(5), interpreter.IntVal(1)}},
	} {
		t.Run(fn.name, func(t *testing.T) {
			ResetForTest()
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			defer w.Close()
			fd, err := unix.Dup(int(r.Fd()))
			if err != nil {
				t.Fatal(err)
			}
			linesMu.Lock()
			lines[5] = &lineState{fd: fd, dir: "out"}
			linesMu.Unlock()

			entered, finish := make(chan struct{}), make(chan struct{})
			closedDuringIoctl := false
			old := ioctlPtr
			ioctlPtr = func(fd int, _ uintptr, _ unsafe.Pointer) error {
				close(entered)
				<-finish
				// Still our pipe: fcntl on a closed fd fails with EBADF.
				if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); err != nil {
					closedDuringIoctl = true
				}
				return nil
			}
			defer func() { ioctlPtr = old }()

			done := make(chan error, 1)
			go func() {
				_, err := fn.b(interpreter.BuiltinCtx{}, fn.args)
				done <- err
			}()
			<-entered
			released := make(chan error, 1)
			go func() {
				_, err := releaseFn(interpreter.BuiltinCtx{}, []Value{interpreter.IntVal(5)})
				released <- err
			}()
			var relErr error
			early := false
			select {
			case relErr = <-released:
				early = true
				t.Error("release finished while the ioctl was in flight")
			case <-time.After(50 * time.Millisecond):
			}
			close(finish)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if !early {
				relErr = <-released
			}
			if relErr != nil {
				t.Fatal(relErr)
			}
			if closedDuringIoctl {
				t.Error("line fd closed during the ioctl")
			}
		})
	}
}
