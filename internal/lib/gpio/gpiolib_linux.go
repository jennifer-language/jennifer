// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build linux && !tinygo

// Linux implementation of the `gpio` library over the GPIO v2 line ioctls
// (<linux/gpio.h>; x/sys/unix does not export them). The struct layouts mirror
// the kernel ABI exactly - gpio_test.go asserts their sizes match the ABI so a
// drift is caught at test time rather than as a corrupt ioctl.

package gpiolib

import (
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"

	"jennifer-lang.dev/jennifer/internal/interpreter"
	"jennifer-lang.dev/jennifer/internal/lib/devio"
)

// GPIO v2 line flags (<linux/gpio.h>).
const (
	gpioLineFlagInput  = 1 << 2
	gpioLineFlagOutput = 1 << 3
	gpioMagic          = 0xB4
	gpioMaxName        = 32
)

// asm-generic _IOWR(type, nr, size): dir(3)<<30 | size<<16 | type<<8 | nr.
func iowr(nr, size uintptr) uintptr {
	return (uintptr(3) << 30) | (size << 16) | (uintptr(gpioMagic) << 8) | nr
}

// asm-generic _IOR(type, nr, size): dir(2)<<30 | size<<16 | type<<8 | nr.
func ior(nr, size uintptr) uintptr {
	return (uintptr(2) << 30) | (size << 16) | (uintptr(gpioMagic) << 8) | nr
}

// gpiochip_info (68 bytes).
type gpioChipInfo struct {
	name  [gpioMaxName]byte
	label [gpioMaxName]byte
	lines uint32
}

// gpio_v2_line_values.
type gpioV2LineValues struct {
	bits uint64
	mask uint64
}

// gpio_v2_line_attribute (16 bytes: id + padding + 8-byte union).
type gpioV2LineAttribute struct {
	id      uint32
	padding uint32
	value   uint64
}

// gpio_v2_line_config_attribute (24 bytes).
type gpioV2LineConfigAttribute struct {
	attr gpioV2LineAttribute
	mask uint64
}

// gpio_v2_line_config (272 bytes).
type gpioV2LineConfig struct {
	flags    uint64
	numAttrs uint32
	padding  [5]uint32
	attrs    [10]gpioV2LineConfigAttribute
}

// gpio_v2_line_request (592 bytes).
type gpioV2LineRequest struct {
	offsets         [64]uint32
	consumer        [gpioMaxName]byte
	config          gpioV2LineConfig
	numLines        uint32
	eventBufferSize uint32
	padding         [5]uint32
	fd              int32
}

// ioctl requests, sized from the actual Go structs so the number always matches
// the bytes the kernel copies (a layout drift is caught by the size test).
var (
	gpioGetInfo = ior(0x01, unsafe.Sizeof(gpioChipInfo{}))
	gpioGetLine = iowr(0x07, unsafe.Sizeof(gpioV2LineRequest{}))
	gpioGetVals = iowr(0x0e, unsafe.Sizeof(gpioV2LineValues{}))
	gpioSetVals = iowr(0x0f, unsafe.Sizeof(gpioV2LineValues{}))
	defaultChip = "/dev/gpiochip0"
	chipEnvVar  = "JENNIFER_GPIO_CHIP"
)

type lineState struct {
	fd  int
	dir string
}

// lineKey names a requested line: the chip handle id (0 for the process-wide
// chip of the deprecated pin-only calls) and the pin.
type lineKey struct{ chip, pin int64 }

// chipState is one gpio.open handle: the chip device, kept open for line
// requests until gpio.close.
type chipState struct {
	path string
	f    *os.File
}

var (
	linesMu  sync.Mutex                 // guards lines, chips, nextChip and chipPath
	lines    = map[lineKey]*lineState{} // requested lines
	chips    = map[int64]*chipState{}   // open gpio.Chip handles
	nextChip int64                      // last gpio.Chip id handed out
	chipPath = ""                       // deprecated gpio.chip; "" means resolve the default at setup time
)

