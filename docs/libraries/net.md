# `net` - TCP + UDP sockets and DNS lookups

Enable with `use net;`. Blocking TCP and UDP sockets plus two
DNS lookup helpers. Non-blocking use composes with M16.0
[`spawn`](../user-guide/concurrency.md) rather than a
duplicated `*Async` surface.

```jennifer
use io;
use net;
use convert;

def c as net.Conn init net.connect("example.com:80");
net.writeBytes($c, convert.bytesFromString(
    "GET / HTTP/1.0\r\nHost: example.com\r\n\r\n", "utf-8"));
def reply as bytes init net.readBytes($c, 4096);
net.close($c);
io.printf("%s\n", convert.stringFromBytes($reply, "utf-8"));
```

## TCP

TCP is stream-oriented: writes go into a byte stream on one
side; reads pull bytes off the other side in the order they
arrived.

| Call                             | Returns         | Notes                                                                                     |
| -------------------------------- | --------------- | ----------------------------------------------------------------------------------------- |
| `net.connect(address)`           | `net.Conn`      | TCP client. `address` is `"host:port"` (Go's convention).                                 |
| `net.listen(address)`            | `net.Listener`  | Bind and listen. `":0"` selects an ephemeral port.                                        |
| `net.accept($listener)`          | `net.Conn`      | Blocking accept. Non-blocking use pairs with `spawn`.                                     |
| `net.readBytes($conn, n)`        | `bytes`         | Blocks for at least one byte; returns whatever's available, capped at `n`. Sticky-EOF on close. |
| `net.writeBytes($conn, b)`       | `null`          | Blocking write of every byte.                                                             |
| `net.eof($conn)`                 | `bool`          | Looks ahead: true iff the next read would return partial or fail.                        |
| `net.address($conn)`             | `string`        | Peer's `"host:port"` (for logs). Polymorphic - see [Address helpers](#address-helpers).  |

### `net.Conn`

```jennifer
def struct net.Conn { id as int };
```

Handles share underlying state between copies via the integer
id (same discipline as M16.0's `task of T` and M16.1's
`fs.File`). `net.close($c)` closes the connection for every
copy of the handle.

### Canonical read loop

```jennifer
use io;
use net;
use convert;

def c as net.Conn init net.connect("some.host:1234");
while (not net.eof($c)) {
    def chunk as bytes init net.readBytes($c, 4096);
    io.printf("got %d bytes\n", len($chunk));
}
net.close($c);
```

`net.eof` peeks one byte through the buffered reader so the
loop terminates on the exact byte after the last real read.

### Server pattern

```jennifer
use net;
use task;

func handle(conn as net.Conn) {
    # ... read/write on $conn ...
    net.close($conn);
    return null;
}

def listener as net.Listener init net.listen(":8080");
while (true) {
    def conn as net.Conn init net.accept($listener);
    def worker as task of null init spawn { return handle($conn); };
    task.discard($worker);   # fire-and-forget per-connection worker
}
```

Under `jennifer-go` each `spawn` runs on its own OS thread and
scales across cores; under `jennifer` (TinyGo) the server
compiles but every net call surfaces the "not available"
message - see [TinyGo compatibility](#tinygo-compatibility)
below.

## UDP

UDP is datagram-oriented: each `sendTo` / `recvFrom` is one
packet with an associated peer address. There's no connection
to establish; the socket is a bound port.

| Call                             | Returns         | Notes                                                                                     |
| -------------------------------- | --------------- | ----------------------------------------------------------------------------------------- |
| `net.listenUDP(address)`         | `net.UDPSocket` | Bind a UDP port. Usable as both client and server (send from wherever you bound).         |
| `net.sendTo($sock, peer, bytes)` | `null`          | Send one datagram to `peer` (`"host:port"`).                                              |
| `net.recvFrom($sock, n)`         | `net.Datagram`  | Block for one datagram, up to `n` bytes.                                                  |

### Structs

```jennifer
def struct net.UDPSocket { id as int };

def struct net.Datagram {
    data as bytes,
    peer as string      # "host:port" of the sender
};
```

### Example: minimal client

```jennifer
use net;
use convert;

def s as net.UDPSocket init net.listenUDP(":0");
net.sendTo($s, "1.2.3.4:53", convert.bytesFromString("query", "utf-8"));
def reply as net.Datagram init net.recvFrom($s, 4096);
# $reply.data is the payload; $reply.peer is who sent it
net.close($s);
```

The bind-and-use pattern doubles as the client pattern: bind
to `:0` (kernel picks a port), send to the remote peer.

## DNS

Two lookup helpers; specialised record-type variants
(`net.lookupMX`, `net.lookupTXT`, ...) ship when needed.

| Call                             | Returns          | Notes                                                                     |
| -------------------------------- | ---------------- | ------------------------------------------------------------------------- |
| `net.lookup(host)`               | `list of string` | Resolve `host` to a list of IP addresses (v4 and v6 mixed).              |
| `net.reverseLookup(ip)`          | `list of string` | Reverse DNS: IP address to a list of hostnames.                           |

```jennifer
use io;
use net;

def ips as list of string init net.lookup("example.com");
for (def ip in $ips) {
    io.printf("%s\n", $ip);
}
```

DNS lookups are blocking. Compose with `spawn` to overlap
resolution with other work.

## Close

`net.close` is polymorphic - it accepts a `net.Conn`, a
`net.Listener`, or a `net.UDPSocket` and dispatches based on
the struct tag. One verb; three closable kinds; the boundary
check errors cleanly if the caller passes anything else.

```jennifer
net.close($conn);         # closes a TCP connection
net.close($listener);     # closes a TCP listener
net.close($socket);       # closes a UDP socket
```

Use-after-close on any of the three surfaces:
`net.readBytes: net.Conn id 3 is not open (already closed, or
never opened)`.

## Address helpers

`net.address` is polymorphic over the three handle kinds:

- `net.address($conn)` returns the peer's remote address
  ("who am I talking to?").
- `net.address($listener)` returns the local bound address
  ("what did the kernel pick for me?").
- `net.address($sock)` (UDP) returns the local bound address.

The listener form is the one you need after binding to
`":0"` to discover which ephemeral port you actually got.

```jennifer
def listener as net.Listener init net.listen(":0");
io.printf("bound to %s\n", net.address($listener));
```

## Address format

TCP and UDP addresses take the standard `"host:port"` form.
For IPv6 you must bracket the host: `"[::1]:8080"`. `host`
may be a hostname (resolved via the system's DNS at
connect/bind time) or a literal IP address; `:port` alone
binds all interfaces.

```
"example.com:80"         # v4 or v6, resolver decides
"127.0.0.1:8080"         # v4 loopback
"[::1]:8080"             # v6 loopback (brackets required)
":8080"                  # bind on all interfaces
":0"                     # bind to any free ephemeral port
```

## Concurrency composition (M16.0)

Blocking calls compose with `spawn` for non-blocking use:

```jennifer
use net;
use task;

def slow as task of net.Conn init spawn {
    return net.connect("some.slow.host:80");
};
# ... other work while the connect is in flight ...
def c as net.Conn init task.wait($slow);
```

The parallel-server pattern in [TCP > Server pattern](#server-pattern)
above is the workhorse case.

## Errors

Every error is positioned at the Jennifer call site with the
address or handle id in the message.

- **Missing host / unreachable peer**: `net.connect: nonexistent.invalid:9999: dial tcp: lookup nonexistent.invalid: no such host`.
- **Bind failure** (port in use, permission): `net.listen: :80: listen tcp :80: bind: permission denied`.
- **Use after close**: `net.readBytes: net.Conn id 3 is not open (already closed, or never opened)`.
- **Wrong type on polymorphic close**: `net.close: argument must be a net.Conn, net.Listener, or net.UDPSocket; got net.Datagram`.
- **DNS misconfig**: `net.lookup: whatever: lookup whatever: no such host`.
- **Peer address parse**: `net.sendTo: bogus: address bogus: missing port in address`.

Every error is catchable with M13.2 `try` / `catch`:

```jennifer
try {
    def c as net.Conn init net.connect("possibly-down.host:80");
} catch (e) {
    io.printf("connect failed: %s\n", $e.message);
}
```

## TinyGo compatibility

The shipping `jennifer` binary (TinyGo build) **does not
include a network stack**. TinyGo 0.41 compiles most of
`net.Dial` / `net.Listen` but requires a netdev driver to be
registered at runtime; UDP (`net.ListenPacket`) isn't part of
TinyGo's surface at all. Rather than let each call fail with
cryptic runtime errors from deep inside Go's `net` package,
every `net` entry point on the TinyGo binary returns a
friendly Jennifer-level error:

```
net.connect: the TinyGo build of `jennifer` does not include a
network stack; use the `jennifer-go` binary for network I/O
```

Same pattern as M15.3 `os.run` / `os.spawn` on TinyGo. If
you're writing network code, use `jennifer-go`. See
[../technical/tinygo.md](../technical/tinygo.md).

## What's not in v1

Recorded so the design decisions stay visible; ships if a
concrete workload forces it.

- **TLS.** Sub-milestone of its own; brings in certificate
  stores, ALPN, session tickets.
- **Unix domain sockets.**
- **Timeouts / deadlines.** Compose with `time.sleep`,
  `spawn`, `task.waitAny` for a first cut. A proper
  `net.setDeadline($conn, ms)` verb ships later.
- **Socket options** (SO_REUSEADDR, KEEPALIVE, NODELAY).
- **DNS record-type helpers** (`net.lookupMX`,
  `net.lookupTXT`, `net.lookupSRV`). The M16.2 pair covers
  90% of use.
- **Explicit IPv6 control.** Auto-selected by the resolver;
  users force by writing `"[::1]:port"` or `"127.0.0.1:port"`.

## See also

- [../user-guide/concurrency.md](../user-guide/concurrency.md) -
  the `spawn`-and-compose story `net` builds on.
- [`task`](task.md) - observe results from `spawn`ed network
  workers.
- [`fs`](fs.md) - the parallel design for filesystem I/O.
- [`convert`](convert.md) - `bytesFromString` / `stringFromBytes`
  bridge network payloads and Jennifer strings.
- [../technical/tinygo.md](../technical/tinygo.md) - the
  netdev-driver row in the restrictions table.
- [../milestones.md](../milestones.md) - M16.2 design spec.
