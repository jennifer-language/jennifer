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
| `i2c.read(bus, n)` | `bytes` | Reads `n` raw bytes from the selected slave. |
| `i2c.write(bus, data)` | `int` | Writes raw bytes; returns the count written. |
| `i2c.readReg(bus, reg, n)` | `bytes` | Writes the 1-byte register pointer `reg`, then reads `n` bytes (the common "set register, read back"). |
| `i2c.writeReg(bus, reg, data)` | `int` | Writes `reg` followed by `data` in one transaction; returns the data-byte count. |
| `i2c.close(bus)` | `null` | Closes the bus. |

Slave selection is the `I2C_SLAVE` ioctl - the reason a Go library is needed
rather than plain `fs`. Blocking; compose with `spawn`.

## See also

[serial](serial.md), [spi](spi.md), [gpio](gpio.md), [fs](fs.md).
