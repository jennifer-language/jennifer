# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Raspberry-Pi (and any Linux SBC) GPIO over sysfs. /sys/class/gpio is plain
 * files, so `fs` is the whole backend: export a pin, set its direction, read /
 * write its value, unexport it. "Blink an LED from a five-line script." It is
 * stateless and pin-keyed (sysfs derives every path from the pin number, so no
 * handle is needed). The sysfs root defaults to /sys/class/gpio; set the
 * JENNIFER_GPIO_BASE environment variable to point elsewhere (a differently
 * mounted sysfs, or a mock tree under test). This is the sysfs backend, which is
 * deprecated in favour of the /dev/gpiochip character device but stays available
 * on the hobbyist Pi kernels this targets; the API is kept stable so the backend
 * could later be swapped for a chardev system library with no change to scripts.
 * GPIO is a Linux-platform feature: off a GPIO-capable host every call reports a
 * clear "base directory not found" error rather than crashing.
 * @module gpio
 * @example
 * import "gpio.j" as gpio;
 * gpio.setup(17, gpio.OUT);
 * gpio.write(17, 1);
 * gpio.release(17);
 */

use fs;
use os;
use convert;
use strings;
use time;

def const DEFAULT_BASE as string init "/sys/class/gpio";
def const BASE_ENV as string init "JENNIFER_GPIO_BASE";

# Pin-direction values for gpio.setup. These are the sysfs `direction` strings,
# exported as named constants so callers write `gpio.setup(17, gpio.OUT)` rather
# than a bare `"out"` (matching RPi.GPIO's `GPIO.IN` / `GPIO.OUT`). A raw "in" /
# "out" string still works, since the constant values are exactly those strings.
export def const IN as string init "in";
export def const OUT as string init "out";

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

# gpioWrite / gpioRead wrap sysfs I/O so a hardware failure surfaces as a
# gpio-kind error, not a raw fs-kind one - callers catching `kind == "gpio"`
# would otherwise miss every real I/O failure.
func gpioWrite(path as string, data as string) {
    try {
        fs.writeString($path, $data);
    } catch (err) {
        throw Error{ kind: "gpio", message: "gpio: write to " + $path + " failed: " + $err.message, file: "", line: 0, col: 0 };
    }
}

func gpioRead(path as string) {
    try {
        return fs.readString($path);
    } catch (err) {
        throw Error{ kind: "gpio", message: "gpio: read from " + $path + " failed: " + $err.message, file: "", line: 0, col: 0 };
    }
}

/**
 * Export a pin and set its direction (gpio.IN or gpio.OUT).
 * @param pin {int} the GPIO pin number
 * @param direction {string} the pin direction, gpio.IN or gpio.OUT (the "in" / "out" strings)
 * @throws {Error} when direction is not gpio.IN/gpio.OUT, or the sysfs GPIO tree is absent
 */
export func setup(pin as int, direction as string) {
    if (not ($direction == IN or $direction == OUT)) {
        throw Error{ kind: "gpio", message: "gpio.setup: direction must be gpio.IN (\"" + IN + "\") or gpio.OUT (\"" + OUT + "\"), got \"" + $direction + "\"", file: "", line: 0, col: 0 };
    }
    def dir as string init base();
    requireBase($dir);
    def pinStr as string init convert.toString($pin);
    def pd as string init pinDir($dir, $pin);
    # Export only when the pin is not already exported: writing `export` for an
    # already-exported pin fails with EBUSY on real sysfs, so a second setup
    # (a re-run after a crash, or reconfiguring the direction) would throw.
    if (not fs.exists($pd)) {
        gpioWrite($dir + "/export", $pinStr);
    }
    # The kernel creates gpioN on export; mkdirAll is a no-op when it already
    # exists (real sysfs) and creates it on a mock tree.
    fs.mkdirAll($pd);
    # Retry the direction write briefly: after export, udev needs a moment to
    # grant write permission on the new attribute files, so a non-root write
    # can transiently fail with EACCES (the canonical sysfs-GPIO flake).
    def attempt as int init 0;
    def wrote as bool init false;
    while (not $wrote) {
        try {
            fs.writeString($pd + "/direction", $direction);
            $wrote = true;
        } catch (err) {
            $attempt = $attempt + 1;
            if ($attempt >= 10) {
                throw Error{ kind: "gpio", message: "gpio.setup: could not set direction after export (udev permission delay?): " + $err.message, file: "", line: 0, col: 0 };
            }
            time.sleep(time.fromMilliseconds(20));
        }
    }
}

/**
 * Set an output pin's value (0 or 1).
 * @param pin {int} the GPIO pin number
 * @param value {int} the value to write, 0 or 1
 * @throws {Error} when value is not 0/1, or the sysfs GPIO tree is absent
 */
export func write(pin as int, value as int) {
    if (not ($value == 0 or $value == 1)) {
        throw Error{ kind: "gpio", message: "gpio.write: value must be 0 or 1", file: "", line: 0, col: 0 };
    }
    def dir as string init base();
    requireBase($dir);
    gpioWrite(pinDir($dir, $pin) + "/value", convert.toString($value));
}

/**
 * Return a pin's current value (0 or 1).
 * @param pin {int} the GPIO pin number
 * @return {int} the pin's current value, 0 or 1
 * @throws {Error} when the sysfs GPIO tree is absent
 */
export func read(pin as int) {
    def dir as string init base();
    requireBase($dir);
    def raw as string init gpioRead(pinDir($dir, $pin) + "/value");
    return convert.toInt(strings.trim($raw));
}

/**
 * Unexport a pin.
 * @param pin {int} the GPIO pin number
 * @throws {Error} when the sysfs GPIO tree is absent
 */
export func release(pin as int) {
    def dir as string init base();
    requireBase($dir);
    gpioWrite($dir + "/unexport", convert.toString($pin));
}
