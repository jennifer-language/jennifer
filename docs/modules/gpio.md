# `gpio` - Raspberry-Pi GPIO over sysfs

`import "gpio.j" as gpio;`

General-purpose I/O pins on a Raspberry Pi (or any Linux single-board computer)
through the sysfs interface. `/sys/class/gpio` is plain files, so
[`fs`](../libraries/fs.md) is the entire backend: no system library and no build
tag. Blink an LED:

```jennifer
import "gpio.j" as gpio;
gpio.setup(17, gpio.OUT);
gpio.write(17, 1);   # LED on
gpio.write(17, 0);   # LED off
gpio.release(17);
```

## Surface

Stateless and pin-keyed - no handle is needed:

| Call | Returns | |
| ---- | ------- | - |
| `gpio.setup(pin, direction)` | `null` | Export `pin` and set its direction: `gpio.IN` or `gpio.OUT`. An output starts low. |
| `gpio.write(pin, value)` | `null` | Set an output pin's value: `0` or `1`. |
| `gpio.read(pin)` | `int` | Read a pin's current value (`0` / `1`). |
| `gpio.release(pin)` | `null` | Unexport `pin`. |
| `gpio.useChip(label)` | `null` | Select the chip the pin numbers refer to (see below): `gpio.AUTO` (default), a chip label, or `gpio.GLOBAL`. |

The direction is passed as one of two exported constants, `gpio.IN` / `gpio.OUT`
(matching RPi.GPIO's `GPIO.IN` / `GPIO.OUT`). Their values are the sysfs strings
`"in"` / `"out"`, so a raw string also works. A bad direction or a value other
than `0` / `1` throws `Error{kind: "gpio"}`. When the sysfs GPIO tree is absent
(not a GPIO-capable host, or sysfs GPIO disabled) every call throws a positioned
`Error{kind: "gpio"}` ("base directory not found: ...").

## Pin numbers

A pin is a **line offset on one GPIO chip** - on a Raspberry Pi the BCM GPIO
number (`17` is header pin 11). sysfs itself wants the *global* number, the
chip's base plus the offset, and the base is not fixed: kernels before 6.2 put
the Pi header chip at base 0, newer ones allocate bases from 512 (Pi 4 header at
512, Pi 5 header at 571 or so). Every call therefore reads
`/sys/class/gpio/gpiochip*/{base,label,ngpio}` and maps the pin through the
selected chip:

- **`gpio.AUTO`** (the default): the Raspberry Pi header chip, recognised by its
  label - `pinctrl-bcm2835` (Pi 1-3, Zero), `pinctrl-bcm2711` (Pi 4, 400) or
  `pinctrl-rp1` (Pi 5). Exactly one chip must carry one of these labels; none
  (another board) or several throws `Error{kind: "gpio"}` listing the chips
  present, rather than guessing.
- **a label**, e.g. `gpio.useChip("gpio-brcmstb@107d508500")`: the one chip with
  exactly that sysfs `label`.
- **`gpio.GLOBAL`** (`"global"`): pin numbers are global numbers, written to
  `export` as given, for boards whose scripts already use global numbers.

A pin at or past the chip's `ngpio` lines throws. `gpio.useChip` sets the
`JENNIFER_GPIO_CHIP_LABEL` environment variable for the process (the module
holds no state), so a deployment can also set it outside the program.

Right after `export`, udev takes a moment to grant a non-root user write access
to the new `direction` and `value` files. `setup` retries those writes for about
1 s in total (for an output it writes the initial `0` to `value`, so the first
`gpio.write` does not race udev), then throws.

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
on a laptop. A *platform*-bound capability is **absent** off its platform (so
`import` fails at the top with a clear message), unlike a *toolchain*-bound one
like `net`, which stubs on `jennifer-tiny` because the same source must load in
both binaries.

## sysfs, and the future

sysfs GPIO is deprecated in favour of the `/dev/gpiochip` character device and
can be compiled out of a kernel; it is expected to stay available on the
hobbyist Pi kernels this targets. The pure-module form stays the default because
it needs no interpreter support.

The `/dev/gpiochip` backend is the [`gpio` library](../libraries/gpio.md) (the
`/dev/gpiochipN` GPIO v2 ioctls). It reuses this module's pin-keyed shape -
`setup` / `read` / `write` / `release` plus the `IN` / `OUT` constants. Both
take a pin as a line offset on a chip, so a script moves between the two
unchanged **when both name the header chip**: this module finds it by label, the
library opens `/dev/gpiochip0` unless `gpio.chip(path)` picks another. On a
Pi 1-4 the header is `/dev/gpiochip0`; on a Pi 5 it is `/dev/gpiochip0` on
current kernels and `/dev/gpiochip4` on early ones (`gpioinfo` shows which
carries `pinctrl-rp1`). `gpio.useChip` and `gpio.chip` differ: a sysfs label
here, a device path there. This sysfs module stays the portable default; the
library is the successor for targets that ship without sysfs GPIO.

## See also

- [`gpio` library](../libraries/gpio.md) - the `/dev/gpiochip` character-device
  successor with the same pin-keyed surface (default `jennifer` binary, Linux).
- [`fs`](../libraries/fs.md) - the file I/O behind every call.
- [`os`](../libraries/os.md) - `os.setEnv` to point `JENNIFER_GPIO_BASE` at a
  non-standard sysfs mount or a mock (a mock needs a `gpiochipN` directory with
  `base`, `label` and `ngpio` files, as `gpio_test.j` builds).

## Academy talks

| Talk | What it shows |
| ---- | ------------- |
| [Hardware with Jennifer: GPIO, I2C, SPI and serial](https://academy.jennifer-lang.dev/talks/mplx/hardware/) | blinking an LED and debouncing a button through sysfs |

## API reference

Every exported function, struct and constant of `gpio`, with its signature, generated
from the module's doc comments: [the `gpio` API reference](../api/gpio.md).
