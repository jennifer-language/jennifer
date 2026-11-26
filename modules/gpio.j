# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
# pragma-jennifer-version: >=0.25.0

/**
 * Raspberry-Pi (and any Linux SBC) GPIO over sysfs. /sys/class/gpio is plain
 * files, so `fs` is the whole backend: export a pin, set its direction, read /
 * write its value, unexport it. "Blink an LED from a five-line script." It is
 * stateless and pin-keyed (no handle is needed). A pin is the line offset on the
 * Raspberry Pi header chip - the BCM GPIO number - and each call adds that chip's
 * sysfs base (0 on old kernels, 512 and up since Linux 6.2) to get the global
 * number sysfs wants. The header chip is found by its label (pinctrl-bcm2835,
 * pinctrl-bcm2711, pinctrl-rp1); `gpio.useChip` picks another chip by label or
 * switches to raw global numbers (gpio.GLOBAL). The sysfs root defaults to
 * /sys/class/gpio; set the JENNIFER_GPIO_BASE environment variable to point
 * elsewhere (a differently mounted sysfs, or a mock tree under test). This is
 * the sysfs backend, which is deprecated in favour of the /dev/gpiochip
 * character device but stays available on the hobbyist Pi kernels this targets.
 * GPIO is a Linux-platform feature: off a GPIO-capable host every call reports a
 * clear "base directory not found" error rather than crashing.
 *
 * Deprecated: the `gpio` library (`use gpio;`) drives the same pins over the
 * /dev/gpiochipN character device, the interface the kernel keeps. Open the
 * chip once and pass it to each call: `gpio.open("/dev/gpiochip0")`, then
 * `gpio.setup($chip, 17, gpio.OUT)`, `gpio.write($chip, 17, 1)`, ... The
 * module is removed in 0.29.0.
 * @module gpio
 * @deprecated use the `gpio` library (`gpio.open(path)` -> `gpio.Chip`); removed in 0.29.0
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
def const CHIP_ENV as string init "JENNIFER_GPIO_CHIP_LABEL";

# The labels of the chip that drives the Raspberry Pi 40-pin header: Pi 1-3 and
# Zero (pinctrl-bcm2835), Pi 4 / 400 (pinctrl-bcm2711), Pi 5 (pinctrl-rp1, the
# RP1 south bridge).
def const HEADER_LABELS as list of string init [
    "pinctrl-bcm2835",
    "pinctrl-bcm2711",
    "pinctrl-rp1"
];

# Attempts and pause of the post-export wait for udev (50 x 20 ms, about 1 s in
# total across the direction and value writes of one setup).
def const SETTLE_ATTEMPTS as int init 50;
def const SETTLE_PAUSE_MS as int init 20;

/**
 * Chip selection for `gpio.useChip`: find the Raspberry Pi header chip by its
 * label (the default).
 */
export def const AUTO as string init "";

/**
 * Chip selection for `gpio.useChip`: pin numbers are sysfs global numbers,
 * written to `export` as given (no chip lookup, no range check).
 */
export def const GLOBAL as string init "global";

/**
 * Pin direction "input", for `gpio.setup`: the sysfs `direction` string, named so
 * callers write `gpio.setup(17, gpio.IN)` (as RPi.GPIO's `GPIO.IN`). A raw "in"
 * still works, since the value is exactly that string.
 */
export def const IN as string init "in";

/**
 * Pin direction "output", for `gpio.setup` (`gpio.setup(17, gpio.OUT)`); the
 * sysfs `direction` string "out".
 */
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
        throw Error{
            kind: "gpio",
            message: "gpio: base directory not found: " + $dir +
                " (not a GPIO-capable Linux host, or sysfs GPIO disabled)"
        };
    }
}

# pinDir is the per-pin sysfs directory (dir/gpioN) of a global number.
func pinDir(dir as string, global as int) {
    return $dir + "/gpio{$global}";
}

func checkPin(pin as int) {
    if ($pin < 0) {
        throw Error{kind: "gpio", message: "gpio: pin must be >= 0, got {$pin}"};
    }
}

# gpioWrite / gpioRead wrap sysfs I/O so a hardware failure surfaces as a
# gpio-kind error, not a raw fs-kind one - callers catching `kind == "gpio"`
# would otherwise miss every real I/O failure.
func gpioWrite(path as string, data as string) {
    try {
        fs.writeString($path, $data);
    } catch (err) {
        throw Error{kind: "gpio", message: "gpio: write to " + $path + " failed: " + $err.message};
    }
}

func gpioRead(path as string) {
    try {
        return fs.readString($path);
    } catch (err) {
        throw Error{kind: "gpio", message: "gpio: read from " + $path + " failed: " + $err.message};
    }
}

