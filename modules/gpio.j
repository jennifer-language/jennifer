# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# gpio.j - Raspberry-Pi (and any Linux SBC) GPIO over sysfs. /sys/class/gpio is
# plain files, so `fs` is the whole backend: export a pin, set its direction,
# read / write its value, unexport it. "Blink an LED from a five-line script."
#
# It is stateless and pin-keyed (sysfs derives every path from the pin number,
# so no handle is needed). The sysfs root defaults to /sys/class/gpio; set the
# JENNIFER_GPIO_BASE environment variable to point elsewhere (a differently
# mounted sysfs, or a mock tree under test).
#
# This is the sysfs backend. sysfs GPIO is deprecated in favour of the
# /dev/gpiochip character device, but stays available on the hobbyist Pi
# kernels this targets; the API is kept stable so the backend could later be
# swapped for a chardev system library with no change to scripts.
#
# GPIO is a Linux-platform feature: off a GPIO-capable host every call reports a
# clear "base directory not found" error rather than crashing.

use fs;
use os;
use convert;
use strings;

def const DEFAULT_BASE as string init "/sys/class/gpio";
def const BASE_ENV as string init "JENNIFER_GPIO_BASE";

# base resolves the sysfs GPIO root: the JENNIFER_GPIO_BASE override if set,
# else /sys/class/gpio.
func base() {
    def override as string init os.getEnv(BASE_ENV);
    if (not ($override == "")) {
        return $override;
    }
    return DEFAULT_BASE;
}

# requireBase errors clearly when the sysfs GPIO tree is absent - not a
# GPIO-capable Linux host, or sysfs GPIO disabled.
func requireBase(dir as string) {
    if (not fs.exists($dir)) {
        throw Error{ kind: "gpio", message: "gpio: base directory not found: " + $dir + " (not a GPIO-capable Linux host, or sysfs GPIO disabled)", file: "", line: 0, col: 0 };
    }
}

# pinDir is the per-pin sysfs directory (dir/gpioN).
func pinDir(dir as string, pin as int) {
    return $dir + "/gpio" + convert.toString($pin);
}

# setup exports a pin and sets its direction ("in" or "out").
export func setup(pin as int, direction as string) {
    if (not ($direction == "in" or $direction == "out")) {
        throw Error{ kind: "gpio", message: "gpio.setup: direction must be \"in\" or \"out\", got \"" + $direction + "\"", file: "", line: 0, col: 0 };
    }
    def dir as string init base();
    requireBase($dir);
    def pinStr as string init convert.toString($pin);
    fs.writeString($dir + "/export", $pinStr);
    # The kernel creates gpioN on export; mkdirAll is a no-op when it already
    # exists (real sysfs) and creates it on a mock tree.
    def pd as string init pinDir($dir, $pin);
    fs.mkdirAll($pd);
    fs.writeString($pd + "/direction", $direction);
}

# write sets an output pin's value (0 or 1).
export func write(pin as int, value as int) {
    if (not ($value == 0 or $value == 1)) {
        throw Error{ kind: "gpio", message: "gpio.write: value must be 0 or 1", file: "", line: 0, col: 0 };
    }
    def dir as string init base();
    requireBase($dir);
    fs.writeString(pinDir($dir, $pin) + "/value", convert.toString($value));
}

# read returns a pin's current value (0 or 1).
export func read(pin as int) {
    def dir as string init base();
    requireBase($dir);
    def raw as string init fs.readString(pinDir($dir, $pin) + "/value");
    return convert.toInt(strings.trim($raw));
}

# release unexports a pin.
export func release(pin as int) {
    def dir as string init base();
    requireBase($dir);
    fs.writeString($dir + "/unexport", convert.toString($pin));
}
