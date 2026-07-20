# gpio

GPIO over the modern `/dev/gpiochipN` **character device** (the GPIO v2 line
ioctls), the mainline-supported interface since sysfs `/sys/class/gpio` was
deprecated. **Linux-only**; a stub elsewhere and on `jennifer-tiny`.

This library reuses the pin-keyed shape of the sysfs-backed `gpio`
[module](../modules/gpio.md) - `setup` / `read` / `write` / `release` plus the
`IN` / `OUT` direction constants - so a script moves between the two unchanged.
The chip device is selected once (default `/dev/gpiochip0`).

```jennifer
use gpio;

gpio.setup(17, gpio.OUT);
gpio.write(17, 1);
gpio.release(17);
```

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `gpio.chip(path)` | `null` | Selects the gpiochip device for later setups (default `/dev/gpiochip0`; env `JENNIFER_GPIO_CHIP` also honoured). |
| `gpio.setup(pin, direction)` | `null` | Requests `pin` (0..63) with `gpio.IN` or `gpio.OUT`. Errors if the pin is already set up. |
| `gpio.read(pin)` | `int` | 0 or 1. |
| `gpio.write(pin, value)` | `null` | `value` is 0 or 1; the pin must be set up as `gpio.OUT`. |
| `gpio.release(pin)` | `null` | Releases the line back to the kernel. |

### Constants

- `gpio.IN` = `"in"`, `gpio.OUT` = `"out"` - the same values as the sysfs module.

The sysfs `gpio` module stays the portable default (it runs on the hobbyist Pi
kernels it targets); this ioctl library is the successor for targets that ship
without sysfs GPIO.

## See also

[gpio module](../modules/gpio.md), [serial](serial.md), [spi](spi.md), [iic](iic.md).
