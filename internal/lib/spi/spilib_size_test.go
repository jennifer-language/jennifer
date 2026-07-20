// SPDX-License-Identifier: LGPL-3.0-only
// Copyright (C) 2026 <developer@mplx.eu>

//go:build linux && !tinygo

package spilib

import (
	"testing"
	"unsafe"
)

// struct spi_ioc_transfer is 32 bytes (<linux/spi/spidev.h>). A drift in the Go
// layout would corrupt SPI_IOC_MESSAGE, so pin the size and the ioctl numbers.
func TestSpiTransferLayout(t *testing.T) {
	if got := unsafe.Sizeof(spiIocTransfer{}); got != xferBytes {
		t.Errorf("sizeof(spi_ioc_transfer) = %d, want %d", got, xferBytes)
	}
	// SPI_IOC_WR_MODE=_IOW('k',1,1), WR_MAX_SPEED_HZ=_IOW('k',4,4), MESSAGE(1) size=32.
	if want := uintptr(0x40016b01); spiWrMode != want {
		t.Errorf("SPI_IOC_WR_MODE = %#x, want %#x", spiWrMode, want)
	}
	if want := uintptr(0x40046b04); spiWrMaxSpeedHz != want {
		t.Errorf("SPI_IOC_WR_MAX_SPEED_HZ = %#x, want %#x", spiWrMaxSpeedHz, want)
	}
	if want := uintptr(0x40206b00); spiMessage1() != want {
		t.Errorf("SPI_IOC_MESSAGE(1) = %#x, want %#x", spiMessage1(), want)
	}
}
