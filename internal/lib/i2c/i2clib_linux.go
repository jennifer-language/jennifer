// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build linux && !tinygo

// Linux implementation of the `i2c` library over golang.org/x/sys/unix.

package i2clib

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/devio"
)

// i2cSlave is the I2C_SLAVE ioctl request: select the 7-bit slave address this
// fd talks to. (Value from <linux/i2c-dev.h>; x/sys/unix does not export it.)
// I2C_SLAVE_FORCE (0x0706) is deliberately not used: it would talk past a
// kernel driver that has bound the address.
const i2cSlave = 0x0703

// i2cRdwr is the I2C_RDWR ioctl: several messages as one combined transfer,
// with a repeated START (no STOP) between them. i2cMRd marks a read message.
const (
	i2cRdwr = 0x0707
	i2cMRd  = 0x0001
)

// i2cMsg mirrors struct i2c_msg from <linux/i2c.h>: three __u16 and a buffer
// pointer, so 12 bytes on 32-bit targets and 16 on 64-bit ones (Go aligns the
// pointer as C does). The pointer is a real Go pointer, so the buffer stays
// reachable; runtime.KeepAlive covers the call itself.
type i2cMsg struct {
	addr  uint16
	flags uint16
	len   uint16
	buf   *byte
}

// i2cRdwrData mirrors struct i2c_rdwr_ioctl_data: the message array and its
// count (8 bytes on 32-bit targets, 16 on 64-bit).
type i2cRdwrData struct {
	msgs  *i2cMsg
	nmsgs uint32
}

// rdwr issues I2C_RDWR and returns the number of messages the kernel ran; a
// variable so tests can stand in for the adapter.
var rdwr = func(fd int, d *i2cRdwrData) (int, error) {
	r, _, e := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), i2cRdwr, uintptr(unsafe.Pointer(d)))
	if e != 0 {
		return 0, e
	}
	return int(r), nil
}

// maxRead is i2c-dev's per-message limit (i2cdev_read / i2cdev_write clamp a
// larger count to 8192, I2C_RDWR refuses one). A longer read or write throws
// kind "limit" rather than transferring a silently shortened message (M29.49).
const maxRead = 8192

// overMessage is the "limit" error for a message longer than maxRead.
func overMessage(ctx interpreter.BuiltinCtx, fn, what string, n int64) error {
	return interpreter.RaiseError("limit", fmt.Sprintf("%s: a message is at most %d bytes (i2c-dev), got %d %s", fn, maxRead, n, what), ctx.File, ctx.Line, ctx.Col)
}

// busState is one open bus. mu serialises the transfers on it, so readReg's
// pointer write and read are not interleaved with another task's transfer,
// and close waits for a transfer in flight (whose fd number could otherwise be
// reused by a later open).
type busState struct {
	mu   sync.Mutex
	f    *os.File
	addr int64
}

var (
	busesMu sync.Mutex
	buses   = map[int64]*busState{}
	nextID  int64
)

// ResetForTest wipes the registry between tests.
func ResetForTest() {
	busesMu.Lock()
	defer busesMu.Unlock()
	for _, b := range buses {
		if b != nil && b.f != nil {
			_ = b.f.Close()
		}
	}
	buses = map[int64]*busState{}
	nextID = 0
}

// failure turns a transfer error into what a program sees: an adapter
// timeout is Error{kind: "timeout"}; anything else a runtime error with the
// OS text and a hint for the errno values a bus commonly returns.
func failure(ctx interpreter.BuiltinCtx, fn string, addr int64, err error) error {
	if errors.Is(err, unix.ETIMEDOUT) {
		return interpreter.RaiseError("timeout", fmt.Sprintf("%s: address 0x%02x: bus timed out", fn, addr), ctx.File, ctx.Line, ctx.Col)
	}
	return fmt.Errorf("%s: address 0x%02x: %s", fn, addr, explain(err))
}

// explain is err's OS text plus a hint for the errno values I2C commonly hits.
// A *PathError's path is dropped: callers name it.
func explain(err error) string {
	var en unix.Errno
	if !errors.As(err, &en) {
		return err.Error()
	}
	switch en {
	case unix.EACCES, unix.EPERM:
		return en.Error() + " (add the user to the group that owns the device, often i2c)"
	case unix.EBUSY:
		return en.Error() + " (a kernel driver has bound this address; unbind it first, I2C_SLAVE_FORCE is not used)"
	case unix.ENXIO, unix.EREMOTEIO:
		return en.Error() + " (no ACK: nothing at this address, or the device is busy)"
	case unix.ENODEV:
		return en.Error() + " (no I2C adapter behind this device node; is I2C enabled?)"
	case unix.ENOTTY:
		return en.Error() + " (not an i2c-dev device)"
	case unix.EOPNOTSUPP:
		return en.Error() + " (the adapter does not support this transfer)"
	}
	return en.Error()
}

