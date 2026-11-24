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
	// exclusive: configure set TIOCEXCL, which close clears again.
	exclusive bool
	// marked: parity is on, so the line discipline marks a byte received
	// with a parity or framing error (PARMRK) and read decodes the marks.
	marked bool
	// rmu serialises marked reads and guards the decode state below.
	rmu sync.Mutex
	dec parityDecoder
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
		return nil, interpreter.KindedError("closed", fmt.Sprintf("%s: serial.Port id %d is not open (already closed, or never opened)", fn, id))
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
		1500000: unix.B1500000, 2000000: unix.B2000000, 2500000: unix.B2500000,
		3000000: unix.B3000000, 3500000: unix.B3500000, 4000000: unix.B4000000,
	}
	c, ok := m[baud]
	return c, ok
}

// lineConfig is a validated line setting: everything configure needs, checked
// before the device is opened (opening a tty raises DTR / RTS, which resets an
// Arduino-style board, so a bad argument must not get that far).
type lineConfig struct {
	speed    uint32 // the Bxxx code
	csize    uint32 // CS5..CS8
	parity   string // "none", "even", "odd"
	stopBits int64
	flow     string // "none", "rtscts", "xonxoff"
	shared   bool   // skip TIOCEXCL
}

// newLineConfig validates the line settings; fn prefixes the errors.
func newLineConfig(fn string, baud, dataBits int64, parity string, stopBits int64, flow string, shared bool) (lineConfig, error) {
	c := lineConfig{parity: parity, stopBits: stopBits, flow: flow, shared: shared}
	code, ok := baudCode(baud)
	if !ok {
		return c, fmt.Errorf("%s: unsupported baud rate %d (use a standard rate, e.g. 9600 / 19200 / 115200)", fn, baud)
	}
	c.speed = code
	switch dataBits {
	case 5:
		c.csize = unix.CS5
	case 6:
		c.csize = unix.CS6
	case 7:
		c.csize = unix.CS7
	case 8:
		c.csize = unix.CS8
	default:
		return c, fmt.Errorf("%s: data bits must be 5, 6, 7, or 8, got %d", fn, dataBits)
	}
	switch parity {
	case "none", "even", "odd":
	default:
		return c, fmt.Errorf(`%s: parity must be "none", "even", or "odd", got %q`, fn, parity)
	}
	if stopBits != 1 && stopBits != 2 {
		return c, fmt.Errorf("%s: stop bits must be 1 or 2, got %d", fn, stopBits)
	}
	switch flow {
	case "none", "rtscts", "xonxoff":
	default:
		return c, fmt.Errorf(`%s: flowControl must be "none", "rtscts", or "xonxoff", got %q`, fn, flow)
	}
	return c, nil
}

// makeRaw rewrites t for c: raw mode (as cfmakeraw, plus every flag a previous
// user of the port may have left set that would alter the byte stream: flow
// control, input parity checking, case mapping, mark/space parity, a separate
// input speed) and then the requested line settings.
func makeRaw(t *unix.Termios, c lineConfig) {
	t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.IGNPAR | unix.PARMRK | unix.INPCK | unix.ISTRIP |
		unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IUCLC | unix.IXON | unix.IXANY | unix.IXOFF | unix.IMAXBEL
	t.Oflag &^= unix.OPOST
	t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	t.Cflag &^= unix.CSIZE | unix.PARENB | unix.PARODD | unix.CMSPAR | unix.CSTOPB | unix.CBAUD | unix.CIBAUD | unix.CRTSCTS
	t.Cflag |= unix.CLOCAL | unix.CREAD | c.csize | c.speed
	switch c.parity {
	case "even":
		t.Cflag |= unix.PARENB
		t.Iflag |= unix.INPCK | unix.PARMRK
	case "odd":
		t.Cflag |= unix.PARENB | unix.PARODD
		t.Iflag |= unix.INPCK | unix.PARMRK
	}
	if c.stopBits == 2 {
		t.Cflag |= unix.CSTOPB
	}
	switch c.flow {
	case "rtscts":
		t.Cflag |= unix.CRTSCTS
	case "xonxoff":
		t.Iflag |= unix.IXON | unix.IXOFF
		t.Cc[unix.VSTART] = 0x11 // DC1, ^Q
		t.Cc[unix.VSTOP] = 0x13  // DC3, ^S
	}
	t.Ispeed = c.speed
	t.Ospeed = c.speed
	// Blocking read: return as soon as at least one byte is available.
	t.Cc[unix.VMIN] = 1
	t.Cc[unix.VTIME] = 0
}

// configure applies raw mode plus the line settings to fd's termios, then
// claims the port exclusively (TIOCEXCL) unless c.shared: another non-root
// open of the port then fails with EBUSY instead of interleaving its bytes.
func configure(fd int, c lineConfig) error {
	t, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return err
	}
	makeRaw(t, c)
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, t); err != nil {
		return err
	}
	if !c.shared {
		if err := unix.IoctlSetInt(fd, unix.TIOCEXCL, 0); err != nil {
			return fmt.Errorf("claiming the port exclusively: %w", err)
		}
	}
	return nil
}

// explain is err's OS text plus a hint for the errno values a serial port
// commonly hits. A *PathError's path is dropped: callers name the path.
func explain(err error) string {
	var en unix.Errno
	if !errors.As(err, &en) {
		return err.Error()
	}
	switch en {
	case unix.EACCES, unix.EPERM:
		return en.Error() + " (add the user to the group that owns the device, often dialout or uucp)"
	case unix.EBUSY:
		return en.Error() + " (another program has the port open exclusively)"
	case unix.ENXIO, unix.ENODEV:
		return en.Error() + " (unplugged, or no UART behind this device node)"
	case unix.ENOTTY:
		return en.Error() + " (not a serial port)"
	}
	return en.Error()
}

