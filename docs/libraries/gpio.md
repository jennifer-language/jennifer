# gpio

GPIO over the `/dev/gpiochipN` **character device** (the GPIO v2 line ioctls),
the mainline interface that replaces the deprecated sysfs `/sys/class/gpio`.
**Linux-only**; a stub elsewhere and on `jennifer-tiny`.

A chip is a handle: `gpio.open(path)` opens one, and `setup` / `read` /
`write` / `release` take it with the pin, a line offset on that chip (on a
Raspberry Pi the BCM GPIO number). The header chip of a Raspberry Pi is
`/dev/gpiochip0` on Pi 1-4, and on Pi 5 with current kernels (early Pi 5
kernels number it `/dev/gpiochip4`; `gpiodetect` lists each node with its
label, `pinctrl-rp1` on a Pi 5).

```jennifer
use gpio;

def chip as gpio.Chip init gpio.open("/dev/gpiochip0");
defer gpio.close($chip);          # releases the chip's pins too
gpio.setup($chip, 17, gpio.OUT);
gpio.write($chip, 17, 1);
```

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `gpio.open(path)` | `gpio.Chip` | Opens a gpiochip device; a node that is not one throws (`not a gpiochip device`). |
| `gpio.setup(chip, pin, direction)` | `null` | Requests line `pin` (a u32 offset; many SoC gpiochips expose 100+ lines) with `gpio.IN` or `gpio.OUT`. An offset the chip does not have surfaces as the kernel's error. Throws if the pin is already set up on this chip. |
| `gpio.read(chip, pin)` | `int` | 0 or 1. |
| `gpio.write(chip, pin, value)` | `null` | `value` is 0 or 1; the pin must be set up as `gpio.OUT`. |
| `gpio.release(chip, pin)` | `null` | Releases the line back to the kernel. |
| `gpio.close(chip)` | `null` | Releases every line set up on the chip and closes it. A second `close` does nothing. |

Two chip handles are independent: the same pin number on two chips is two
lines. Any call but `close` on a closed chip throws `Error{kind: "closed"}`.

## Deprecated: the process-wide chip

Removed in 0.29.0; `jennifer lint` reports each use (L301).

| Deprecated | Use |
|---|---|
| `gpio.chip(path)`, `JENNIFER_GPIO_CHIP` | `gpio.open(path)` |
| `gpio.setup(pin, direction)` | `gpio.setup(chip, pin, direction)` |
| `gpio.read(pin)` / `gpio.write(pin, value)` | `gpio.read(chip, pin)` / `gpio.write(chip, pin, value)` |
| `gpio.release(pin)` | `gpio.release(chip, pin)` or `gpio.close(chip)` |

The pin-only forms use the chip `gpio.chip` selected, else `JENNIFER_GPIO_CHIP`,
else `/dev/gpiochip0`, opened per `setup`. Their pins are separate from those of
a chip handle.

## Constants

| Constant | Value |
| -------- | ----- |
| `gpio.IN` | `"in"` |
| `gpio.OUT` | `"out"` |

## Errors

| Kind | Raised for |
|---|---|
| `"closed"` | a call on a closed `gpio.Chip` |
| `"runtime"` | misuse (a bad direction or value, a pin not set up or set up twice) and an operating-system failure (`gpio.open: /dev/gpiochip0: permission denied (add the user to the group that owns the device, often gpio)`) |

## See also

[gpio module](../modules/gpio.md) (sysfs, deprecated), [serial](serial.md), [spi](spi.md), [i2c](i2c.md).
