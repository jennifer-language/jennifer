// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build linux && !tinygo

// Linux implementation of the `serial` library: termios-configured serial-port
// I/O over golang.org/x/sys/unix. Selected only for the standard-Go build on
// Linux (the supported device-I/O platform); every other build gets the stub in
// seriallib_other.go.

package seriallib

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/devio"
	"jennifer-lang.dev/jennifer/internal/limits"
)

// portState is one open port. The fd stays non-blocking on Go's poller (never
// f.Fd(), which would switch it to blocking): read / write park in the poller,
// so the read and write deadlines of *os.File apply, ioctls run through
// SyscallConn, and close wakes a pending read.
type portState struct {
	f *os.File
	// mu guards deadline, the read deadline last armed by
	// serial.setReadDeadline (zero when cleared), which a read cancelled by
	// task.cancel restores after moving the deadline into the past to wake.
	mu       sync.Mutex
	deadline time.Time
}

// clampMillis converts a non-negative millisecond count to a time.Duration,
// clamping one that would overflow int64 nanoseconds to the largest duration
// (as net does), so a huge value means "practically never", not a wrap.
func clampMillis(ms int64) time.Duration {
	if !limits.DurationFitsUnits(ms, time.Millisecond) {
		return time.Duration(limits.MaxDurationUnits(time.Millisecond)) * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}

// failure turns an I/O error into what a Jennifer program sees, the shapes net
// uses: a passed deadline is a catchable Error{kind: "timeout"} ("serial.read:
// read timed out"), a port closed under the call Error{kind: "closed"}, anything
// else a runtime error with the OS text.
func failure(ctx interpreter.BuiltinCtx, fn, op string, err error) error {
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return interpreter.RaiseError("timeout", fmt.Sprintf("%s: %s timed out", fn, op), ctx.File, ctx.Line, ctx.Col)
	}
	if errors.Is(err, os.ErrClosed) {
		return interpreter.RaiseError("closed", fmt.Sprintf("%s: %s on a closed port", fn, op), ctx.File, ctx.Line, ctx.Col)
	}
	return fmt.Errorf("%s: %v", fn, err)
}

// watchCancel lets task.cancel wake a read blocked in the poller, as it wakes
// time.sleep and a socket read: when the calling task is cancelled the read
// deadline moves into the past. stop ends the watch and reports whether the
// cancel fired; the caller then restores the user's deadline. A no-op outside
// a task.
func watchCancel(ctx interpreter.BuiltinCtx, p *portState) func() bool {
	if ctx.Cancel == nil {
		return func() bool { return false }
	}
	var (
		mu    sync.Mutex
		fired bool
	)
	done, ended := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(ended)
		select {
		case <-ctx.Cancel.CancelSignal():
			mu.Lock()
			fired = true
			_ = p.f.SetReadDeadline(time.Unix(1, 0))
			mu.Unlock()
		case <-done:
		}
	}()
	var once sync.Once
	return func() bool {
		once.Do(func() { close(done) })
		<-ended
		mu.Lock()
		defer mu.Unlock()
		return fired
	}
}

var (
	portsMu sync.Mutex
	ports   = map[int64]*portState{}
	nextID  int64
)

// ResetForTest wipes the registry between tests.
func ResetForTest() {
	portsMu.Lock()
	defer portsMu.Unlock()
	for _, p := range ports {
		if p != nil && p.f != nil {
			_ = p.f.Close()
		}
	}
	ports = map[int64]*portState{}
	nextID = 0
}

func resolvePort(fn string, args []Value) (*portState, error) {
	id, err := devio.HandleID(fn, args[0], LibraryName, "Port")
	if err != nil {
		return nil, err
	}
	portsMu.Lock()
	defer portsMu.Unlock()
	p, ok := ports[id]
	if !ok {
		return nil, fmt.Errorf("%s: serial.Port id %d is not open (already closed, or never opened)", fn, id)
	}
	return p, nil
}

// baudCode maps a numeric baud rate to its termios Bxxx constant. Only the
// standard rates are accepted; a non-standard rate is a positioned error rather
// than a silently-wrong line speed (arbitrary rates need termios2 / BOTHER).
func baudCode(baud int64) (uint32, bool) {
	m := map[int64]uint32{
		50: unix.B50, 75: unix.B75, 110: unix.B110, 134: unix.B134, 150: unix.B150,
		200: unix.B200, 300: unix.B300, 600: unix.B600, 1200: unix.B1200, 1800: unix.B1800,
		2400: unix.B2400, 4800: unix.B4800, 9600: unix.B9600, 19200: unix.B19200,
		38400: unix.B38400, 57600: unix.B57600, 115200: unix.B115200, 230400: unix.B230400,
		460800: unix.B460800, 921600: unix.B921600, 1000000: unix.B1000000,
		1500000: unix.B1500000, 2000000: unix.B2000000, 3000000: unix.B3000000,
	}
	c, ok := m[baud]
	return c, ok
}

