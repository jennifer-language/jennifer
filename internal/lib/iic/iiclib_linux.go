// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

//go:build linux && !tinygo

// Linux implementation of the `iic` library over golang.org/x/sys/unix.

package iiclib

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

type busState struct {
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
		return nil, fmt.Errorf("%s: iic.Bus id %d is not open (already closed, or never opened)", fn, id)
	}
	return b, nil
}

// iic.open(path, addr) -> iic.Bus. addr is a 7-bit slave address (0x08..0x77 is
// the usable range; 0..0x7f is accepted).
func openFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("iic.open", args, 2); err != nil {
		return interpreter.Null(), err
	}
	path, err := devio.StringArg("iic.open", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	addr, err := devio.IntArg("iic.open", args, 1, "addr")
	if err != nil {
		return interpreter.Null(), err
	}
	if addr < 0 || addr > 0x7f {
		return interpreter.Null(), fmt.Errorf("iic.open: slave address %d out of 7-bit range (0..127)", addr)
	}
	f, oerr := os.OpenFile(path, os.O_RDWR, 0)
	if oerr != nil {
		return interpreter.Null(), fmt.Errorf("iic.open: %s: %v", path, oerr)
	}
	if serr := unix.IoctlSetInt(int(f.Fd()), i2cSlave, int(addr)); serr != nil {
		_ = f.Close()
		return interpreter.Null(), fmt.Errorf("iic.open: selecting address 0x%02x: %v", addr, serr)
	}
	busesMu.Lock()
	nextID++
	id := nextID
	buses[id] = &busState{f: f, addr: addr}
	busesMu.Unlock()
	return devio.Handle(LibraryName, "Bus", id), nil
}

// iic.read(bus, n) -> bytes.
func readFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("iic.read", args, 2); err != nil {
		return interpreter.Null(), err
	}
	b, err := resolveBus("iic.read", args)
	if err != nil {
		return interpreter.Null(), err
	}
	n, err := devio.IntArg("iic.read", args, 1, "n")
	if err != nil {
		return interpreter.Null(), err
	}
	sz, err := devio.ReadSize("iic.read", n)
	if err != nil {
		return interpreter.Null(), err
	}
	buf := make([]byte, sz)
	got, rerr := unix.Read(int(b.f.Fd()), buf)
	if rerr != nil {
		return interpreter.Null(), fmt.Errorf("iic.read: %v", rerr)
	}
	if got < 0 {
		got = 0
	}
	return interpreter.BytesVal(buf[:got]), nil
}

// iic.write(bus, data) -> int (bytes written).
func writeFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("iic.write", args, 2); err != nil {
		return interpreter.Null(), err
	}
	b, err := resolveBus("iic.write", args)
	if err != nil {
		return interpreter.Null(), err
	}
	data, err := devio.BytesArg("iic.write", args, 1, "data")
	if err != nil {
		return interpreter.Null(), err
	}
	wrote, werr := unix.Write(int(b.f.Fd()), data)
	if werr != nil {
		return interpreter.Null(), fmt.Errorf("iic.write: %v", werr)
	}
	return interpreter.IntVal(int64(wrote)), nil
}

// iic.readReg(bus, reg, n) -> bytes. Writes the 1-byte register pointer, then
// reads n bytes (the common "set register, read back" transaction).
func readRegFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("iic.readReg", args, 3); err != nil {
		return interpreter.Null(), err
	}
	b, err := resolveBus("iic.readReg", args)
	if err != nil {
		return interpreter.Null(), err
	}
	reg, err := devio.IntArg("iic.readReg", args, 1, "reg")
	if err != nil {
		return interpreter.Null(), err
	}
	n, err := devio.IntArg("iic.readReg", args, 2, "n")
	if err != nil {
		return interpreter.Null(), err
	}
	if reg < 0 || reg > 0xff {
		return interpreter.Null(), fmt.Errorf("iic.readReg: register %d out of byte range (0..255)", reg)
	}
	sz, err := devio.ReadSize("iic.readReg", n)
	if err != nil {
		return interpreter.Null(), err
	}
	if _, werr := unix.Write(int(b.f.Fd()), []byte{byte(reg)}); werr != nil {
		return interpreter.Null(), fmt.Errorf("iic.readReg: selecting register 0x%02x: %v", reg, werr)
	}
	buf := make([]byte, sz)
	got, rerr := unix.Read(int(b.f.Fd()), buf)
	if rerr != nil {
		return interpreter.Null(), fmt.Errorf("iic.readReg: %v", rerr)
	}
	if got < 0 {
		got = 0
	}
	return interpreter.BytesVal(buf[:got]), nil
}

// iic.writeReg(bus, reg, data) -> int (data bytes written, not counting the
// register pointer). Writes the register pointer and data in one transaction.
func writeRegFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("iic.writeReg", args, 3); err != nil {
		return interpreter.Null(), err
	}
	b, err := resolveBus("iic.writeReg", args)
	if err != nil {
		return interpreter.Null(), err
	}
	reg, err := devio.IntArg("iic.writeReg", args, 1, "reg")
	if err != nil {
		return interpreter.Null(), err
	}
	data, err := devio.BytesArg("iic.writeReg", args, 2, "data")
	if err != nil {
		return interpreter.Null(), err
	}
	if reg < 0 || reg > 0xff {
		return interpreter.Null(), fmt.Errorf("iic.writeReg: register %d out of byte range (0..255)", reg)
	}
	frame := make([]byte, 0, len(data)+1)
	frame = append(frame, byte(reg))
	frame = append(frame, data...)
	wrote, werr := unix.Write(int(b.f.Fd()), frame)
	if werr != nil {
		return interpreter.Null(), fmt.Errorf("iic.writeReg: %v", werr)
	}
	// Report data bytes written, excluding the register pointer.
	if wrote > 0 {
		wrote--
	}
	return interpreter.IntVal(int64(wrote)), nil
}

// iic.close(bus) -> null.
func closeFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("iic.close", args, 1); err != nil {
		return interpreter.Null(), err
	}
	id, err := devio.HandleID("iic.close", args[0], LibraryName, "Bus")
	if err != nil {
		return interpreter.Null(), err
	}
	busesMu.Lock()
	b, ok := buses[id]
	if !ok {
		busesMu.Unlock()
		return interpreter.Null(), fmt.Errorf("iic.close: iic.Bus id %d is not open (already closed?)", id)
	}
	delete(buses, id)
	busesMu.Unlock()
	if cerr := b.f.Close(); cerr != nil {
		return interpreter.Null(), fmt.Errorf("iic.close: %v", cerr)
	}
	return interpreter.Null(), nil
}
