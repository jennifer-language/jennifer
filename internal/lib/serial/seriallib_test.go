// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

//go:build linux && !tinygo

package seriallib_test

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/convert"
	iolib "jennifer-lang.dev/jennifer/internal/lib/io"
	seriallib "jennifer-lang.dev/jennifer/internal/lib/serial"
	"jennifer-lang.dev/jennifer/internal/parser"
)

func runSerial(t *testing.T, src string) (string, error) {
	t.Helper()
	seriallib.ResetForTest()
	prog, err := parser.Parse(src)
	if err != nil {
		return "", err
	}
	in := interpreter.New()
	var buf bytes.Buffer
	in.Out = &buf
	iolib.Install(in)
	convert.Install(in)
	seriallib.Install(in)
	rerr := in.Run(prog)
	return buf.String(), rerr
}

// openPTY returns a master file and the slave device path of a fresh pty pair.
// The slave behaves like a serial port for termios purposes.
func openPTY(t *testing.T) (*os.File, string) {
	t.Helper()
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no /dev/ptmx: %v", err)
	}
	if err := unix.IoctlSetPointerInt(int(m.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		m.Close()
		t.Skipf("unlockpt: %v", err)
	}
	n, err := unix.IoctlGetInt(int(m.Fd()), unix.TIOCGPTN)
	if err != nil {
		m.Close()
		t.Skipf("ptn: %v", err)
	}
	return m, fmt.Sprintf("/dev/pts/%d", n)
}

// A configured port reads bytes the peer sent and writes bytes the peer receives.
func TestSerialPTYLoopback(t *testing.T) {
	master, slave := openPTY(t)
	defer master.Close()
	if _, err := master.Write([]byte("PING\n")); err != nil {
		t.Fatal(err)
	}
	out, err := runSerial(t, fmt.Sprintf(`use serial; use io; use convert;
def p as serial.Port init serial.open(%q, 115200);
def got as bytes init serial.read($p, 5);
io.printf("[%%s]", convert.stringFromBytes($got, "utf-8"));
def n as int init serial.write($p, convert.bytesFromString("PONG", "utf-8"));
io.printf("wrote=%%d", $n);
serial.close($p);`, slave))
	if err != nil {
		t.Fatalf("run: %v (out=%q)", err, out)
	}
	if out != "[PING\n]wrote=4" {
		t.Errorf("got %q, want [PING\\n]wrote=4", out)
	}
	// The port's PONG must reach the master side.
	_ = master.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 32)
	n, _ := master.Read(buf)
	if !bytes.Contains(buf[:n], []byte("PONG")) {
		t.Errorf("master read %q, want it to contain PONG", buf[:n])
	}
}

func TestSerialErrors(t *testing.T) {
	// Opening a missing device is a positioned error, not a crash.
	if _, err := runSerial(t, `use serial; def p as serial.Port init serial.open("/dev/tty-does-not-exist-xyz", 9600);`); err == nil {
		t.Error("expected an error opening a missing device")
	}
	// A non-standard baud rate is rejected up front.
	master, slave := openPTY(t)
	defer master.Close()
	_, err := runSerial(t, fmt.Sprintf(`use serial; def p as serial.Port init serial.open(%q, 12345);`, slave))
	if err == nil || !contains(err.Error(), "baud") {
		t.Errorf("expected an unsupported-baud error, got %v", err)
	}
	// A non-Port first argument is a boundary error.
	if _, err := runSerial(t, `use serial; def n as int init serial.read(5, 1);`); err == nil {
		t.Error("expected a wrong-argument error for serial.read(5, 1)")
	}
}

func contains(s, sub string) bool {
	return bytes.Contains([]byte(s), []byte(sub))
}