func resolveBus(fn string, args []Value) (*busState, error) {
	id, err := devio.HandleID(fn, args[0], LibraryName, "Bus")
	if err != nil {
		return nil, err
	}
	busesMu.Lock()
	defer busesMu.Unlock()
	b, ok := buses[id]
	if !ok {
		return nil, interpreter.KindedError("closed", fmt.Sprintf("%s: i2c.Bus id %d is not open (already closed, or never opened)", fn, id))
	}
	return b, nil
}

// i2c.open(path, addr) -> i2c.Bus. addr is a 7-bit slave address (0x08..0x77 is
// the usable range; 0..0x7f is accepted).
func openFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("i2c.open", args, 2); err != nil {
		return interpreter.Null(), err
	}
	path, err := devio.StringArg("i2c.open", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	addr, err := devio.IntArg("i2c.open", args, 1, "addr")
	if err != nil {
		return interpreter.Null(), err
	}
	if addr < 0 || addr > 0x7f {
		return interpreter.Null(), fmt.Errorf("i2c.open: slave address %d out of 7-bit range (0..127)", addr)
	}
	f, oerr := os.OpenFile(path, os.O_RDWR, 0)
	if oerr != nil {
		return interpreter.Null(), fmt.Errorf("i2c.open: %s: %s", path, explain(oerr))
	}
	if serr := unix.IoctlSetInt(int(f.Fd()), i2cSlave, int(addr)); serr != nil {
		_ = f.Close()
		return interpreter.Null(), fmt.Errorf("i2c.open: %s: selecting address 0x%02x: %s", path, addr, explain(serr))
	}
	busesMu.Lock()
	nextID++
	id := nextID
	buses[id] = &busState{f: f, addr: addr}
	busesMu.Unlock()
	return devio.Handle(LibraryName, "Bus", id), nil
}

// i2c.read(bus, n) -> bytes.
func readFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("i2c.read", args, 2); err != nil {
		return interpreter.Null(), err
	}
	b, err := resolveBus("i2c.read", args)
	if err != nil {
		return interpreter.Null(), err
	}
	n, err := devio.IntArg("i2c.read", args, 1, "n")
	if err != nil {
		return interpreter.Null(), err
	}
	sz, err := devio.ReadSize("i2c.read", n)
	if err != nil {
		return interpreter.Null(), err
	}
	if sz > maxRead {
		return interpreter.Null(), overMessage(ctx, "i2c.read", "to read", n)
	}
	buf := make([]byte, sz)
	b.mu.Lock()
	defer b.mu.Unlock()
	got, rerr := unix.Read(int(b.f.Fd()), buf)
	if rerr != nil {
		return interpreter.Null(), failure(ctx, "i2c.read", b.addr, rerr)
	}
	if got < 0 {
		got = 0
	}
	return interpreter.BytesVal(buf[:got]), nil
}

// i2c.write(bus, data) -> int (bytes written).
func writeFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("i2c.write", args, 2); err != nil {
		return interpreter.Null(), err
	}
	b, err := resolveBus("i2c.write", args)
	if err != nil {
		return interpreter.Null(), err
	}
	data, err := devio.BytesArg("i2c.write", args, 1, "data")
	if err != nil {
		return interpreter.Null(), err
	}
	if len(data) > maxRead {
		return interpreter.Null(), overMessage(ctx, "i2c.write", "to write", int64(len(data)))
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	wrote, werr := unix.Write(int(b.f.Fd()), data)
	if werr != nil {
		return interpreter.Null(), failure(ctx, "i2c.write", b.addr, werr)
	}
	return interpreter.IntVal(int64(wrote)), nil
}

// i2c.readReg(bus, reg, n) -> bytes. Writes the 1-byte register pointer, then
// reads n bytes as a separate transfer (a STOP between them, no repeated
// start); no other transfer on this bus runs in between.
func readRegFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("i2c.readReg", args, 3); err != nil {
		return interpreter.Null(), err
	}
	b, err := resolveBus("i2c.readReg", args)
	if err != nil {
		return interpreter.Null(), err
	}
	reg, err := devio.IntArg("i2c.readReg", args, 1, "reg")
	if err != nil {
		return interpreter.Null(), err
	}
	n, err := devio.IntArg("i2c.readReg", args, 2, "n")
	if err != nil {
		return interpreter.Null(), err
	}
	if reg < 0 || reg > 0xff {
		return interpreter.Null(), fmt.Errorf("i2c.readReg: register %d out of byte range (0..255)", reg)
	}
	sz, err := devio.ReadSize("i2c.readReg", n)
	if err != nil {
		return interpreter.Null(), err
	}
	if sz > maxRead {
		return interpreter.Null(), overMessage(ctx, "i2c.readReg", "to read", n)
	}
	// Two transfers (a STOP between them), held as one unit against other
	// tasks on this bus.
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, werr := unix.Write(int(b.f.Fd()), []byte{byte(reg)}); werr != nil {
		return interpreter.Null(), failure(ctx, fmt.Sprintf("i2c.readReg: selecting register 0x%02x", reg), b.addr, werr)
	}
	buf := make([]byte, sz)
	got, rerr := unix.Read(int(b.f.Fd()), buf)
	if rerr != nil {
		return interpreter.Null(), failure(ctx, "i2c.readReg", b.addr, rerr)
	}
	if got < 0 {
		got = 0
	}
	return interpreter.BytesVal(buf[:got]), nil
}