# Chip is one /sys/class/gpio/gpiochipN entry: N is its base, the first global
# number of its ngpio lines.
def struct Chip {
    name as string,
    label as string,
    base as int,
    ngpio as int
};

# chipInt reads a non-negative decimal attribute (base, ngpio) of a chip.
func chipInt(path as string) {
    def raw as string init strings.trim(gpioRead($path));
    def n as int init -1;
    try {
        $n = convert.toInt($raw);
    } catch (err) {
        throw Error{kind: "gpio", message: "gpio: " + $path + " is not a number: '" + $raw + "'"};
    }
    if ($n < 0) {
        throw Error{kind: "gpio", message: "gpio: " + $path + " is negative: '" + $raw + "'"};
    }
    return $n;
}

# chips lists the gpiochipN entries under the sysfs root.
func chips(dir as string) {
    def found as list of Chip;
    for (def name in fs.list($dir)) {
        if (strings.startsWith($name, "gpiochip")) {
            def cd as string init $dir + "/" + $name;
            $found[] = Chip{
                name: $name,
                label: strings.trim(gpioRead($cd + "/label")),
                base: chipInt($cd + "/base"),
                ngpio: chipInt($cd + "/ngpio")
            };
        }
    }
    return $found;
}

func describe(found as list of Chip) {
    if (len($found) == 0) {
        return "none";
    }
    def parts as list of string;
    for (def c in $found) {
        $parts[] = $c.name + " [" + $c.label + "]";
    }
    return strings.join($parts, ", ");
}

func isHeaderLabel(label as string) {
    for (def l in HEADER_LABELS) {
        if ($l == $label) {
            return true;
        }
    }
    return false;
}

# globalNumber maps a pin to the global number sysfs uses. With gpio.GLOBAL
# selected it is the pin itself. Otherwise exactly one chip must match - the
# label chosen with gpio.useChip, or else one of HEADER_LABELS - and the pin
# must be one of its lines; no match or several is an error, never a guess.
func globalNumber(dir as string, pin as int) {
    checkPin($pin);
    def want as string init os.getEnv(CHIP_ENV);
    if ($want == GLOBAL) {
        return $pin;
    }
    def found as list of Chip init chips($dir);
    def picked as list of Chip;
    for (def c in $found) {
        if (($want == AUTO and isHeaderLabel($c.label)) or ($want != AUTO and $c.label == $want)) {
            $picked[] = $c;
        }
    }
    def what as string init "the Raspberry Pi header chip (label " +
        strings.join(HEADER_LABELS, " / ") + ")";
    if ($want != AUTO) {
        $what = "the chip labelled '" + $want + "'";
    }
    if (len($picked) != 1) {
        def problem as string init "no chip under " + $dir + " is " + $what + " (chips present: " +
            describe($found) + ")";
        if (len($picked) > 1) {
            $problem = "several chips under " + $dir + " match " + $what + ": " + describe($picked);
        }
        throw Error{
            kind: "gpio",
            message: "gpio: " + $problem +
                ". Select one with gpio.useChip(label), or gpio.useChip(gpio.GLOBAL) " +
                "for global numbers"
        };
    }
    def c as Chip init $picked[0];
    if ($pin >= $c.ngpio) {
        throw Error{
            kind: "gpio",
            message: "gpio: pin {$pin} is out of range for " + $c.name + " [" + $c.label +
                "], which has " + convert.toString($c.ngpio) + " lines (0.." +
                convert.toString($c.ngpio - 1) + ")"
        };
    }
    return $c.base + $pin;
}

# settle writes data to path, retrying while udev has not yet granted write
# permission on a freshly exported pin's attribute files (a non-root write
# fails with EACCES for a moment: the canonical sysfs-GPIO flake). It returns
# the attempts left, so one setup shares one bounded budget of about 1 s.
func settle(path as string, data as string, attempts as int, what as string) {
    def left as int init $attempts;
    while (true) {
        try {
            fs.writeString($path, $data);
            return $left;
        } catch (err) {
            $left = $left - 1;
            if ($left <= 0) {
                throw Error{
                    kind: "gpio",
                    message: "gpio.setup: could not set " + $what +
                        " after export (udev permission delay?): " + $err.message
                };
            }
            time.sleep(time.fromMilliseconds(SETTLE_PAUSE_MS));
        }
    }
}

/**
 * Select the chip whose lines the pin numbers name, for this process (it sets
 * the JENNIFER_GPIO_CHIP_LABEL environment variable, which can also be set
 * outside the program). gpio.AUTO (the default) finds the Raspberry Pi header
 * chip; a label selects the chip with exactly that sysfs `label`; gpio.GLOBAL
 * makes pin numbers sysfs global numbers.
 * @param label {string} gpio.AUTO, gpio.GLOBAL, or a chip label such as "pinctrl-bcm2711"
 * @deprecated use `gpio.open(path)` from the `gpio` library; removed in 0.29.0
 */