// ResetForTest wipes the registry between tests.
func ResetForTest() {
	linesMu.Lock()
	defer linesMu.Unlock()
	for _, l := range lines {
		if l != nil {
			_ = unix.Close(l.fd)
		}
	}
	lines = map[lineKey]*lineState{}
	for _, c := range chips {
		_ = c.f.Close()
	}
	chips = map[int64]*chipState{}
	nextChip = 0
	chipPath = ""
}

func resolveChipPath() string {
	if chipPath != "" {
		return chipPath
	}
	if env := os.Getenv(chipEnvVar); env != "" {
		return env
	}
	return defaultChip
}

// ioctlPtr issues one ioctl; a variable so a test can observe what runs
// while a line ioctl is in flight.
var ioctlPtr = func(fd int, req uintptr, p unsafe.Pointer) error {
	_, _, e := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), req, uintptr(p))
	if e != 0 {
		return e
	}
	return nil
}

// gpio.chip(path) -> null. Selects the gpiochip device for the pin-only calls.
// Deprecated (M29.49): gpio.open returns a chip handle.
func chipFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("gpio.chip", args, 1); err != nil {
		return interpreter.Null(), err
	}
	path, err := devio.StringArg("gpio.chip", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	linesMu.Lock()
	chipPath = path
	linesMu.Unlock()
	return interpreter.Null(), nil
}

// gpio.open(path) -> gpio.Chip. Opens a gpiochip device and checks it is one
// (GPIO_GET_CHIPINFO); its lines are requested through the handle.
func openFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("gpio.open", args, 1); err != nil {
		return interpreter.Null(), err
	}
	path, err := devio.StringArg("gpio.open", args, 0, "path")
	if err != nil {
		return interpreter.Null(), err
	}
	f, oerr := os.OpenFile(path, os.O_RDWR, 0)
	if oerr != nil {
		return interpreter.Null(), fmt.Errorf("gpio.open: %s: %s", path, explain(oerr))
	}
	var info gpioChipInfo
	if ierr := ioctlPtr(int(f.Fd()), gpioGetInfo, unsafe.Pointer(&info)); ierr != nil {
		_ = f.Close()
		return interpreter.Null(), fmt.Errorf("gpio.open: %s: %s", path, explain(ierr))
	}
	linesMu.Lock()
	defer linesMu.Unlock()
	nextChip++
	chips[nextChip] = &chipState{path: path, f: f}
	return devio.Handle(LibraryName, "Chip", nextChip), nil
}

// gpio.close(chip) -> null. Releases the chip's requested lines and closes
// it. A second close of the same chip does nothing.
func closeFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("gpio.close", args, 1); err != nil {
		return interpreter.Null(), err
	}
	id, err := devio.HandleID("gpio.close", args[0], LibraryName, "Chip")
	if err != nil {
		return interpreter.Null(), err
	}
	linesMu.Lock()
	defer linesMu.Unlock()
	c, ok := chips[id]
	if !ok {
		return interpreter.Null(), nil
	}
	delete(chips, id)
	var first error
	for k, l := range lines {
		if k.chip != id {
			continue
		}
		delete(lines, k)
		if cerr := unix.Close(l.fd); cerr != nil && first == nil {
			first = fmt.Errorf("gpio.close: pin %d: %v", k.pin, cerr)
		}
	}
	if cerr := c.f.Close(); cerr != nil && first == nil {
		first = fmt.Errorf("gpio.close: %s: %v", c.path, cerr)
	}
	if first != nil {
		return interpreter.Null(), first
	}
	return interpreter.Null(), nil
}

// target is the chip and pin a call addresses: (chip, pin, ...) when the call
// has handleArgs arguments, else the deprecated (pin, ...) form on the
// process-wide chip. rest are the arguments after the pin.
func target(fn string, args []Value, handleArgs int) (chip int64, pin int64, rest []Value, err error) {
	i := 0
	if len(args) == handleArgs {
		if chip, err = devio.HandleID(fn, args[0], LibraryName, "Chip"); err != nil {
			return 0, 0, nil, err
		}
		i = 1
	} else if len(args) != handleArgs-1 {
		return 0, 0, nil, fmt.Errorf("%s expects %d arguments (gpio.Chip, pin, ...), got %d", fn, handleArgs, len(args))
	}
	if pin, err = devio.IntArg(fn, args, i, "pin"); err != nil {
		return 0, 0, nil, err
	}
	return chip, pin, args[i+1:], nil
}

