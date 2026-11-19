// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build linux && !tinygo

// Linux implementation of the `spi` library over golang.org/x/sys/unix. The SPI
// ioctl numbers and the spi_ioc_transfer layout come from <linux/spi/spidev.h>
// (x/sys/unix exports neither), computed with the asm-generic _IOC macro so the
// magic numbers are derived, not transcribed.

package spilib

import (
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
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

// devState is one open device. mu serialises configure / transfer / close on
// it: speedHz is read by transfer and written by configure, and close must not
// release the fd number while a transfer still uses it.
type devState struct {
	mu      sync.Mutex
	f       *os.File
	mode    uint8
	speedHz uint32
	bufsiz  int // spidev's per-message buffer, read at open
}

// defaultBufsiz is spidev's default buffer size (its bufsiz module parameter).
const defaultBufsiz = 4096

// bufsizPath holds spidev's buffer size: the most bytes one SPI_IOC_MESSAGE
// may move, summed over its transfers. A variable for tests.
var bufsizPath = "/sys/module/spidev/parameters/bufsiz"

// spidevBufsiz reads the spidev buffer size, or the default when the
// parameter is not readable (no sysfs, or no spidev module information).
func spidevBufsiz() int {
	raw, err := os.ReadFile(bufsizPath)
	if err != nil {
		return defaultBufsiz
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || n <= 0 {
		return defaultBufsiz
	}
	return n
}

// ioctl issues one ioctl; a variable so tests can stand in for the driver.
var ioctl = ioctlPtr

// explain is err's OS text plus a hint for the errno values an spidev open or
// transfer commonly hits. A *PathError's path is dropped: callers name it.
func explain(err error) string {
	var en unix.Errno
	if !errors.As(err, &en) {
		return err.Error()
	}
	switch en {
	case unix.EACCES, unix.EPERM:
		return en.Error() + " (add the user to the group that owns the device, often spi)"
	case unix.EBUSY:
		return en.Error() + " (the device is in use)"
	case unix.ENODEV, unix.ENXIO:
		return en.Error() + " (no SPI controller behind this device node; is SPI enabled?)"
	case unix.ENOTTY:
		return en.Error() + " (not an spidev device)"
	case unix.EINVAL:
		return en.Error() + " (the controller does not support this setting)"
	}
	return en.Error()
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
	if err := ioctl(fd, spiWrMode, unsafe.Pointer(&mode)); err != nil {
		return fmt.Errorf("setting mode %d: %s", mode, explain(err))
	}
	if err := ioctl(fd, spiWrBits, unsafe.Pointer(&bits)); err != nil {
		return fmt.Errorf("setting bits-per-word: %s", explain(err))
	}
	if err := ioctl(fd, spiWrMaxSpeedHz, unsafe.Pointer(&speedHz)); err != nil {
		return fmt.Errorf("setting speed %d Hz: %s", speedHz, explain(err))
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
		return interpreter.Null(), fmt.Errorf("spi.open: %s: %s", path, explain(oerr))
	}
	const defSpeed = 500000
	if cerr := applyConfig(int(f.Fd()), 0, defSpeed); cerr != nil {
		_ = f.Close()
		return interpreter.Null(), fmt.Errorf("spi.open: %s: %v", path, cerr)
	}
	devsMu.Lock()
	nextID++
	id := nextID
	devs[id] = &devState{f: f, mode: 0, speedHz: defSpeed, bufsiz: spidevBufsiz()}
	devsMu.Unlock()
	return devio.Handle(LibraryName, "Device", id), nil
}

// spi.configure(dev, mode, speedHz) -> null. mode is 0..3 (CPOL/CPHA).
func configureFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
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
	// speed_hz is a u32: 2^32 used to truncate to 0 ("keep the current max").
	if speed <= 0 || speed > math.MaxUint32 {
		return interpreter.Null(), fmt.Errorf("spi.configure: speedHz must be in 1..%d, got %d", uint32(math.MaxUint32), speed)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.f == nil {
		return interpreter.Null(), interpreter.RaiseError("closed", "spi.configure: the device was closed", ctx.File, ctx.Line, ctx.Col)
	}
	if cerr := applyConfig(int(d.f.Fd()), uint8(mode), uint32(speed)); cerr != nil {
		return interpreter.Null(), fmt.Errorf("spi.configure: %v", cerr)
	}
	d.mode = uint8(mode)
	d.speedHz = uint32(speed)
	return interpreter.Null(), nil
}

// limitError is the Error{kind: "limit"} for a transfer over spidev's buffer.
// One transfer is one message with chip select held for its whole length, so
// it is not split: splitting would drop chip select between the parts.
func limitError(ctx interpreter.BuiltinCtx, n, bufsiz int) error {
	return interpreter.RaiseError("limit", fmt.Sprintf("spi.transfer: %d bytes exceed the spidev buffer of %d bytes "+
		"(one transfer is one chip-select assertion and is not split; raise the spidev.bufsiz module parameter, "+
		"e.g. spidev.bufsiz=65536 on the kernel command line, or transfer in parts if the device allows "+
		"chip select to drop between them)", n, bufsiz), ctx.File, ctx.Line, ctx.Col)
}

// spi.transfer(dev, data) -> bytes. Full-duplex: clocks out len(data) bytes and
// returns the len(data) bytes clocked in simultaneously, in one message (chip
// select asserted throughout). More than spidev's bufsiz bytes is kind "limit".
func transferFn(ctx interpreter.BuiltinCtx, args []Value) (Value, error) {
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
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.f == nil {
		return interpreter.Null(), interpreter.RaiseError("closed", "spi.transfer: the device was closed", ctx.File, ctx.Line, ctx.Col)
	}
	if len(tx) > d.bufsiz {
		return interpreter.Null(), limitError(ctx, len(tx), d.bufsiz)
	}
	rx := make([]byte, len(tx))
	xfer := spiIocTransfer{
		txBuf:       uint64(uintptr(unsafe.Pointer(&tx[0]))),
		rxBuf:       uint64(uintptr(unsafe.Pointer(&rx[0]))),
		length:      uint32(len(tx)),
		speedHz:     d.speedHz,
		bitsPerWord: 8,
	}
	if terr := ioctl(int(d.f.Fd()), spiMessage1(), unsafe.Pointer(&xfer)); terr != nil {
		if errors.Is(terr, unix.EMSGSIZE) {
			return interpreter.Null(), limitError(ctx, len(tx), d.bufsiz)
		}
		return interpreter.Null(), fmt.Errorf("spi.transfer: %s", explain(terr))
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
	d.mu.Lock() // wait for a transfer in flight
	defer d.mu.Unlock()
	f := d.f
	d.f = nil
	if cerr := f.Close(); cerr != nil {
		return interpreter.Null(), fmt.Errorf("spi.close: %v", cerr)
	}
	return interpreter.Null(), nil
}
