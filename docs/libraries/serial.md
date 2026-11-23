# serial

Blocking serial-port I/O (`/dev/ttyUSB0`, `/dev/ttyAMA0`, ...) with termios line
configuration that plain [`fs`](fs.md) cannot reach. **Linux-only** (a `/dev` +
ioctl feature); every entry point on a non-Linux host or the `jennifer-tiny`
build raises an error pointing at the default `jennifer` binary on Linux.

```jennifer
use encoding;
use serial;

def port as serial.Port init serial.open("/dev/ttyUSB0", 115200);   # 8N1
serial.write($port, encoding.encode("utf-8", "AT\r\n"));
def reply as bytes init serial.read($port, 64);                     # blocks for >=1 byte
serial.close($port);
```

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `serial.open(path, baud)` | `serial.Port` | Opens raw 8N1 at `baud` (a standard rate: 9600 / 19200 / 115200 / ... / 4000000), no flow control, exclusive. A non-standard rate errors. |
| `serial.openWith(path, serial.Options)` | `serial.Port` | Full configuration - see `serial.Options` below. |
| `serial.read(port, n)` | `bytes` | Blocks until at least one byte arrives, then returns up to `n` bytes. Past a read deadline it throws `Error{kind: "timeout"}`; when another task closes the port, or the port hangs up (USB adapter unplugged, far end of a pty closed), `Error{kind: "closed"}`; `task.cancel` of the calling task wakes it with "task cancelled". An `n` over 256 MiB is `Error{kind: "limit"}`. |
| `serial.write(port, data)` | `int` | Writes `data`; returns the number of bytes written. Past a write deadline it throws `Error{kind: "timeout"}`; on a hung-up port, `Error{kind: "closed"}`. |
| `serial.setReadDeadline(port, ms)` | `null` | Arm a read deadline `ms` milliseconds from now; `0` clears it. A read past it fails with `Error{kind: "timeout"}` (`serial.read: read timed out`); the deadline stays until reset. The write side is untouched. |
| `serial.setWriteDeadline(port, ms)` | `null` | Arm a write deadline (a send held up by flow control); `0` clears it. Reads are untouched. |
| `serial.flush(port)` | `null` | Discards buffered input and output. |
| `serial.close(port)` | `null` | Closes the port; later ops on the handle error, and a `read` pending in another task throws. |

## `serial.Options`

`serial.Options { baud as int, dataBits as int, parity as string, stopBits as int, flowControl as string, shared as bool }`

| Field | Values |
| --- | --- |
| `baud` | A standard rate (50 ... 4000000) |
| `dataBits` | 5, 6, 7, or 8 |
| `parity` | `"none"`, `"even"`, or `"odd"`. With parity on, input parity checking is on too: a byte received with a parity error reads as `0x00` |
| `stopBits` | 1 or 2 |
| `flowControl` | `"none"`, `"rtscts"` (hardware, CRTSCTS), or `"xonxoff"` (software, DC1 / DC3 in both directions) |
| `shared` | `true` skips the exclusive claim (below) |

A field left out of the literal (or `0` / `""` / `false`) takes the default,
9600 8N1, no flow control, exclusive: `serial.Options{baud: 115200}` is 115200
8N1. A bad value errors before the device is opened.

## Line state

Both open calls put the port in raw mode and reset every setting a previous
program may have left behind (flow control, input parity checking, case and
CR / LF mapping, mark / space parity), so the line is exactly what was asked
for. The port is opened without waiting for carrier detect (CLOCAL).

The open is **exclusive** (`TIOCEXCL`): while the port is open, another open of
it (`screen`, a second program) fails with "device or resource busy" - root
bypasses this. `serial.close` gives the claim up; `shared: true` skips it.

Opening a tty raises **DTR and RTS**, and closing it drops them again (HUPCL,
the kernel default). Many boards with a USB serial bridge (Arduino Uno / Nano,
ESP32 dev kits) wire DTR or RTS to reset, so each open restarts the board: wait
for its boot (often 1-2 s) before the first write, and keep the port open
rather than reopening it per message.

Errors name the call and path (`serial.open: /dev/ttyUSB0: permission denied
(add the user to the group that owns the device, often dialout or uucp)`).

Blocking; run a read loop in a `spawn` for concurrency (as with
[`fs`](fs.md) / [`net`](net.md)), or poll with a read deadline, the same way as
`net.setReadDeadline`:

```jennifer
serial.setReadDeadline($port, 500);
try {
    def reply as bytes init serial.read($port, 64);
} catch (e) {
    if ($e.kind != "timeout") { throw $e; }
    # nothing within 500 ms: retry, send a keepalive, give up ...
}
serial.setReadDeadline($port, 0);
```

The handle is the integer-registry pattern: copies share the underlying port, so
close it exactly once.

## See also

[fs](fs.md), [net](net.md), [spi](spi.md), [i2c](i2c.md), [gpio](gpio.md).

## Academy talks

| Talk | What it shows |
| ---- | ------------- |
| [Hardware with Jennifer: GPIO, I2C, SPI and serial](https://academy.jennifer-lang.dev/talks/mplx/hardware/) | a line protocol to a microcontroller with read timeouts |