// i2c.writeReg(bus, reg, data) -> int (data bytes written, not counting the
// register pointer). Writes the register pointer and data in one transaction.
func writeRegFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("i2c.writeReg", args, 3); err != nil {
		return interpreter.Null(), err
	}
	b, err := resolveBus("i2c.writeReg", args)
	if err != nil {
		return interpreter.Null(), err
	}
	reg, err := devio.IntArg("i2c.writeReg", args, 1, "reg")
	if err != nil {
		return interpreter.Null(), err
	}
	data, err := devio.BytesArg("i2c.writeReg", args, 2, "data")
	if err != nil {
		return interpreter.Null(), err
	}
	if reg < 0 || reg > 0xff {
		return interpreter.Null(), fmt.Errorf("i2c.writeReg: register %d out of byte range (0..255)", reg)
	}
	if len(data)+1 > maxRead {
		return interpreter.Null(), overMessage(ctx, "i2c.writeReg", "to write (the register pointer and the data)", int64(len(data)+1))
	}
	frame := make([]byte, 0, len(data)+1)
	frame = append(frame, byte(reg))
	frame = append(frame, data...)
	b.mu.Lock()
	defer b.mu.Unlock()
	wrote, werr := unix.Write(int(b.f.Fd()), frame)
	if werr != nil {
		return interpreter.Null(), failure(ctx, "i2c.writeReg", b.addr, werr)
	}
	// Report data bytes written, excluding the register pointer.
	if wrote > 0 {
		wrote--
	}
	return interpreter.IntVal(int64(wrote)), nil
}

// i2c.writeRead(bus, data, n) -> bytes. Writes data, then reads exactly n
// bytes, as one combined transfer (I2C_RDWR): a repeated START between the two
// messages, no STOP, which is what a device that loses its register pointer at
// a STOP needs. Each message is at most 8192 bytes (i2c-dev's limit).
func writeReadFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("i2c.writeRead", args, 3); err != nil {
		return interpreter.Null(), err
	}
	b, err := resolveBus("i2c.writeRead", args)
	if err != nil {
		return interpreter.Null(), err
	}
	data, err := devio.BytesArg("i2c.writeRead", args, 1, "data")
	if err != nil {
		return interpreter.Null(), err
	}
	n, err := devio.IntArg("i2c.writeRead", args, 2, "n")
	if err != nil {
		return interpreter.Null(), err
	}
	if len(data) == 0 {
		return interpreter.Null(), fmt.Errorf("i2c.writeRead: data must not be empty (use i2c.read for a plain read)")
	}
	if n < 1 {
		return interpreter.Null(), fmt.Errorf("i2c.writeRead: n must be at least 1, got %d", n)
	}
	if len(data) > maxRead || n > maxRead {
		return interpreter.Null(), interpreter.RaiseError("limit", fmt.Sprintf("i2c.writeRead: a message is at most %d bytes (i2c-dev), got %d to write and %d to read", maxRead, len(data), n), ctx.File, ctx.Line, ctx.Col)
	}
	rx := make([]byte, n)
	b.mu.Lock()
	defer b.mu.Unlock()
	msgs := []i2cMsg{
		{addr: uint16(b.addr), len: uint16(len(data)), buf: &data[0]},
		{addr: uint16(b.addr), flags: i2cMRd, len: uint16(n), buf: &rx[0]},
	}
	req := i2cRdwrData{msgs: &msgs[0], nmsgs: uint32(len(msgs))}
	ran, ierr := rdwr(int(b.f.Fd()), &req)
	runtime.KeepAlive(msgs)
	runtime.KeepAlive(data)
	runtime.KeepAlive(rx)
	if ierr != nil {
		return interpreter.Null(), failure(ctx, "i2c.writeRead", b.addr, ierr)
	}
	if ran != len(msgs) {
		return interpreter.Null(), fmt.Errorf("i2c.writeRead: address 0x%02x: the adapter ran %d of %d messages", b.addr, ran, len(msgs))
	}
	return interpreter.BytesVal(rx), nil
}

// i2c.close(bus) -> null. A second close of the same bus does nothing.
func closeFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("i2c.close", args, 1); err != nil {
		return interpreter.Null(), err
	}
	id, err := devio.HandleID("i2c.close", args[0], LibraryName, "Bus")
	if err != nil {
		return interpreter.Null(), err
	}
	busesMu.Lock()
	b, ok := buses[id]
	if !ok {
		busesMu.Unlock()
		return interpreter.Null(), nil
	}
	delete(buses, id)
	busesMu.Unlock()
	b.mu.Lock() // wait for a transfer in flight
	defer b.mu.Unlock()
	if cerr := b.f.Close(); cerr != nil {
		return interpreter.Null(), fmt.Errorf("i2c.close: %v", cerr)
	}
	return interpreter.Null(), nil
}