// liveChip returns an open chip; the caller holds linesMu.
func liveChip(fn string, id int64) (*chipState, error) {
	c, ok := chips[id]
	if !ok {
		return nil, interpreter.KindedError("closed", fmt.Sprintf("%s: gpio.Chip id %d is not open (already closed, or never opened)", fn, id))
	}
	return c, nil
}

// lookupLine finds a set-up line for a read / write / release; the caller
// holds linesMu.
func lookupLine(fn string, chip, pin int64) (*lineState, error) {
	if chip != 0 {
		if _, err := liveChip(fn, chip); err != nil {
			return nil, err
		}
	}
	l, ok := lines[lineKey{chip, pin}]
	if !ok {
		return nil, fmt.Errorf("%s: pin %d is not set up (call gpio.setup first)", fn, pin)
	}
	return l, nil
}

// gpio.setup(chip, pin, direction) -> null (or the deprecated setup(pin,
// direction) on the process-wide chip). Requests `pin` with direction gpio.IN
// or gpio.OUT.
func setupFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	chip, pin, rest, err := target("gpio.setup", args, 3)
	if err != nil {
		return interpreter.Null(), err
	}
	dir, err := devio.StringArg("gpio.setup", rest, 0, "direction")
	if err != nil {
		return interpreter.Null(), err
	}
	// A line offset is a u32 on the chip; the 64 in the ABI is GPIO_V2_LINES_MAX
	// (lines per request), not the offset range, and we always request one line.
	// Many SoC gpiochips expose 100+ lines, so bound by the u32 range and let the
	// kernel's EINVAL surface for an offset the chip does not have.
	if pin < 0 || pin > math.MaxUint32 {
		return interpreter.Null(), fmt.Errorf("gpio.setup: pin %d out of range (0..%d)", pin, uint32(math.MaxUint32))
	}
	var flags uint64
	switch dir {
	case "in":
		flags = gpioLineFlagInput
	case "out":
		flags = gpioLineFlagOutput
	default:
		return interpreter.Null(), fmt.Errorf(`gpio.setup: direction must be gpio.IN ("in") or gpio.OUT ("out"), got %q`, dir)
	}

	linesMu.Lock()
	defer linesMu.Unlock()
	var path string
	var chipFd int
	if chip != 0 {
		c, cerr := liveChip("gpio.setup", chip)
		if cerr != nil {
			return interpreter.Null(), cerr
		}
		path, chipFd = c.path, int(c.f.Fd())
	}
	if _, exists := lines[lineKey{chip, pin}]; exists {
		return interpreter.Null(), fmt.Errorf("gpio.setup: pin %d is already set up; release it first", pin)
	}
	if chip == 0 {
		path = resolveChipPath()
		f, oerr := os.OpenFile(path, os.O_RDWR, 0)
		if oerr != nil {
			return interpreter.Null(), fmt.Errorf("gpio.setup: %s: %s", path, explain(oerr))
		}
		defer f.Close() // the requested line has its own fd; the chip fd isn't needed after.
		chipFd = int(f.Fd())
	}

	req := gpioV2LineRequest{numLines: 1}
	req.offsets[0] = uint32(pin)
	req.config.flags = flags
	copy(req.consumer[:], "jennifer")
	if ierr := ioctlPtr(chipFd, gpioGetLine, unsafe.Pointer(&req)); ierr != nil {
		return interpreter.Null(), fmt.Errorf("gpio.setup: %s: requesting pin %d: %s", path, pin, explain(ierr))
	}
	if req.fd < 0 {
		return interpreter.Null(), fmt.Errorf("gpio.setup: kernel returned no line fd for pin %d", pin)
	}
	lines[lineKey{chip, pin}] = &lineState{fd: int(req.fd), dir: dir}
	return interpreter.Null(), nil
}