// configure applies raw mode plus the requested line settings to fd's termios.
func configure(fd int, baud, dataBits int64, parity string, stopBits int64) error {
	code, ok := baudCode(baud)
	if !ok {
		return fmt.Errorf("serial: unsupported baud rate %d (use a standard rate, e.g. 9600 / 19200 / 115200)", baud)
	}
	t, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return err
	}
	// Raw mode: no input processing, no echo/canonical/signals, no output post.
	t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	t.Oflag &^= unix.OPOST
	t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	t.Cflag &^= unix.CSIZE | unix.PARENB | unix.PARODD | unix.CSTOPB | unix.CBAUD
	t.Cflag |= unix.CLOCAL | unix.CREAD

	switch dataBits {
	case 5:
		t.Cflag |= unix.CS5
	case 6:
		t.Cflag |= unix.CS6
	case 7:
		t.Cflag |= unix.CS7
	case 8:
		t.Cflag |= unix.CS8
	default:
		return fmt.Errorf("serial: data bits must be 5, 6, 7, or 8, got %d", dataBits)
	}
	switch parity {
	case "none":
	case "even":
		t.Cflag |= unix.PARENB
	case "odd":
		t.Cflag |= unix.PARENB | unix.PARODD
	default:
		return fmt.Errorf(`serial: parity must be "none", "even", or "odd", got %q`, parity)
	}
	switch stopBits {
	case 1:
	case 2:
		t.Cflag |= unix.CSTOPB
	default:
		return fmt.Errorf("serial: stop bits must be 1 or 2, got %d", stopBits)
	}
	t.Cflag |= code
	t.Ispeed = code
	t.Ospeed = code
	// Blocking read: return as soon as at least one byte is available.
	t.Cc[unix.VMIN] = 1
	t.Cc[unix.VTIME] = 0
	return unix.IoctlSetTermios(fd, unix.TCSETS, t)
}

// control runs fn on f's fd without taking it off the poller.
func control(f *os.File, fn func(fd int) error) error {
	rc, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var ferr error
	if err := rc.Control(func(fd uintptr) { ferr = fn(int(fd)) }); err != nil {
		return err
	}
	return ferr
}

func openPort(path string, baud, dataBits int64, parity string, stopBits int64) (Value, error) {
	// O_NOCTTY: opening a tty must not make it our controlling terminal.
	// O_NONBLOCK: on a tty with CLOCAL clear (UARTs, modems) a blocking open
	// waits for carrier detect, before configure can set CLOCAL.
	f, err := os.OpenFile(path, os.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK, 0)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("serial.open: %s: %v", path, err)
	}
	if err := control(f, func(fd int) error { return configure(fd, baud, dataBits, parity, stopBits) }); err != nil {
		_ = f.Close()
		return interpreter.Null(), err
	}
	portsMu.Lock()
	nextID++
	id := nextID
	ports[id] = &portState{f: f}
	portsMu.Unlock()
	return devio.Handle(LibraryName, "Port", id), nil
}

// serial.open(path, baud) -> serial.Port, the 8N1 shorthand.
func openFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("serial.open", args, 2); err != nil {
		return interpreter.Null(), err
	}
	path, err := devio.StringArg("serial.open", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	baud, err := devio.IntArg("serial.open", args, 1, "baud")
	if err != nil {
		return interpreter.Null(), err
	}
	return openPort(path, baud, 8, "none", 1)
}

// serial.openWith(path, serial.Options) -> serial.Port.
func openWithFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("serial.openWith", args, 2); err != nil {
		return interpreter.Null(), err
	}
	path, err := devio.StringArg("serial.openWith", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	opts := args[1]
	if opts.Kind != interpreter.KindStruct || opts.StructNS != LibraryName || opts.StructName != "Options" {
		return interpreter.Null(), fmt.Errorf("serial.openWith: second argument must be a serial.Options, got %s", opts.Kind)
	}
	// An omitted field is its zero value in the struct literal: 0 / "" take
	// the 9600 8N1 default.
	baud, dataBits, stopBits := int64(9600), int64(8), int64(1)
	parity := "none"
	for _, f := range opts.Fields {
		switch f.Name {
		case "baud":
			if f.Value.Int != 0 {
				baud = f.Value.Int
			}
		case "dataBits":
			if f.Value.Int != 0 {
				dataBits = f.Value.Int
			}
		case "parity":
			if f.Value.Str != "" {
				parity = f.Value.Str
			}
		case "stopBits":
			if f.Value.Int != 0 {
				stopBits = f.Value.Int
			}
		}
	}
	return openPort(path, baud, dataBits, parity, stopBits)
}

