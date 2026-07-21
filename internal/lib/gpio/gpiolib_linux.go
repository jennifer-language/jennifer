// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 mplx <jennifer@mplx.dev>

//go:build linux && !tinygo

// Linux implementation of the `gpio` library over the GPIO v2 line ioctls
// (<linux/gpio.h>; x/sys/unix does not export them). The struct layouts mirror
// the kernel ABI exactly - gpio_test.go asserts their sizes match the ABI so a
// drift is caught at test time rather than as a corrupt ioctl.

package gpiolib

import (
	"fmt"
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

var (
	linesMu  sync.Mutex
	lines    = map[int64]*lineState{} // pin -> requested line
	chipPath = ""                     // "" means resolve default at setup time
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
	lines = map[int64]*lineState{}
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

func ioctlPtr(fd int, req uintptr, p unsafe.Pointer) error {
	_, _, e := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), req, uintptr(p))
	if e != 0 {
		return e
	}
	return nil
}

// gpio.chip(path) -> null. Selects the gpiochip device for subsequent setups.
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

// gpio.setup(pin, direction) -> null. Requests `pin` from the current chip with
// direction gpio.IN or gpio.OUT.
func setupFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("gpio.setup", args, 2); err != nil {
		return interpreter.Null(), err
	}
	pin, err := devio.IntArg("gpio.setup", args, 0, "pin")
	if err != nil {
		return interpreter.Null(), err
	}
	dir, err := devio.StringArg("gpio.setup", args, 1, "direction")
	if err != nil {
		return interpreter.Null(), err
	}
	if pin < 0 || pin >= 64 {
		return interpreter.Null(), fmt.Errorf("gpio.setup: pin %d out of range (0..63 per request)", pin)
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
	if _, exists := lines[pin]; exists {
		return interpreter.Null(), fmt.Errorf("gpio.setup: pin %d is already set up; release it first", pin)
	}
	path := resolveChipPath()
	chip, oerr := os.OpenFile(path, os.O_RDWR, 0)
	if oerr != nil {
		return interpreter.Null(), fmt.Errorf("gpio.setup: %s: %v", path, oerr)
	}
	defer chip.Close() // the requested line has its own fd; the chip fd isn't needed after.

	req := gpioV2LineRequest{numLines: 1}
	req.offsets[0] = uint32(pin)
	req.config.flags = flags
	copy(req.consumer[:], "jennifer")
	if ierr := ioctlPtr(int(chip.Fd()), gpioGetLine, unsafe.Pointer(&req)); ierr != nil {
		return interpreter.Null(), fmt.Errorf("gpio.setup: requesting pin %d: %v", pin, ierr)
	}
	if req.fd < 0 {
		return interpreter.Null(), fmt.Errorf("gpio.setup: kernel returned no line fd for pin %d", pin)
	}
	lines[pin] = &lineState{fd: int(req.fd), dir: dir}
	return interpreter.Null(), nil
}

// gpio.read(pin) -> int (0 or 1).
func readFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("gpio.read", args, 1); err != nil {
		return interpreter.Null(), err
	}
	pin, err := devio.IntArg("gpio.read", args, 0, "pin")
	if err != nil {
		return interpreter.Null(), err
	}
	linesMu.Lock()
	l, ok := lines[pin]
	linesMu.Unlock()
	if !ok {
		return interpreter.Null(), fmt.Errorf("gpio.read: pin %d is not set up (call gpio.setup first)", pin)
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

// gpio.write(pin, value) -> null. value is 0 or 1.
func writeFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("gpio.write", args, 2); err != nil {
		return interpreter.Null(), err
	}
	pin, err := devio.IntArg("gpio.write", args, 0, "pin")
	if err != nil {
		return interpreter.Null(), err
	}
	value, err := devio.IntArg("gpio.write", args, 1, "value")
	if err != nil {
		return interpreter.Null(), err
	}
	if value != 0 && value != 1 {
		return interpreter.Null(), fmt.Errorf("gpio.write: value must be 0 or 1, got %d", value)
	}
	linesMu.Lock()
	l, ok := lines[pin]
	linesMu.Unlock()
	if !ok {
		return interpreter.Null(), fmt.Errorf("gpio.write: pin %d is not set up (call gpio.setup first)", pin)
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

// gpio.release(pin) -> null. Closes the requested line, freeing it.
func releaseFn(_ interpreter.BuiltinCtx, args []Value) (Value, error) {
	if err := devio.WantArgs("gpio.release", args, 1); err != nil {
		return interpreter.Null(), err
	}
	pin, err := devio.IntArg("gpio.release", args, 0, "pin")
	if err != nil {
		return interpreter.Null(), err
	}
	linesMu.Lock()
	l, ok := lines[pin]
	if !ok {
		linesMu.Unlock()
		return interpreter.Null(), fmt.Errorf("gpio.release: pin %d is not set up", pin)
	}
	delete(lines, pin)
	linesMu.Unlock()
	if cerr := unix.Close(l.fd); cerr != nil {
		return interpreter.Null(), fmt.Errorf("gpio.release: pin %d: %v", pin, cerr)
	}
	return interpreter.Null(), nil
}
