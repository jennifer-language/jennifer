// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

//go:build linux && !tinygo

package gpiolib

import (
	"testing"
	"unsafe"
)

// The GPIO v2 ioctl structs must match the kernel ABI byte-for-byte, or the
// ioctls copy the wrong data. These sizes are from <linux/gpio.h>; a drift in
// the Go layout (added field, wrong type, padding) is caught here rather than as
// a corrupt line request on real hardware.
func TestGpioStructSizesMatchABI(t *testing.T) {
	for _, c := range []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"gpio_v2_line_values", unsafe.Sizeof(gpioV2LineValues{}), 16},
		{"gpio_v2_line_attribute", unsafe.Sizeof(gpioV2LineAttribute{}), 16},
		{"gpio_v2_line_config_attribute", unsafe.Sizeof(gpioV2LineConfigAttribute{}), 24},
		{"gpio_v2_line_config", unsafe.Sizeof(gpioV2LineConfig{}), 272},
		{"gpio_v2_line_request", unsafe.Sizeof(gpioV2LineRequest{}), 592},
	} {
		if c.got != c.want {
			t.Errorf("sizeof(%s) = %d, want %d (kernel ABI)", c.name, c.got, c.want)
		}
	}
	// The ioctl numbers must encode those exact sizes (the read/write dir bits
	// plus the size field are what the kernel dispatches on).
	if want := iowr(0x07, 592); gpioGetLine != want {
		t.Errorf("GPIO_V2_GET_LINE ioctl = %#x, want %#x", gpioGetLine, want)
	}
	if want := iowr(0x0e, 16); gpioGetVals != want {
		t.Errorf("GPIO_V2_LINE_GET_VALUES ioctl = %#x, want %#x", gpioGetVals, want)
	}
	if want := iowr(0x0f, 16); gpioSetVals != want {
		t.Errorf("GPIO_V2_LINE_SET_VALUES ioctl = %#x, want %#x", gpioSetVals, want)
	}
}
