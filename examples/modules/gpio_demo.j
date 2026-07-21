#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Blink an LED with the gpio module.
 * On a real Raspberry Pi, gpio talks to /sys/class/gpio directly - drop the three "mock" lines below and this drives physical pin 17. To keep the demo runnable on any machine (no hardware, no root), it points gpio at a mock sysfs tree via JENNIFER_GPIO_BASE and drives that instead. The gpio calls are exactly what you'd run on the Pi.
 * @module gpio_demo
 */
use io;
use os;
use fs;
import "../../modules/gpio.j" as gpio;

# --- mock sysfs so the demo runs anywhere (remove on a real Pi) -------------
def mock as string init os.tempDir() + "/gpio_demo_mock";
if (fs.exists($mock)) {
    fs.removeAll($mock);
}
fs.mkdirAll($mock);
os.setEnv("JENNIFER_GPIO_BASE", $mock);

# --- the actual GPIO program ------------------------------------------------
io.printf("setup pin 17 as output\n");
gpio.setup(17, gpio.OUT);

for (def i as int init 0; $i < 3; $i = $i + 1) {
    gpio.write(17, 1);
    io.printf("blink %d: pin 17 = %d\n", $i, gpio.read(17));
    gpio.write(17, 0);
    io.printf("blink %d: pin 17 = %d\n", $i, gpio.read(17));
    # On real hardware add `time.sleep(...)` here for a visible blink.
}

gpio.release(17);
io.printf("released pin 17\n");

fs.removeAll($mock);
