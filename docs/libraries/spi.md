# spi

SPI devices (`/dev/spidev0.0`, ...). Open a device with its mode, speed and
word size, then `transfer` bytes full-duplex (write and read happen together over one exchange).
**Linux-only**; a stub elsewhere and on `jennifer-tiny`.

```jennifer
use spi;

def dev as spi.Device init spi.openWith("/dev/spidev0.0", spi.Options{mode: 0, speedHz: 1000000});
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
| `spi.open(path)` | `spi.Device` | `spi.openWith(path, spi.Options{})`: mode 0, 500 kHz, 8 bits per word. |
| `spi.openWith(path, spi.Options)` | `spi.Device` | Opens the device with the options below. |
| `spi.transfer(dev, data)` | `bytes` | Clocks out `len(data)` bytes and returns the `len(data)` bytes clocked in at the same time, with chip select held for the whole exchange. Empty in -> empty out. More than spidev's buffer (default 4096 bytes) throws `Error{kind: "limit"}`. Above 8 bits per word a word takes 2 bytes (9-16 bits) or 4 (17-32), and `data` must be whole words. |
| `spi.close(dev)` | `null` | Closes the device. A second `close` does nothing; any other call on a closed device throws `Error{kind: "closed"}`. |
| `spi.configure(dev, mode, speedHz)` | `null` | **Deprecated**, removed in 0.29.0 (lint L301): open with `spi.openWith` instead. Sets mode and speed on an open device. |

## `spi.Options`

`spi.Options { mode as int, speedHz as int, bitsPerWord as int }`

| Field | Values | Zero means |
|---|---|---|
| `mode` | 0..3 (CPOL / CPHA) | mode 0 |
| `speedHz` | 1..4294967295, the maximum clock | 500000 |
| `bitsPerWord` | 1..32, as the controller supports | 8 |

A value out of range throws before the device is opened; one the controller
does not support throws from the open (`... (the controller does not support
this setting)`).

`spi.open(path)` followed by `spi.configure(dev, mode, speedHz)` is
`spi.openWith(path, spi.Options{mode: mode, speedHz: speedHz})`.

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
