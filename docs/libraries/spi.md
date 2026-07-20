# spi

SPI devices (`/dev/spidev0.0`, ...). Open a device, set mode / speed, then
`transfer` bytes full-duplex (write and read happen together over one exchange).
**Linux-only**; a stub elsewhere and on `jennifer-tiny`.

```jennifer
use spi;
use convert;

def dev as spi.Device init spi.open("/dev/spidev0.0");
spi.configure($dev, 0, 1000000);                        # mode 0, 1 MHz
def rx as bytes init spi.transfer($dev, convert.bytesFromString("\x9f", "utf-8"));
spi.close($dev);
```

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `spi.open(path)` | `spi.Device` | Opens the device (default mode 0, 500 kHz, 8 bits/word). |
| `spi.configure(dev, mode, speedHz)` | `null` | Sets clock mode (0..3, CPOL/CPHA) and max speed in Hz. |
| `spi.transfer(dev, data)` | `bytes` | Clocks out `len(data)` bytes and returns the `len(data)` bytes clocked in at the same time. Empty in -> empty out. |
| `spi.close(dev)` | `null` | Closes the device. |

`transfer` is the `SPI_IOC_MESSAGE` ioctl (one full-duplex message). Blocking;
compose with `spawn`.

## See also

[serial](serial.md), [iic](iic.md), [gpio](gpio.md), [fs](fs.md).
