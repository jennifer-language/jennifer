#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

/**
 * Blink an LED with the gpio module.
 * On a real Raspberry Pi, gpio talks to /sys/class/gpio directly - drop the "mock" block below and this drives BCM GPIO 17 (header pin 11). To keep the demo runnable on any machine (no hardware, no root), it points gpio at a mock sysfs tree via JENNIFER_GPIO_BASE and drives that instead. The mock holds a Raspberry Pi 4 header chip at base 512, as Linux 6.2+ numbers it, so pin 17 is exported as global 529. The gpio calls are exactly what you'd run on the Pi.
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
fs.mkdirAll($mock + "/gpiochip512");
fs.writeString($mock + "/gpiochip512/base", "512\n");
fs.writeString($mock + "/gpiochip512/label", "pinctrl-bcm2711\n");
fs.writeString($mock + "/gpiochip512/ngpio", "58\n");
os.setEnv("JENNIFER_GPIO_BASE", $mock);

# --- the actual GPIO program ------------------------------------------------
io.printf("setup pin 17 as output\n");
gpio.setup(17, gpio.OUT);

for (def i in 0..3) {
    gpio.write(17, 1);
    io.printf("blink %d: pin 17 = %d\n", $i, gpio.read(17));
    gpio.write(17, 0);
    io.printf("blink %d: pin 17 = %d\n", $i, gpio.read(17));
    # On real hardware add `time.sleep(...)` here for a visible blink.
}

io.printf("sysfs exported global gpio %s\n", fs.readString($mock + "/export"));
gpio.release(17);
io.printf("released pin 17\n");

fs.removeAll($mock);
