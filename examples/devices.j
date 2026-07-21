# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

# The device-I/O libraries: serial / spi / iic / gpio, all over the Linux
# /dev + ioctl interface. Each block is guarded so this demo runs on any machine
# (it just reports "no device" when the hardware / kernel node is absent) - on a
# real single-board computer the guarded calls do the actual I/O.
#
# Each block `defer`s its release right after acquiring the resource, so the
# handle is closed however the block exits (success or a mid-exchange throw) -
# the cleanup lives next to the open, not scattered across exit paths.
use serial;
use spi;
use iic;
use gpio;
use convert;
use io;

# serial: open a port at 115200 8N1, write a command, read a reply.
try {
    def port as serial.Port init serial.open("/dev/ttyUSB0", 115200);
    defer serial.close($port);              # closed however this block exits
    serial.write($port, convert.bytesFromString("AT\r\n", "utf-8"));
    def reply as bytes init serial.read($port, 64);
    io.printf("serial: read %d bytes\n", len($reply));
} catch (e) {
    io.printf("serial: no device\n");
}

# spi: full-duplex transfer (send one byte, read one back).
try {
    def dev as spi.Device init spi.open("/dev/spidev0.0");
    defer spi.close($dev);
    spi.configure($dev, 0, 1000000);
    def cmd as bytes;
    $cmd[] = 0x9f;                           # e.g. a JEDEC read-id opcode
    def rx as bytes init spi.transfer($dev, $cmd);
    io.printf("spi: got %d bytes\n", len($rx));
} catch (e) {
    io.printf("spi: no device\n");
}

# iic: read a register from an I2C slave.
try {
    def bus as iic.Bus init iic.open("/dev/i2c-1", 0x76);
    defer iic.close($bus);
    def id as bytes init iic.readReg($bus, 0xd0, 1);
    io.printf("iic: chip id byte read\n");
} catch (e) {
    io.printf("iic: no device\n");
}

# gpio: drive an output pin (pin-keyed, like the sysfs gpio module).
try {
    gpio.setup(17, gpio.OUT);
    defer gpio.release(17);
    gpio.write(17, 1);
    io.printf("gpio: pin 17 driven high\n");
} catch (e) {
    io.printf("gpio: no device\n");
}