// serial.read(port, n) -> bytes. Blocks until at least one byte arrives (VMIN=1),
// then returns up to n bytes. A read deadline (serial.setReadDeadline) that
// passes makes it throw kind "timeout"; a close from another task, kind
// "closed"; task.cancel of the calling task wakes it with "task cancelled".
func readFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("serial.read", args, 2); err != nil {
		return interpreter.Null(), err
	}
	p, err := resolvePort("serial.read", args)
	if err != nil {
		return interpreter.Null(), err
	}
	n, err := devio.IntArg("serial.read", args, 1, "n")
	if err != nil {
		return interpreter.Null(), err
	}
	sz, err := devio.ReadSize("serial.read", n)
	if err != nil {
		return interpreter.Null(), err
	}
	// A tty read returns at most its line-discipline buffer (4 KiB).
	buf := make([]byte, min(sz, 64<<10))
	stop := watchCancel(ctx, p)
	got, rerr := p.f.Read(buf)
	if stop() {
		p.mu.Lock()
		_ = p.f.SetReadDeadline(p.deadline)
		p.mu.Unlock()
		return interpreter.Null(), interpreter.ErrTaskCancelled()
	}
	if rerr == io.EOF { // hangup: no data, as before
		rerr = nil
	}
	if rerr != nil {
		return interpreter.Null(), failure(ctx, "serial.read", "read", rerr)
	}
	return interpreter.BytesVal(buf[:got]), nil
}

// serial.write(port, data) -> int (bytes written). A write deadline
// (serial.setWriteDeadline) that passes makes it throw kind "timeout".
func writeFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("serial.write", args, 2); err != nil {
		return interpreter.Null(), err
	}
	p, err := resolvePort("serial.write", args)
	if err != nil {
		return interpreter.Null(), err
	}
	data, err := devio.BytesArg("serial.write", args, 1, "data")
	if err != nil {
		return interpreter.Null(), err
	}
	wrote, werr := p.f.Write(data)
	if werr != nil {
		return interpreter.Null(), failure(ctx, "serial.write", "write", werr)
	}
	return interpreter.IntVal(int64(wrote)), nil
}

// applyDeadline is the body of serial.setReadDeadline / setWriteDeadline: a
// positive ms arms a deadline that many milliseconds from now, after which a
// pending or later operation in that direction throws kind "timeout" until the
// deadline is reset; 0 clears it. Read and write deadlines are separate, as in
// net, so a write after a read timeout still goes out.
func applyDeadline(fn string, args []Value, read bool) (Value, error) {
	if err := devio.WantArgs(fn, args, 2); err != nil {
		return interpreter.Null(), err
	}
	p, err := resolvePort(fn, args)
	if err != nil {
		return interpreter.Null(), err
	}
	ms, err := devio.IntArg(fn, args, 1, "ms")
	if err != nil {
		return interpreter.Null(), err
	}
	if ms < 0 {
		return interpreter.Null(), fmt.Errorf("%s: ms must be 0 (clear) or positive, got %d", fn, ms)
	}
	var when time.Time // the zero Time clears the deadline
	if ms > 0 {
		when = time.Now().Add(clampMillis(ms))
	}
	var derr error
	if read {
		p.mu.Lock()
		p.deadline = when
		derr = p.f.SetReadDeadline(when)
		p.mu.Unlock()
	} else {
		derr = p.f.SetWriteDeadline(when)
	}
	if derr != nil {
		return interpreter.Null(), fmt.Errorf("%s: %v", fn, derr)
	}
	return interpreter.Null(), nil
}

// serial.setReadDeadline(port, ms) -> null. The poll-with-timeout primitive.
func setReadDeadlineFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	return applyDeadline("serial.setReadDeadline", args, true)
}

// serial.setWriteDeadline(port, ms) -> null. Bounds a stalled send (flow
// control holding the line); reads are untouched.
func setWriteDeadlineFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	return applyDeadline("serial.setWriteDeadline", args, false)
}

// serial.flush(port) -> null. Discards buffered input and output (TCIOFLUSH).
func flushFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("serial.flush", args, 1); err != nil {
		return interpreter.Null(), err
	}
	p, err := resolvePort("serial.flush", args)
	if err != nil {
		return interpreter.Null(), err
	}
	if ferr := control(p.f, func(fd int) error { return unix.IoctlSetInt(fd, unix.TCFLSH, unix.TCIOFLUSH) }); ferr != nil {
		return interpreter.Null(), fmt.Errorf("serial.flush: %v", ferr)
	}
	return interpreter.Null(), nil
}

// serial.close(port) -> null.
func closeFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("serial.close", args, 1); err != nil {
		return interpreter.Null(), err
	}
	id, err := devio.HandleID("serial.close", args[0], LibraryName, "Port")
	if err != nil {
		return interpreter.Null(), err
	}
	portsMu.Lock()
	p, ok := ports[id]
	if !ok {
		portsMu.Unlock()
		return interpreter.Null(), fmt.Errorf("serial.close: serial.Port id %d is not open (already closed?)", id)
	}
	delete(ports, id)
	portsMu.Unlock()
	if cerr := p.f.Close(); cerr != nil {
		return interpreter.Null(), fmt.Errorf("serial.close: %v", cerr)
	}
	return interpreter.Null(), nil
}
