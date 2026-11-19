// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build linux && !tinygo

package seriallib

import (
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"jennifer-lang.dev/jennifer/internal/interpreter"
)

// dirtyTermios sets, through fd, the flags a previous user of a port may leave
// behind and that raw mode must clear: hardware and software flow control,
// input parity checking, upper-case mapping, IMAXBEL, mark/space parity.
func dirtyTermios(t *testing.T, fd int) {
	t.Helper()
	tio, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		t.Fatal(err)
	}
	tio.Cflag |= unix.CRTSCTS | unix.CMSPAR
	tio.Iflag |= unix.IXON | unix.IXOFF | unix.IXANY | unix.INPCK | unix.IUCLC | unix.IMAXBEL | unix.ICRNL
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, tio); err != nil {
		t.Fatal(err)
	}
}

// openSlave opens the pty slave as a second, plain fd for inspecting termios.
func openSlave(t *testing.T, path string) int {
	t.Helper()
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unix.Close(fd) })
	return fd
}

func openWith(t *testing.T, path string, fields ...interpreter.StructField) Value {
	t.Helper()
	p, err := openWithFn(interpreter.BuiltinCtx{}, []Value{interpreter.StringVal(path), options(fields...)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = closeFn(interpreter.BuiltinCtx{}, []Value{p}) })
	return p
}

func field(name string, v Value) interpreter.StructField {
	return interpreter.StructField{Name: name, Value: v}
}

// TestRawModeClearsLeftoverFlags: a port left with CRTSCTS / IXOFF / IXANY /
// INPCK / IUCLC / IMAXBEL / CMSPAR by another program used to keep them after
// serial.open, so output stalled on CTS or XOFF and bytes were altered. The
// flowControl option sets exactly the requested kind.
func TestRawModeClearsLeftoverFlags(t *testing.T) {
	cases := []struct {
		flow            string
		wantI, forbidI  uint32
		wantC, forbidC  uint32
		parity          string
		wantParityCheck bool
	}{
		{flow: "", forbidI: unix.IXON | unix.IXOFF | unix.IXANY, forbidC: unix.CRTSCTS},
		{flow: "none", forbidI: unix.IXON | unix.IXOFF | unix.IXANY, forbidC: unix.CRTSCTS},
		{flow: "rtscts", wantC: unix.CRTSCTS, forbidI: unix.IXON | unix.IXOFF | unix.IXANY},
		{flow: "xonxoff", wantI: unix.IXON | unix.IXOFF, forbidI: unix.IXANY, forbidC: unix.CRTSCTS},
		{flow: "none", parity: "even", wantParityCheck: true, forbidC: unix.CRTSCTS},
	}
	for _, c := range cases {
		t.Run(c.flow+"/"+c.parity, func(t *testing.T) {
			ResetForTest()
			_, slave := ptyPair(t)
			fd := openSlave(t, slave)
			dirtyTermios(t, fd)
			openWith(t, slave,
				field("baud", interpreter.IntVal(115200)),
				field("parity", interpreter.StringVal(c.parity)),
				field("flowControl", interpreter.StringVal(c.flow)),
				field("shared", interpreter.BoolVal(true)))
			tio, err := unix.IoctlGetTermios(fd, unix.TCGETS)
			if err != nil {
				t.Fatal(err)
			}
			always := uint32(unix.IUCLC | unix.IMAXBEL | unix.ICRNL)
			if !c.wantParityCheck {
				always |= unix.INPCK
			}
			if got := tio.Iflag & (always | c.forbidI); got != 0 {
				t.Errorf("iflag keeps %#o", got)
			}
			if got := tio.Cflag & (unix.CMSPAR | unix.CIBAUD | c.forbidC); got != 0 {
				t.Errorf("cflag keeps %#o", got)
			}
			if tio.Iflag&c.wantI != c.wantI || tio.Cflag&c.wantC != c.wantC {
				t.Errorf("iflag %#o / cflag %#o lack %#o / %#o", tio.Iflag, tio.Cflag, c.wantI, c.wantC)
			}
			if c.wantParityCheck && tio.Iflag&unix.INPCK == 0 {
				t.Error("parity enabled without INPCK")
			}
		})
	}
}

// TestMakeRawParity: the termios word for 7O2 (a pty forces CS8 and no parity,
// so the line settings are checked on the struct itself).
func TestMakeRawParity(t *testing.T) {
	c, err := newLineConfig("t", 4000000, 7, "odd", 2, "none", false)
	if err != nil {
		t.Fatal(err)
	}
	tio := unix.Termios{Cflag: unix.CS8 | unix.CMSPAR | unix.CIBAUD | unix.B9600}
	makeRaw(&tio, c)
	want := uint32(unix.CS7 | unix.PARENB | unix.PARODD | unix.CSTOPB | unix.B4000000 | unix.CLOCAL | unix.CREAD)
	if tio.Cflag != want {
		t.Errorf("cflag = %#o, want %#o", tio.Cflag, want)
	}
	if tio.Iflag&unix.INPCK == 0 {
		t.Error("INPCK not set with parity")
	}
}

