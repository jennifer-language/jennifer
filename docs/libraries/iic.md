# iic

The I2C bus (`/dev/i2c-1`, ...). Named **`iic`** (Inter-IC) because a library
namespace is letters-only, so `i2c` is not spellable. **Linux-only**; a stub
elsewhere and on `jennifer-tiny`.

```jennifer
use iic;

def bus as iic.Bus init iic.open("/dev/i2c-1", 0x76);   # 7-bit slave address
def id as bytes init iic.readReg($bus, 0xd0, 1);        # read register 0xd0
iic.close($bus);
```

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `iic.open(path, addr)` | `iic.Bus` | Opens the bus and selects the 7-bit slave `addr` (0..127; the usable range is 0x08..0x77). |
| `iic.read(bus, n)` | `bytes` | Reads `n` raw bytes from the selected slave. |
| `iic.write(bus, data)` | `int` | Writes raw bytes; returns the count written. |
| `iic.readReg(bus, reg, n)` | `bytes` | Writes the 1-byte register pointer `reg`, then reads `n` bytes (the common "set register, read back"). |
| `iic.writeReg(bus, reg, data)` | `int` | Writes `reg` followed by `data` in one transaction; returns the data-byte count. |
| `iic.close(bus)` | `null` | Closes the bus. |

Slave selection is the `I2C_SLAVE` ioctl - the reason a Go library is needed
rather than plain `fs`. Blocking; compose with `spawn`.

## See also

[serial](serial.md), [spi](spi.md), [gpio](gpio.md), [fs](fs.md).