export func useChip(label as string) {
    os.setEnv(CHIP_ENV, $label);
}

/**
 * Export a pin and set its direction (gpio.IN or gpio.OUT). An output starts
 * low. Right after the export, writes are retried for about 1 s in total while
 * udev grants access to the new attribute files.
 * @param pin {int} the line offset on the selected chip (the BCM GPIO number on a Raspberry Pi)
 * @param direction {string} the pin direction, gpio.IN or gpio.OUT (the "in" / "out" strings)
 * @throws {Error} kind "gpio" on a negative pin, a direction other than gpio.IN / gpio.OUT, an absent sysfs GPIO tree, no or several matching chips, a pin past the chip's lines, or a write failure
 * @deprecated use `gpio.setup(chip, pin, direction)` from the `gpio` library; removed in 0.29.0
 */
export func setup(pin as int, direction as string) {
    if (not ($direction == IN or $direction == OUT)) {
        throw Error{
            kind: "gpio",
            message: "gpio.setup: direction must be gpio.IN (\"" + IN + "\") or gpio.OUT (\"" +
                OUT + "\"), got \"" + $direction + "\""
        };
    }
    checkPin($pin);
    def dir as string init base();
    requireBase($dir);
    def global as int init globalNumber($dir, $pin);
    def pd as string init pinDir($dir, $global);
    # Export only when the pin is not already exported: writing `export` for an
    # already-exported pin fails with EBUSY on real sysfs, so a second setup
    # (a re-run after a crash, or reconfiguring the direction) would throw.
    if (not fs.exists($pd)) {
        gpioWrite($dir + "/export", convert.toString($global));
    }
    # The kernel creates gpioN on export; mkdirAll is a no-op when it already
    # exists (real sysfs) and creates it on a mock tree.
    fs.mkdirAll($pd);
    def left as int init settle($pd + "/direction", $direction, SETTLE_ATTEMPTS, "direction");
    # "out" drives the line low; writing that 0 again waits until `value` is
    # writable too, so the first gpio.write does not hit the udev delay.
    if ($direction == OUT) {
        settle($pd + "/value", "0", $left, "value");
    }
}

/**
 * Set an output pin's value (0 or 1).
 * @param pin {int} the line offset on the selected chip (the BCM GPIO number on a Raspberry Pi)
 * @param value {int} the value to write, 0 or 1
 * @throws {Error} kind "gpio" on a negative pin, a value other than 0 / 1, an absent sysfs GPIO tree, no or several matching chips, a pin past the chip's lines, or a write failure
 * @deprecated use `gpio.write(chip, pin, value)` from the `gpio` library; removed in 0.29.0
 */
export func write(pin as int, value as int) {
    if (not ($value == 0 or $value == 1)) {
        throw Error{kind: "gpio", message: "gpio.write: value must be 0 or 1"};
    }
    def dir as string init base();
    requireBase($dir);
    gpioWrite(pinDir($dir, globalNumber($dir, $pin)) + "/value", convert.toString($value));
}

/**
 * Return a pin's current value (0 or 1).
 * @param pin {int} the line offset on the selected chip (the BCM GPIO number on a Raspberry Pi)
 * @return {int} the pin's current value, 0 or 1
 * @throws {Error} kind "gpio" on a negative pin, an absent sysfs GPIO tree, no or several matching chips, a pin past the chip's lines, a read failure, or a value file holding anything but 0 / 1
 * @deprecated use `gpio.read(chip, pin)` from the `gpio` library; removed in 0.29.0
 */
export func read(pin as int) {
    def dir as string init base();
    requireBase($dir);
    def path as string init pinDir($dir, globalNumber($dir, $pin)) + "/value";
    def raw as string init strings.trim(gpioRead($path));
    if ($raw != "0" and $raw != "1") {
        throw Error{
            kind: "gpio",
            message: "gpio.read: pin {$pin} value is not 0 or 1: '" + $raw + "'"
        };
    }
    return convert.toInt($raw);
}

/**
 * Unexport a pin.
 * @param pin {int} the line offset on the selected chip (the BCM GPIO number on a Raspberry Pi)
 * @throws {Error} kind "gpio" on a negative pin, an absent sysfs GPIO tree, no or several matching chips, a pin past the chip's lines, or a write failure
 * @deprecated use `gpio.release(chip, pin)` from the `gpio` library; removed in 0.29.0
 */
export func release(pin as int) {
    checkPin($pin);
    def dir as string init base();
    requireBase($dir);
    gpioWrite($dir + "/unexport", convert.toString(globalNumber($dir, $pin)));
}
