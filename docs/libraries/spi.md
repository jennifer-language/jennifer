# spi

SPI devices (`/dev/spidev0.0`, ...). Open a device, set mode / speed, then
`transfer` bytes full-duplex (write and read happen together over one exchange).
**Linux-only**; a stub elsewhere and on `jennifer-tiny`.

```jennifer
use spi;

def dev as spi.Device init spi.open("/dev/spidev0.0");
spi.configure($dev, 0, 1000000);                        # mode 0, 1 MHz
# JEDEC "read ID": the 0x9f command, then three dummy bytes that clock the
# reply in - rx[1..4] holds the manufacturer and device id.
def tx as bytes;
$tx[] = 0x9f;
$tx[] = 0x00;
$tx[] = 0x00;
$tx[] = 0x00;
def rx as bytes init spi.transfer($dev, $tx);
spi.close($dev);
```

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `spi.open(path)` | `spi.Device` | Opens the device (default mode 0, 500 kHz, 8 bits/word). |
| `spi.configure(dev, mode, speedHz)` | `null` | Sets clock mode (0..3, CPOL/CPHA) and max speed in Hz (1..4294967295). |
| `spi.transfer(dev, data)` | `bytes` | Clocks out `len(data)` bytes and returns the `len(data)` bytes clocked in at the same time, with chip select held for the whole exchange. Empty in -> empty out. More than spidev's buffer (default 4096 bytes) throws `Error{kind: "limit"}`. |
| `spi.close(dev)` | `null` | Closes the device. |

`transfer` is the `SPI_IOC_MESSAGE` ioctl (one full-duplex message). Blocking;
compose with `spawn`. Calls on one device run one at a time, and `close` waits
for a transfer in flight.

## Transfer size

spidev moves at most `bufsiz` bytes per message (its module parameter, default
4096; `spi.open` reads `/sys/module/spidev/parameters/bufsiz`). A longer
`transfer` is **not split**: splitting would release chip select between the
parts, which ends the command on most devices (a flash read, a display frame).
It throws `Error{kind: "limit"}` instead. Raise the buffer with
`spidev.bufsiz=65536` on the kernel command line (Raspberry Pi:
`/boot/firmware/cmdline.txt`) or `options spidev bufsiz=65536` in
`/etc/modprobe.d/`, or send the data in parts where the device allows chip
select to drop between them.

## See also

[serial](serial.md), [i2c](i2c.md), [gpio](gpio.md), [fs](fs.md).

## Academy talks

| Talk | What it shows |
| ---- | ------------- |
| [Hardware with Jennifer: GPIO, I2C, SPI and serial](https://academy.jennifer-lang.dev/talks/mplx/hardware/) | reading an MCP3008 analog-to-digital converter over SPI |
