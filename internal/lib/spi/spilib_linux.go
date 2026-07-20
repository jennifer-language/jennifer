// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

//go:build linux && !tinygo

// Linux implementation of the `spi` library over golang.org/x/sys/unix. The SPI
// ioctl numbers and the spi_ioc_transfer layout come from <linux/spi/spidev.h>
// (x/sys/unix exports neither), computed with the asm-generic _IOC macro so the
// magic numbers are derived, not transcribed.

package spilib

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/devio"
)

// asm-generic ioctl encoding: dir<<30 | size<<16 | type<<8 | nr.
const (
	iocWrite  = 1
	spiMagic  = 0x6b // 'k'
	xferBytes = 32   // sizeof(struct spi_ioc_transfer)
)

func iow(nr, size uintptr) uintptr {
	return (uintptr(iocWrite) << 30) | (size << 16) | (uintptr(spiMagic) << 8) | nr
}

// SPI ioctl requests.
var (
	spiWrMode       = iow(1, 1) // SPI_IOC_WR_MODE, __u8
	spiWrBits       = iow(3, 1) // SPI_IOC_WR_BITS_PER_WORD, __u8
	spiWrMaxSpeedHz = iow(4, 4) // SPI_IOC_WR_MAX_SPEED_HZ, __u32
)

// spiMessage1 is SPI_IOC_MESSAGE(1): _IOW('k', 0, sizeof(spi_ioc_transfer)).
func spiMessage1() uintptr { return iow(0, xferBytes) }

// spiIocTransfer mirrors struct spi_ioc_transfer (32 bytes, naturally packed).
type spiIocTransfer struct {
	txBuf          uint64
	rxBuf          uint64
	length         uint32
	speedHz        uint32
	delayUsecs     uint16
	bitsPerWord    uint8
	csChange       uint8
	txNbits        uint8
	rxNbits        uint8
	wordDelayUsecs uint8
	pad            uint8
}

type devState struct {
	f       *os.File
	mode    uint8
	speedHz uint32
}

var (
	devsMu sync.Mutex
	devs   = map[int64]*devState{}
	nextID int64
)

// ResetForTest wipes the registry between tests.
func ResetForTest() {
	devsMu.Lock()
	defer devsMu.Unlock()
	for _, d := range devs {
		if d != nil && d.f != nil {
			_ = d.f.Close()
		}
	}
	devs = map[int64]*devState{}
	nextID = 0
}

func resolveDev(fn string, args []Value) (*devState, error) {
	id, err := devio.HandleID(fn, args[0], LibraryName, "Device")
	if err != nil {
		return nil, err
	}
	devsMu.Lock()
	defer devsMu.Unlock()
	d, ok := devs[id]
	if !ok {
		return nil, fmt.Errorf("%s: spi.Device id %d is not open (already closed, or never opened)", fn, id)
	}
	return d, nil
}

func ioctlPtr(fd int, req uintptr, p unsafe.Pointer) error {
	_, _, e := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), req, uintptr(p))
	if e != 0 {
		return e
	}
	return nil
}

// applyConfig pushes mode / bits / speed to the device.
func applyConfig(fd int, mode uint8, speedHz uint32) error {
	bits := uint8(8)
	if err := ioctlPtr(fd, spiWrMode, unsafe.Pointer(&mode)); err != nil {
		return fmt.Errorf("setting mode: %v", err)
	}
	if err := ioctlPtr(fd, spiWrBits, unsafe.Pointer(&bits)); err != nil {
		return fmt.Errorf("setting bits-per-word: %v", err)
	}
	if err := ioctlPtr(fd, spiWrMaxSpeedHz, unsafe.Pointer(&speedHz)); err != nil {
		return fmt.Errorf("setting speed: %v", err)
	}
	return nil
}