// explain is err's OS text plus a hint for the errno values a gpiochip open or
// line request commonly hits. A *PathError's path is dropped: callers name it.
func explain(err error) string {
	var en unix.Errno
	if !errors.As(err, &en) {
		return err.Error()
	}
	switch en {
	case unix.EACCES, unix.EPERM:
		return en.Error() + " (add the user to the group that owns the device, often gpio)"
	case unix.EBUSY:
		return en.Error() + " (the line is in use by another program or a kernel driver)"
	case unix.EINVAL:
		return en.Error() + " (no such line on this chip, or the request is not supported)"
	case unix.ENODEV, unix.ENXIO:
		return en.Error() + " (no GPIO chip behind this device node)"
	case unix.ENOTTY:
		return en.Error() + " (not a gpiochip device)"
	}
	return en.Error()
}

// gpio.read(chip, pin) -> int (0 or 1); the deprecated read(pin) uses the
// process-wide chip.
func readFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	chip, pin, _, err := target("gpio.read", args, 2)
	if err != nil {
		return interpreter.Null(), err
	}
	// Held across the ioctl: a concurrent release must not close the line fd
	// (whose number a later open could reuse) while it is in use. Line ioctls
	// do not block.
	linesMu.Lock()
	defer linesMu.Unlock()
	l, err := lookupLine("gpio.read", chip, pin)
	if err != nil {
		return interpreter.Null(), err
	}
	vals := gpioV2LineValues{mask: 1}
	if ierr := ioctlPtr(l.fd, gpioGetVals, unsafe.Pointer(&vals)); ierr != nil {
		return interpreter.Null(), fmt.Errorf("gpio.read: pin %d: %v", pin, ierr)
	}
	if vals.bits&1 != 0 {
		return interpreter.IntVal(1), nil
	}
	return interpreter.IntVal(0), nil
}

// gpio.write(chip, pin, value) -> null, value 0 or 1; the deprecated
// write(pin, value) uses the process-wide chip.
func writeFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	chip, pin, rest, err := target("gpio.write", args, 3)
	if err != nil {
		return interpreter.Null(), err
	}
	value, err := devio.IntArg("gpio.write", rest, 0, "value")
	if err != nil {
		return interpreter.Null(), err
	}
	if value != 0 && value != 1 {
		return interpreter.Null(), fmt.Errorf("gpio.write: value must be 0 or 1, got %d", value)
	}
	linesMu.Lock() // held across the ioctl, as in read
	defer linesMu.Unlock()
	l, err := lookupLine("gpio.write", chip, pin)
	if err != nil {
		return interpreter.Null(), err
	}
	if l.dir != "out" {
		return interpreter.Null(), fmt.Errorf("gpio.write: pin %d was set up as input; set it up with gpio.OUT to write", pin)
	}
	vals := gpioV2LineValues{mask: 1}
	if value == 1 {
		vals.bits = 1
	}
	if ierr := ioctlPtr(l.fd, gpioSetVals, unsafe.Pointer(&vals)); ierr != nil {
		return interpreter.Null(), fmt.Errorf("gpio.write: pin %d: %v", pin, ierr)
	}
	return interpreter.Null(), nil
}

// gpio.release(chip, pin) -> null (the deprecated release(pin) on the
// process-wide chip). Closes the requested line, freeing it.
func releaseFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	chip, pin, _, err := target("gpio.release", args, 2)
	if err != nil {
		return interpreter.Null(), err
	}
	linesMu.Lock()
	if chip != 0 {
		if _, cerr := liveChip("gpio.release", chip); cerr != nil {
			linesMu.Unlock()
			return interpreter.Null(), cerr
		}
	}
	l, ok := lines[lineKey{chip, pin}]
	if !ok {
		linesMu.Unlock()
		return interpreter.Null(), fmt.Errorf("gpio.release: pin %d is not set up", pin)
	}
	// No read / write holds l past its own critical section, so closing
	// after the unlock cannot hit an ioctl in flight.
	delete(lines, lineKey{chip, pin})
	linesMu.Unlock()
	if cerr := unix.Close(l.fd); cerr != nil {
		return interpreter.Null(), fmt.Errorf("gpio.release: pin %d: %v", pin, cerr)
	}
	return interpreter.Null(), nil
}