// hungUp reports whether a read / write error means the port is gone: a tty
// that was hung up (USB adapter unplugged, the far end of a pty closed) fails
// writes with EIO; ENXIO / ENODEV come from a vanished device.
func hungUp(err error) bool {
	return errors.Is(err, unix.EIO) || errors.Is(err, unix.ENXIO) || errors.Is(err, unix.ENODEV)
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

func openPort(fn, path string, c lineConfig) (Value, error) {
	// O_NOCTTY: opening a tty must not make it our controlling terminal.
	// O_NONBLOCK: on a tty with CLOCAL clear (UARTs, modems) a blocking open
	// waits for carrier detect, before configure can set CLOCAL.
	f, err := os.OpenFile(path, os.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK, 0)
	if err != nil {
		return interpreter.Null(), fmt.Errorf("%s: %s: %s", fn, path, explain(err))
	}
	if err := control(f, func(fd int) error { return configure(fd, c) }); err != nil {
		_ = f.Close()
		return interpreter.Null(), fmt.Errorf("%s: %s: configuring the port: %s", fn, path, explain(err))
	}
	portsMu.Lock()
	nextID++
	id := nextID
	ports[id] = &portState{f: f, exclusive: !c.shared, marked: c.parity != "none"}
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
	c, err := newLineConfig("serial.open", baud, 8, "none", 1, "none", false)
	if err != nil {
		return interpreter.Null(), err
	}
	return openPort("serial.open", path, c)
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
	if opts.Kind != interpreter.KindStruct || opts.StructNS() != LibraryName || opts.StructName() != "Options" {
		return interpreter.Null(), fmt.Errorf("serial.openWith: second argument must be a serial.Options, got %s", opts.Kind)
	}
	// An omitted field is its zero value in the struct literal: 0 / "" / false
	// take the 9600 8N1 default, no flow control, exclusive.
	baud, dataBits, stopBits := int64(9600), int64(8), int64(1)
	parity, flow, shared := "none", "none", false
	for _, f := range opts.Fields() {
		switch f.Name {
		case "baud":
			if f.Value.Int() != 0 {
				baud = f.Value.Int()
			}
		case "dataBits":
			if f.Value.Int() != 0 {
				dataBits = f.Value.Int()
			}
		case "parity":
			if f.Value.Str() != "" {
				parity = f.Value.Str()
			}
		case "stopBits":
			if f.Value.Int() != 0 {
				stopBits = f.Value.Int()
			}
		case "flowControl":
			if f.Value.Str() != "" {
				flow = f.Value.Str()
			}
		case "shared":
			shared = f.Value.Bool()
		}
	}
	c, err := newLineConfig("serial.openWith", baud, dataBits, parity, stopBits, flow, shared)
	if err != nil {
		return interpreter.Null(), err
	}
	return openPort("serial.openWith", path, c)
}

// serial.read(port, n) -> bytes. Blocks until at least one byte arrives (VMIN=1),
// then returns up to n bytes. A read deadline (serial.setReadDeadline) that
// passes makes it throw kind "timeout"; a close from another task or a hangup,
// kind "closed"; task.cancel of the calling task wakes it with "task cancelled".
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
	if p.marked {
		return readMarked(ctx, p, sz)
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
	// With VMIN=1 a read returns 0 bytes (io.EOF) only once the tty is hung
	// up; a pty whose master closed fails with EIO instead. Either way no byte
	// will come again, so returning empty bytes would make a read loop spin.
	if rerr == io.EOF || (rerr != nil && hungUp(rerr)) {
		return interpreter.Null(), interpreter.RaiseError("closed", "serial.read: port hung up (device unplugged or the other end closed)", ctx.File, ctx.Line, ctx.Col)
	}
	if rerr != nil {
		return interpreter.Null(), failure(ctx, "serial.read", "read", rerr)
	}
	return interpreter.BytesVal(buf[:got]), nil
}

// serial.write(port, data) -> null; every byte is written or it throws. A write deadline
// (serial.setWriteDeadline) that passes makes it throw kind "timeout"; a
// hung-up port, kind "closed".
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
	_, werr := p.f.Write(data)
	if werr != nil && hungUp(werr) {
		return interpreter.Null(), interpreter.RaiseError("closed", "serial.write: port hung up (device unplugged or the other end closed)", ctx.File, ctx.Line, ctx.Col)
	}
	if werr != nil {
		return interpreter.Null(), failure(ctx, "serial.write", "write", werr)
	}
	return interpreter.Null(), nil
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

// serial.close(port) -> null. A second close of the same port does nothing.
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
		return interpreter.Null(), nil
	}
	delete(ports, id)
	portsMu.Unlock()
	if p.exclusive {
		// Give up the exclusive claim first: a pty slave (and any tty whose
		// other side stays open) keeps TIOCEXCL past this close, so a later
		// open of the same path would fail with EBUSY. On a hung-up port
		// this fails harmlessly.
		_ = control(p.f, func(fd int) error { return unix.IoctlSetInt(fd, unix.TIOCNXCL, 0) })
	}
	if cerr := p.f.Close(); cerr != nil {
		return interpreter.Null(), fmt.Errorf("serial.close: %v", cerr)
	}
	return interpreter.Null(), nil
}
