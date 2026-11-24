# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

# The device-I/O libraries: serial / spi / i2c / gpio, all over the Linux
# /dev + ioctl interface. Each block is guarded so this demo runs on any machine
# (it just reports "no device" when the hardware / kernel node is absent) - on a
# real single-board computer the guarded calls do the actual I/O.
#
# Each block `defer`s its release right after acquiring the resource, so the
# handle is closed however the block exits (success or a mid-exchange throw) -
# the cleanup lives next to the open, not scattered across exit paths.
use encoding;
use serial;
use spi;
use i2c;
use gpio;
use io;

# serial: open a port at 115200 8N1, write a command, read a reply.
try {
    def port as serial.Port init serial.open("/dev/ttyUSB0", 115200);
    defer serial.close($port); # closed however this block exits
    serial.write($port, encoding.encode("utf-8", "AT\r\n"));
    def reply as bytes init serial.read($port, 64);
    io.printf("serial: read %d bytes\n", len($reply));
} catch (e) {
    io.printf("serial: no device\n");
}

# spi: full-duplex transfer (send one byte, read one back).
try {
    def dev as spi.Device init spi.openWith(
        "/dev/spidev0.0",
        spi.Options{mode: 0, speedHz: 1000000});
    defer spi.close($dev);
    def cmd as bytes;
    $cmd[] = 0x9f; # e.g. a JEDEC read-id opcode
    def rx as bytes init spi.transfer($dev, $cmd);
    io.printf("spi: got %d bytes\n", len($rx));
} catch (e) {
    io.printf("spi: no device\n");
}

# i2c: read a register from an I2C slave.
try {
    def bus as i2c.Bus init i2c.open("/dev/i2c-1", 0x76);
    defer i2c.close($bus);
    def id as bytes init i2c.readReg($bus, 0xd0, 1);
    io.printf("i2c: chip id byte read\n");
} catch (e) {
    io.printf("i2c: no device\n");
}

# gpio: drive an output pin on the first GPIO chip.
try {
    def chip as gpio.Chip init gpio.open("/dev/gpiochip0");
    defer gpio.close($chip); # releases the chip's pins too
    gpio.setup($chip, 17, gpio.OUT);
    gpio.write($chip, 17, 1);
    io.printf("gpio: pin 17 driven high\n");
} catch (e) {
    io.printf("gpio: no device\n");
}