// spi.open(path) -> spi.Device. Defaults to mode 0, 500 kHz, 8 bits/word.
func openFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("spi.open", args, 1); err != nil {
		return interpreter.Null(), err
	}
	path, err := devio.StringArg("spi.open", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	f, oerr := os.OpenFile(path, os.O_RDWR, 0)
	if oerr != nil {
		return interpreter.Null(), fmt.Errorf("spi.open: %s: %v", path, oerr)
	}
	const defSpeed = 500000
	if cerr := applyConfig(int(f.Fd()), 0, defSpeed); cerr != nil {
		_ = f.Close()
		return interpreter.Null(), fmt.Errorf("spi.open: %v", cerr)
	}
	devsMu.Lock()
	nextID++
	id := nextID
	devs[id] = &devState{f: f, mode: 0, speedHz: defSpeed}
	devsMu.Unlock()
	return devio.Handle(LibraryName, "Device", id), nil
}

// spi.configure(dev, mode, speedHz) -> null. mode is 0..3 (CPOL/CPHA).
func configureFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("spi.configure", args, 3); err != nil {
		return interpreter.Null(), err
	}
	d, err := resolveDev("spi.configure", args)
	if err != nil {
		return interpreter.Null(), err
	}
	mode, err := devio.IntArg("spi.configure", args, 1, "mode")
	if err != nil {
		return interpreter.Null(), err
	}
	speed, err := devio.IntArg("spi.configure", args, 2, "speedHz")
	if err != nil {
		return interpreter.Null(), err
	}
	if mode < 0 || mode > 3 {
		return interpreter.Null(), fmt.Errorf("spi.configure: mode must be 0..3, got %d", mode)
	}
	if speed <= 0 {
		return interpreter.Null(), fmt.Errorf("spi.configure: speedHz must be positive, got %d", speed)
	}
	if cerr := applyConfig(int(d.f.Fd()), uint8(mode), uint32(speed)); cerr != nil {
		return interpreter.Null(), fmt.Errorf("spi.configure: %v", cerr)
	}
	d.mode = uint8(mode)
	d.speedHz = uint32(speed)
	return interpreter.Null(), nil
}

// spi.transfer(dev, data) -> bytes. Full-duplex: clocks out len(data) bytes and
// returns the len(data) bytes clocked in simultaneously.
func transferFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("spi.transfer", args, 2); err != nil {
		return interpreter.Null(), err
	}
	d, err := resolveDev("spi.transfer", args)
	if err != nil {
		return interpreter.Null(), err
	}
	tx, err := devio.BytesArg("spi.transfer", args, 1, "data")
	if err != nil {
		return interpreter.Null(), err
	}
	if len(tx) == 0 {
		return interpreter.BytesVal(nil), nil
	}
	rx := make([]byte, len(tx))
	xfer := spiIocTransfer{
		txBuf:       uint64(uintptr(unsafe.Pointer(&tx[0]))),
		rxBuf:       uint64(uintptr(unsafe.Pointer(&rx[0]))),
		length:      uint32(len(tx)),
		speedHz:     d.speedHz,
		bitsPerWord: 8,
	}
	if terr := ioctlPtr(int(d.f.Fd()), spiMessage1(), unsafe.Pointer(&xfer)); terr != nil {
		return interpreter.Null(), fmt.Errorf("spi.transfer: %v", terr)
	}
	// Keep tx/rx alive until the ioctl returns (their addresses live in xfer).
	runtime.KeepAlive(tx)
	runtime.KeepAlive(rx)
	return interpreter.BytesVal(rx), nil
}

// spi.close(dev) -> null.
func closeFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("spi.close", args, 1); err != nil {
		return interpreter.Null(), err
	}
	id, err := devio.HandleID("spi.close", args[0], LibraryName, "Device")
	if err != nil {
		return interpreter.Null(), err
	}
	devsMu.Lock()
	d, ok := devs[id]
	if !ok {
		devsMu.Unlock()
		return interpreter.Null(), fmt.Errorf("spi.close: spi.Device id %d is not open (already closed?)", id)
	}
	delete(devs, id)
	devsMu.Unlock()
	if cerr := d.f.Close(); cerr != nil {
		return interpreter.Null(), fmt.Errorf("spi.close: %v", cerr)
	}
	return interpreter.Null(), nil
}