// TestExclusiveOpen: serial.open claims the port (TIOCEXCL), so a second
// open fails with a clear "busy" error; close releases the claim (a pty slave
// keeps it while the master stays open); shared: true skips it.
func TestExclusiveOpen(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses TIOCEXCL")
	}
	ResetForTest()
	_, slave := ptyPair(t)
	p, err := openFn(interpreter.BuiltinCtx{}, []Value{interpreter.StringVal(slave), interpreter.IntVal(9600)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = openFn(interpreter.BuiltinCtx{}, []Value{interpreter.StringVal(slave), interpreter.IntVal(9600)})
	if err == nil || !strings.Contains(err.Error(), "serial.open: "+slave+": device or resource busy (another program") {
		t.Fatalf("second open: %v", err)
	}
	if _, err := closeFn(interpreter.BuiltinCtx{}, []Value{p}); err != nil {
		t.Fatal(err)
	}
	openWith(t, slave, field("shared", interpreter.BoolVal(true)))
	openWith(t, slave, field("shared", interpreter.BoolVal(true)))
}

// TestOpenErrors: errors carry the function and the path; argument errors come
// before the device is opened (no DTR pulse for a typo).
func TestOpenErrors(t *testing.T) {
	ResetForTest()
	cases := []struct {
		fn   func(interpreter.BuiltinCtx, []Value) (Value, error)
		args []Value
		want string
	}{
		{openFn, []Value{interpreter.StringVal("/dev/null"), interpreter.IntVal(9600)},
			"serial.open: /dev/null: configuring the port: inappropriate ioctl for device (not a serial port)"},
		{openFn, []Value{interpreter.StringVal("/no/such/tty"), interpreter.IntVal(14400)},
			"serial.open: unsupported baud rate 14400"},
		{openFn, []Value{interpreter.StringVal("/no/such/tty"), interpreter.IntVal(9600)},
			"serial.open: /no/such/tty: no such file or directory"},
		{openWithFn, []Value{interpreter.StringVal("/no/such/tty"), options(field("flowControl", interpreter.StringVal("dtrdsr")))},
			`serial.openWith: flowControl must be "none", "rtscts", or "xonxoff", got "dtrdsr"`},
		{openWithFn, []Value{interpreter.StringVal("/no/such/tty"), options(field("parity", interpreter.StringVal("mark")))},
			`serial.openWith: parity must be`},
	}
	for _, c := range cases {
		_, err := c.fn(interpreter.BuiltinCtx{}, c.args)
		if err == nil || !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("got %v, want prefix %q", err, c.want)
		}
	}
}

// errKind returns a raised error's kind and message ("runtime" otherwise).
func errKind(err error) (string, string) {
	sig, ok := err.(*interpreter.ErrorSignal)
	if !ok {
		return "runtime", err.Error()
	}
	var kind, msg string
	for _, f := range sig.Value.Fields {
		switch f.Name {
		case "kind":
			kind = f.Value.Str
		case "message":
			msg = f.Value.Str
		}
	}
	return kind, msg
}

// TestHangup: once the far end goes away (a USB adapter unplugged; here the
// pty master closes), read and write throw Error{kind: "closed"}. A read used
// to return empty bytes forever, so a read loop spun at full CPU.
func TestHangup(t *testing.T) {
	ResetForTest()
	master, slave := ptyPair(t)
	p, err := openFn(interpreter.BuiltinCtx{}, []Value{interpreter.StringVal(slave), interpreter.IntVal(9600)})
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn(interpreter.BuiltinCtx{}, []Value{p})
	if _, err := master.Write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	v, err := readFn(interpreter.BuiltinCtx{}, []Value{p, interpreter.IntVal(16)})
	if err != nil || string(v.Bytes) != "hi" {
		t.Fatalf("read before hangup: %q %v", v.Bytes, err)
	}
	master.Close()
	for i := 0; i < 3; i++ {
		_, err := readFn(interpreter.BuiltinCtx{}, []Value{p, interpreter.IntVal(16)})
		if err == nil {
			t.Fatal("read after hangup returned without an error")
		}
		if kind, msg := errKind(err); kind != "closed" || msg != "serial.read: port hung up (device unplugged or the other end closed)" {
			t.Fatalf("read %d after hangup: %s %q", i, kind, msg)
		}
	}
	_, err = writeFn(interpreter.BuiltinCtx{}, []Value{p, interpreter.BytesVal([]byte("x"))})
	if err == nil {
		t.Fatal("write after hangup returned without an error")
	}
	if kind, msg := errKind(err); kind != "closed" || msg != "serial.write: port hung up (device unplugged or the other end closed)" {
		t.Fatalf("write after hangup: %s %q", kind, msg)
	}
}
