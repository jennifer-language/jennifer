# i2c

The I2C (Inter-IC) bus (`/dev/i2c-1`, ...). **Linux-only**; a stub
elsewhere and on `jennifer-tiny`.

```jennifer
use i2c;

def bus as i2c.Bus init i2c.open("/dev/i2c-1", 0x76);   # 7-bit slave address
def id as bytes init i2c.readReg($bus, 0xd0, 1);        # read register 0xd0
i2c.close($bus);
```

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `i2c.open(path, addr)` | `i2c.Bus` | Opens the bus and selects the 7-bit slave `addr` (0..127; the usable range is 0x08..0x77). |
| `i2c.read(bus, n)` | `bytes` | Reads up to `n` raw bytes (at most 8192) from the selected slave. |
| `i2c.write(bus, data)` | `int` | Writes raw bytes (at most 8192); returns the count written. |
| `i2c.readReg(bus, reg, n)` | `bytes` | Writes the 1-byte register pointer `reg`, then reads up to `n` bytes (at most 8192) as a separate transfer: a STOP between them (use `writeRead` for a device that needs a repeated start). No other transfer on the bus runs in between. |
| `i2c.writeReg(bus, reg, data)` | `int` | Writes `reg` followed by `data` (at most 8191 bytes) in one transaction; returns the data-byte count. |
| `i2c.writeRead(bus, data, n)` | `bytes` | Writes `data` (1..8192 bytes), then reads exactly `n` bytes (1..8192) in one combined transfer (`I2C_RDWR`): a repeated START between the two, no STOP. For sensors that drop the register pointer at a STOP, and 16-bit register addresses (`data` = both address bytes). Over 8192 throws `Error{kind: "limit"}`; an adapter without combined transfers fails with "operation not supported". |
| `i2c.close(bus)` | `null` | Closes the bus. A second `close` does nothing; any other call on a closed bus throws `Error{kind: "closed"}`. |

One message moves at most 8192 bytes (the i2c-dev limit). A longer `read`,
`write`, `readReg`, `writeReg` or `writeRead` throws `Error{kind: "limit"}`
rather than moving a shortened message.

Slave selection is the `I2C_SLAVE` ioctl, which is why this is a library
rather than plain `fs`. Blocking; compose with `spawn`.

`I2C_SLAVE_FORCE` is not used: when a kernel driver has bound the address (an
RTC, a sensor with an IIO driver), `i2c.open` fails with "device or resource
busy". Unbind the driver (or drop its device-tree overlay) to use the device
from a script.

Errors name the call and address and carry a hint for the usual errno values:
"no such device or address" / "remote I/O error" mean the device did not ACK
(wrong address, unpowered, or busy); "permission denied" usually means the user
is not in the `i2c` group. An adapter timeout throws `Error{kind: "timeout"}`;
a message over 8192 bytes, `Error{kind: "limit"}`; a call on a closed bus,
`Error{kind: "closed"}`; other failures are runtime errors.

## See also

[serial](serial.md), [spi](spi.md), [gpio](gpio.md), [fs](fs.md).

## Academy talks

| Talk | What it shows |
| ---- | ------------- |
| [Hardware with Jennifer: GPIO, I2C, SPI and serial](https://academy.jennifer-lang.dev/talks/mplx/hardware/) | reading an SHT31 temperature sensor over I2C |
