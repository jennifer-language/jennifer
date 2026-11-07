// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build linux && !tinygo

package seriallib

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// ptyPair opens a pseudo-terminal (no hardware) and returns its master and the
// slave path, which behaves like a serial port for termios purposes.
func ptyPair(t *testing.T) (*os.File, string) {
	t.Helper()
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no /dev/ptmx: %v", err)
	}
	t.Cleanup(func() { m.Close() })
	rc, _ := m.SyscallConn()
	var n int
	var cerr error
	_ = rc.Control(func(fd uintptr) {
		if cerr = unix.IoctlSetPointerInt(int(fd), unix.TIOCSPTLCK, 0); cerr == nil {
			n, cerr = unix.IoctlGetInt(int(fd), unix.TIOCGPTN)
		}
	})
	if cerr != nil {
		t.Skipf("pty: %v", cerr)
	}
	return m, fmt.Sprintf("/dev/pts/%d", n)
}

func options(fields ...interpreter.StructField) Value {
	return interpreter.NamespacedStructVal(LibraryName, "Options", fields)
}

// TestOpenWithDefaults: fields left out of a serial.Options literal arrive as
// 0 / "" and take the 9600 8N1 default instead of failing ("data bits must be
// 5, 6, 7, or 8, got 0").
func TestOpenWithDefaults(t *testing.T) {
	ResetForTest()
	_, slave := ptyPair(t)
	opts := options(
		interpreter.StructField{Name: "baud", Value: interpreter.IntVal(19200)},
		interpreter.StructField{Name: "dataBits", Value: interpreter.IntVal(0)},
		interpreter.StructField{Name: "parity", Value: interpreter.StringVal("")},
		interpreter.StructField{Name: "stopBits", Value: interpreter.IntVal(0)},
	)
	p, err := openWithFn(interpreter.BuiltinCtx{}, []Value{interpreter.StringVal(slave), opts})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := closeFn(interpreter.BuiltinCtx{}, []Value{p}); err != nil {
		t.Fatal(err)
	}
}

// TestCloseWakesRead: a read blocked in another task returns an error when the
// port is closed; it used to stay blocked in the kernel (the fd was taken off
// the poller), holding a freed fd number.
func TestCloseWakesRead(t *testing.T) {
	ResetForTest()
	_, slave := ptyPair(t)
	p, err := openFn(interpreter.BuiltinCtx{}, []Value{interpreter.StringVal(slave), interpreter.IntVal(9600)})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := readFn(interpreter.BuiltinCtx{}, []Value{p, interpreter.IntVal(16)})
		done <- err
	}()
	time.Sleep(50 * time.Millisecond) // let the read block
	if _, err := closeFn(interpreter.BuiltinCtx{}, []Value{p}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "serial.read") {
			t.Fatalf("read after close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("close did not wake the pending read")
	}
}

// TestPortStaysNonBlocking: the port fd is opened O_NONBLOCK (no wait for
// carrier on a CLOCAL-clear tty) and stays non-blocking through configure,
// read and write.
func TestPortStaysNonBlocking(t *testing.T) {
	ResetForTest()
	master, slave := ptyPair(t)
	p, err := openFn(interpreter.BuiltinCtx{}, []Value{interpreter.StringVal(slave), interpreter.IntVal(9600)})
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn(interpreter.BuiltinCtx{}, []Value{p})
	if _, err := master.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := readFn(interpreter.BuiltinCtx{}, []Value{p, interpreter.IntVal(1)}); err != nil {
		t.Fatal(err)
	}
	if _, err := writeFn(interpreter.BuiltinCtx{}, []Value{p, interpreter.BytesVal([]byte("y"))}); err != nil {
		t.Fatal(err)
	}
	ps, err := resolvePort("t", []Value{p})
	if err != nil {
		t.Fatal(err)
	}
	rc, _ := ps.f.SyscallConn()
	var flags int
	_ = rc.Control(func(fd uintptr) { flags, _ = unix.FcntlInt(fd, unix.F_GETFL, 0) })
	if flags&unix.O_NONBLOCK == 0 {
		t.Error("port fd is in blocking mode")
	}
}
