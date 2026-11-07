// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build linux && !tinygo

// Linux implementation of the `i2c` library over golang.org/x/sys/unix.

package i2clib

import (
	"fmt"
	"os"
	"sync"

	"golang.org/x/sys/unix"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/devio"
)

// i2cSlave is the I2C_SLAVE ioctl request: select the 7-bit slave address this
// fd talks to. (Value from <linux/i2c-dev.h>; x/sys/unix does not export it.)
const i2cSlave = 0x0703

// maxRead is i2c-dev's per-read clamp (i2cdev_read: count > 8192 -> 8192); a
// larger buffer would only be allocated and left unused.
const maxRead = 8192

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

func resolveBus(fn string, args []Value) (*busState, error) {
	id, err := devio.HandleID(fn, args[0], LibraryName, "Bus")
	if err != nil {
		return nil, err
	}
	busesMu.Lock()
	defer busesMu.Unlock()
	b, ok := buses[id]
	if !ok {
		return nil, fmt.Errorf("%s: i2c.Bus id %d is not open (already closed, or never opened)", fn, id)
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
		return interpreter.Null(), fmt.Errorf("i2c.open: %s: %v", path, oerr)
	}
	if serr := unix.IoctlSetInt(int(f.Fd()), i2cSlave, int(addr)); serr != nil {
		_ = f.Close()
		return interpreter.Null(), fmt.Errorf("i2c.open: selecting address 0x%02x: %v", addr, serr)
	}
	busesMu.Lock()
	nextID++
	id := nextID
	buses[id] = &busState{f: f, addr: addr}
	busesMu.Unlock()
	return devio.Handle(LibraryName, "Bus", id), nil
}

// i2c.read(bus, n) -> bytes.
func readFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
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
	buf := make([]byte, min(sz, maxRead))
	b.mu.Lock()
	defer b.mu.Unlock()
	got, rerr := unix.Read(int(b.f.Fd()), buf)
	if rerr != nil {
		return interpreter.Null(), fmt.Errorf("i2c.read: %v", rerr)
	}
	if got < 0 {
		got = 0
	}
	return interpreter.BytesVal(buf[:got]), nil
}

// i2c.write(bus, data) -> int (bytes written).
func writeFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
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
	b.mu.Lock()
	defer b.mu.Unlock()
	wrote, werr := unix.Write(int(b.f.Fd()), data)
	if werr != nil {
		return interpreter.Null(), fmt.Errorf("i2c.write: %v", werr)
	}
	return interpreter.IntVal(int64(wrote)), nil
}

// i2c.readReg(bus, reg, n) -> bytes. Writes the 1-byte register pointer, then
// reads n bytes as a separate transfer (a STOP between them, no repeated
// start); no other transfer on this bus runs in between.
func readRegFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
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
	// Two transfers (a STOP between them), held as one unit against other
	// tasks on this bus.
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, werr := unix.Write(int(b.f.Fd()), []byte{byte(reg)}); werr != nil {
		return interpreter.Null(), fmt.Errorf("i2c.readReg: selecting register 0x%02x: %v", reg, werr)
	}
	buf := make([]byte, min(sz, maxRead))
	got, rerr := unix.Read(int(b.f.Fd()), buf)
	if rerr != nil {
		return interpreter.Null(), fmt.Errorf("i2c.readReg: %v", rerr)
	}
	if got < 0 {
		got = 0
	}
	return interpreter.BytesVal(buf[:got]), nil
}

// i2c.writeReg(bus, reg, data) -> int (data bytes written, not counting the
// register pointer). Writes the register pointer and data in one transaction.
func writeRegFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
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
	frame := make([]byte, 0, len(data)+1)
	frame = append(frame, byte(reg))
	frame = append(frame, data...)
	b.mu.Lock()
	defer b.mu.Unlock()
	wrote, werr := unix.Write(int(b.f.Fd()), frame)
	if werr != nil {
		return interpreter.Null(), fmt.Errorf("i2c.writeReg: %v", werr)
	}
	// Report data bytes written, excluding the register pointer.
	if wrote > 0 {
		wrote--
	}
	return interpreter.IntVal(int64(wrote)), nil
}

// i2c.close(bus) -> null.
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
		return interpreter.Null(), fmt.Errorf("i2c.close: i2c.Bus id %d is not open (already closed?)", id)
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
