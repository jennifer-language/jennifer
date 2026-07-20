# `gpio` - Raspberry-Pi GPIO over sysfs

`import "gpio.j" as gpio;`

General-purpose I/O pins on a Raspberry Pi (or any Linux single-board computer)
through the sysfs interface - the physical-computing / IoT-teaching use case.
`/sys/class/gpio` is plain files, so [`fs`](../libraries/fs.md) is the entire
backend; there is no core change, no system library, and no build tag. Blink an
LED from a five-line script:

```jennifer
import "gpio.j" as gpio;
gpio.setup(17, gpio.OUT);
gpio.write(17, 1);   # LED on
gpio.write(17, 0);   # LED off
gpio.release(17);
```

## Surface

Stateless and pin-keyed - sysfs derives every path from the pin number, so no
handle is needed:

| Call | Returns | |
| ---- | ------- | - |
| `gpio.setup(pin, direction)` | `null` | Export `pin` and set its direction: `gpio.IN` or `gpio.OUT`. |
| `gpio.write(pin, value)` | `null` | Set an output pin's value: `0` or `1`. |
| `gpio.read(pin)` | `int` | Read a pin's current value (`0` / `1`). |
| `gpio.release(pin)` | `null` | Unexport `pin`. |

The direction is passed as one of two exported constants, `gpio.IN` / `gpio.OUT`
(matching RPi.GPIO's `GPIO.IN` / `GPIO.OUT`). Their values are the sysfs strings
`"in"` / `"out"`, so a raw string still works, but the constant is the readable
form. A bad direction (not `gpio.IN` / `gpio.OUT`) or a value other than `0` / `1` throws
`Error{kind: "gpio"}`. When the sysfs GPIO tree is absent - not a GPIO-capable
host, or sysfs GPIO disabled - every call throws a clear positioned
`Error{kind: "gpio"}` ("base directory not found: ...") rather than crashing.

## The sysfs root

The root defaults to `/sys/class/gpio`. Set the **`JENNIFER_GPIO_BASE`**
environment variable to point elsewhere - a differently mounted sysfs, or a
mock tree under test:

```jennifer
use os;
os.setEnv("JENNIFER_GPIO_BASE", "/tmp/mock-gpio");   # e.g. in a test
```

That is how [`gpio_test.j`](../../modules/gpio_test.j) drives the module against
a temp-dir mock, and how [`gpio_demo.j`](../../examples/modules/gpio_demo.j)
runs on any machine (no hardware, no root) while making the same calls you'd run
on a Pi.

## Why a module, not a system library

`use` / `import` are static - they resolve before execution, are uncatchable,
and can't be conditional - so there is no "check the platform, then import."
The portability seam is instead **which module file is on the search path**
(Go uses build tags; Jennifer uses the module file). A program writes one
uniform line, `import "gpio.j" as gpio;`, and the deployment supplies the right
`gpio.j`: this sysfs module on a Pi, or an emulator that blinks a console cell
on a laptop. A genuinely *platform*-bound capability being **absent** off its
platform (so `import` fails at the top with a clear message) is the right shape
- distinct from a *toolchain*-bound one like `net`, which stubs on
`jennifer-tiny` because the same source must load in both binaries.

## sysfs, and the future

sysfs GPIO is deprecated in favour of the `/dev/gpiochip` character device and
can be compiled out of a kernel. The bet is that it stays available on the
hobbyist Pi kernels this targets. The API is kept deliberately stable, so if
sysfs is ever removed the backend can be swapped for a `/dev/gpiochip` ioctl
system library with **no change to `.j` scripts** - the pure-module form is the
default because it costs the language nothing; the system library is
future-proofing, taken only when forced.

That system library now exists as the [`gpio` library](../libraries/gpio.md)
(the `/dev/gpiochipN` GPIO v2 ioctls). It reuses this module's pin-keyed shape -
`setup` / `read` / `write` / `release` plus the `IN` / `OUT` constants - so a
script moves between the two unchanged; this sysfs module stays the portable
default, and the library is the successor for targets that ship without sysfs
GPIO.

## See also

- [`gpio` library](../libraries/gpio.md) - the `/dev/gpiochip` character-device
  successor with the same pin-keyed surface (default `jennifer` binary, Linux).
- [`fs`](../libraries/fs.md) - the file I/O behind every call.
- [`os`](../libraries/os.md) - `os.setEnv` to point `JENNIFER_GPIO_BASE` at a
  non-standard sysfs mount or a mock.
