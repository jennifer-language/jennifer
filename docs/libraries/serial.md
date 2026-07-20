# serial

Blocking serial-port I/O (`/dev/ttyUSB0`, `/dev/ttyAMA0`, ...) with termios line
configuration that plain [`fs`](fs.md) cannot reach. **Linux-only** (a `/dev` +
ioctl feature); every entry point on a non-Linux host or the `jennifer-tiny`
build returns a friendly error pointing at the default `jennifer` binary on
Linux.

```jennifer
use serial;
use convert;

def port as serial.Port init serial.open("/dev/ttyUSB0", 115200);   # 8N1
serial.write($port, convert.bytesFromString("AT\r\n", "utf-8"));
def reply as bytes init serial.read($port, 64);                     # blocks for >=1 byte
serial.close($port);
```

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `serial.open(path, baud)` | `serial.Port` | Opens raw 8N1 at `baud` (a standard rate: 9600 / 19200 / 115200 / ...). A non-standard rate errors. |
| `serial.openWith(path, serial.Options)` | `serial.Port` | Full configuration - see `serial.Options` below. |
| `serial.read(port, n)` | `bytes` | Blocks until at least one byte arrives, then returns up to `n` bytes. |
| `serial.write(port, data)` | `int` | Writes `data`; returns the number of bytes written. |
| `serial.flush(port)` | `null` | Discards buffered input and output. |
| `serial.close(port)` | `null` | Closes the port; later ops on the handle error. |

### `serial.Options`

`serial.Options { baud as int, dataBits as int, parity as string, stopBits as int }`
- `baud` - a standard rate.
- `dataBits` - 5, 6, 7, or 8.
- `parity` - `"none"`, `"even"`, or `"odd"`.
- `stopBits` - 1 or 2.

Blocking on purpose - run a read loop in a `spawn` for concurrency (same stance as
[`fs`](fs.md) / [`net`](net.md)). The handle is the integer-registry pattern:
copies share the underlying port, so close it exactly once.

## See also

[fs](fs.md), [net](net.md), [spi](spi.md), [iic](iic.md), [gpio](gpio.md).
