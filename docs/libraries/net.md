# `net` - TCP + UDP sockets and DNS lookups

Enable with `use net;`. Blocking TCP and UDP sockets plus two
DNS lookup helpers. For non-blocking use, combine with
[`spawn`](../user-guide/concurrency.md); there is no `*Async` surface.

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

| Call                             | Returns         | Notes                                                                                     |
| -------------------------------- | --------------- | ----------------------------------------------------------------------------------------- |
| `net.connect(address[, timeoutMs])` | `net.Conn`   | TCP client. `address` is `"host:port"` (Go's convention). An optional `timeoutMs` bounds connection establishment (a slow / unreachable peer otherwise blocks the dial forever); `0` / omitted means no connect timeout. |
| `net.listen(address)`            | `net.Listener`  | Bind and listen. `":0"` selects an ephemeral port.                                        |
| `net.accept($listener)`          | `net.Conn`      | Blocking accept. Non-blocking use pairs with `spawn`.                                     |
| `net.readBytes($conn, n)`        | `bytes`         | Blocks for at least one byte; returns whatever's available, capped at `n`. Once the peer has closed it returns **empty** `bytes` (length 0), and keeps doing so (sticky EOF) - how a reader tells "peer closed" from "nothing yet". |
| `net.readAll($conn[, maxBytes[, idleTimeoutMs]])` | `bytes` | Read until EOF and return the whole remaining stream as one `bytes`. `maxBytes` omitted or `0` uses the default **256 MiB** cap (so a hostile endless stream is a catchable error, not an OOM); `> 0` sets an explicit cap; a **negative** value is the explicit "no limit" opt-in. `idleTimeoutMs > 0` re-arms a read deadline before each chunk (a timeout is a distinct catchable error). See [Whole-stream reads](#whole-stream-reads). |
| `net.readN($conn, n[, idleTimeoutMs])` | `bytes`   | Read **exactly** `n` bytes (a length-prefixed frame). A peer that closes before `n` bytes is a catchable "closed mid-frame" error, never a truncated return. `idleTimeoutMs` as in `readAll`. |
| `net.writeBytes($conn, b)`       | `null`          | Blocking write of every byte.                                                             |
| `net.setReadDeadline($conn, ms)`  | `null`         | Arm a **read** deadline `ms` out; `0` clears it. A read past it fails with a catchable `Error{kind: "timeout"}` (`... read timed out`); the write side is untouched. The poll-with-timeout primitive - see [Deadlines](#deadlines-single-threaded-poll-with-timeout). Accepts a `net.Conn` or a `net.UDPSocket`. |
| `net.setWriteDeadline($conn, ms)` | `null`         | Arm a **write** deadline `ms` out; `0` clears it. Bounds a slow / stalled send; reads are untouched. Accepts a `net.Conn` or a `net.UDPSocket`. |
| `net.eof($conn)`                 | `bool`          | Looks ahead: true iff the next read would return partial or fail.                        |
| `net.address($conn)`             | `string`        | Peer's `"host:port"` (for logs). Polymorphic - see [Address helpers](#address-helpers).  |

### `net.Conn`

```jennifer
def struct net.Conn { id as int };
```

Copies of a handle share the underlying connection via the integer
id (like `task of T` and `fs.File`); `net.close($c)` closes it for
every copy.

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

### Whole-stream reads

Accumulating a large body in a Jennifer loop pays the interpreter's per-step
cost. `net.readAll` and `net.readN` do the accumulation in a single Go loop
instead:

```jennifer
# The whole response body to EOF, in one call (an HTTP / object download):
def body as bytes init net.readAll($c, 0, 30000);   # default 256 MiB cap, 30s idle timeout

# Exactly n bytes of a length-prefixed frame (n came off the wire first):
def frame as bytes init net.readN($c, $n);
```

`net.readAll` reads until the peer closes (or `maxBytes` is hit); `net.readN`
reads a known count and treats an early close as an error rather than a short
read. Both grow one Go slice, so a megabyte-scale transfer runs at native
speed. To search or split the
result once it is in hand, use [`binary`](binary.md) (`indexOf` / `split` /
`slice`).

With `idleTimeoutMs > 0` both calls re-arm the connection's read deadline
before each chunk, and restore the `net.setReadDeadline` state (no deadline when
none was set) on return, so the idle timeout never leaks into a later read.

### Deadlines: single-threaded poll with timeout

`net.setReadDeadline($conn, ms)` arms a read deadline `ms` milliseconds in the
future; a read that reaches it before data arrives fails with a catchable
`Error{kind: "timeout"}` (message `net.readBytes: read timed out`).
`net.setReadDeadline($conn, 0)` clears it. This turns a blocking read into a
poll-with-timeout, so a protocol client can wait for a packet and do idle work
on a timeout without a `spawn`ed reader. The same call accepts a
`net.UDPSocket`, bounding a datagram `recvFrom` (an SNTP client that must not
hang on a lost reply).

A millisecond value large enough to overflow an internal duration ("effectively
never") is clamped to the largest representable deadline instead of wrapping, so
it behaves as unbounded. This applies to
`setReadDeadline` / `setWriteDeadline`, the `connect` / `connectTLS` `timeoutMs`,
and `readAll` / `readN`'s `idleTimeoutMs`.

A read deadline and a write deadline are **separate** (there is no combined
call): `net.setReadDeadline` bounds only the read, so a write after a read
timeout still succeeds and the keepalive or reply goes out. Use
`net.setWriteDeadline` to bound a send; arm both to bound both directions. (A
single combined deadline left expired by the poll idiom would make the next
write fail with `i/o timeout`.)

```jennifer
use net;
use strings;

# Wait up to 1s for a packet; on timeout, send a keepalive and retry.
def running as bool init true;
while ($running) {
    net.setReadDeadline($c, 1000);   # read-only: a timeout does not block the write below
    try {
        def head as bytes init net.readBytes($c, 1);
        net.setReadDeadline($c, 0);  # clear while we read the rest of the packet
        # ... read and dispatch the rest of the message ...
    } catch (err) {
        if ($err.kind == "timeout") {
            net.writeBytes($c, $keepalive);   # works: the read deadline never touched the write side
        } else {
            $running = false;                 # a real failure (closed conn, etc.)
        }
    }
}
```

Clear a deadline on every exit path from the read that armed it with
`defer net.setReadDeadline($conn, 0);` so it never leaks into a later read on
the same connection.

The deadline is absolute and is **not** rearmed automatically: reset it
(or clear it with `0`) before the next read.

**Every timeout has `kind: "timeout"`.** A read or write past its deadline, a
`connect` / `connectTLS` past its `timeoutMs`, and a `startTLS` handshake past its
timeout all raise `Error{kind: "timeout"}` with one message shape,
`<call>: <operation> timed out` (`net.recvFrom: read timed out`,
`net.connect: 10.0.0.1:80: connect timed out`), on TCP and UDP alike.

**A closed socket has `kind: "closed"`.** A call on a handle that was closed
(`net.Conn id 2 is not open ...`) and a call blocked on one that another task
closes (`net.accept: accept on a closed socket`) raise `Error{kind: "closed"}`.
Closing is how a program stops a task parked in `net.accept` or a read, which
`task.cancel` cannot reach inside the system call: close the listener or
connection from another task, and the parked call throws, so the task ends.

```jennifer
def acceptor as task of null init spawn {
    try {
        while (true) {
            serve(net.accept($listener));
        }
    } catch (err) {
        if ($err.kind != "closed") {
            throw $err;
        }
    }
    return;
};
# ... later, to shut down:
net.close($listener);
task.wait($acceptor);
```

Every other socket failure - a reset connection, a refused connect - is
`kind: "runtime"` carrying the OS text, so a loop can tell "nothing arrived yet"
(`timeout`), "we closed it" (`closed`) and "the network failed" (`runtime`) by
`kind` alone.

### Read errors are catchable - including inside `spawn`

Every read error (`read timed out`, `connection reset by peer`, a closed
peer) is an ordinary catchable runtime error. It unwinds through nested
function calls, `import` module boundaries, and `defer` / `errdefer` cleanup
into the nearest `try` that dynamically encloses the read.

The exception comes from the concurrency model: an error raised **inside a
`spawn` body** does not propagate to code around the `spawn`; it belongs to
the task. Observe it with
`task.wait` (which re-raises it at the wait site, catchable there) or
`task.discard`. A task that errors and is **never** observed is reported
at program exit by the loud-fail scan, which prints
`<prog>: unwaited spawn error: ...` and bumps the exit code (distinct from
a top-level uncaught error, `<prog>: runtime error at ...`). So if a
background reader task hits a dropped connection, wrap the
`task.wait` in `try` / `catch`; a `try` around the `spawn` **launch**
cannot catch what happens later inside the body.

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

Under the default `jennifer` binary each `spawn` runs on its own
OS thread and scales across cores; under `jennifer-tiny` (TinyGo)
the server compiles but every net call surfaces the "not
available" message - see
[TinyGo compatibility](#tinygo-compatibility) below.

## TLS

Both entry points yield the **same `net.Conn` handle** as plaintext TCP,
so `readBytes` / `writeBytes` / `close` / `address` work unchanged.

| Call                            | Returns    | Notes                                                                                          |
| ------------------------------- | ---------- | ---------------------------------------------------------------------------------------------- |
| `net.connectTLS(address[, net.TLSOptions][, timeoutMs])` | `net.Conn` | Dial `address` (`"host:port"`, same as `net.connect`) and complete a TLS handshake (implicit TLS: 465 / 993 / 995, HTTPS). Optional trailing `timeoutMs` bounds the dial + handshake. |
| `net.startTLS($conn[, net.TLSOptions][, timeoutMs])` | `net.Conn` | Upgrade an open plaintext connection to TLS in place (STARTTLS: 587 / 143 / 110); same handle. Optional trailing `timeoutMs` bounds the handshake. |

Both take an optional trailing `net.TLSOptions` argument. The certificate
is verified against the connection's host: for `connectTLS`, the host in
`address` (a missing port errors); for `startTLS`, the host the
connection was opened with via `net.connect`. TLS is built only into the default `jennifer` binary;
`jennifer-tiny` returns the friendly no-network stub error.

### `net.TLSOptions`

`net.TLSOptions { skipVerify as bool, caCert as bytes }`. Certificate
verification is **on by default** - the zero value verifies against the
system roots.

- **`caCert`** - a PEM certificate to trust, for a private CA or a
  self-signed server: the **secure** way to reach such a server. An
  invalid PEM is a positioned error.
- **`skipVerify: true`** - accept *any* certificate (self-signed,
  expired, wrong-host). Development and testing only; an explicit,
  greppable opt-out.

A struct literal names every field, so set both - or default-construct
and assign only what you need (the zero value is the secure default):

```jennifer
use net;
use fs;

# secure: trust a self-signed / private-CA certificate
def o as net.TLSOptions;                       # skipVerify false, caCert empty
$o.caCert = fs.readBytes("server-ca.pem");
def c as net.Conn init net.connectTLS("localhost:8443", $o);

# insecure (dev / testing): skip verification entirely
def x as net.TLSOptions;
$x.skipVerify = true;
def d as net.Conn init net.connectTLS("localhost:8443", $x);
```

### STARTTLS example

```jennifer
use net;
def c as net.Conn init net.connect("smtp.example.com:587");   # plaintext
# ... read the greeting, send EHLO, send STARTTLS, read the "220 ready" line ...
$c = net.startTLS($c);                                        # upgraded in place, same handle
# ... continue the session, now encrypted (host reused from connect) ...
```

## UDP

Each `sendTo` / `recvFrom` is one datagram with a peer address. There
is no connection; the socket is a bound port.

| Call                             | Returns         | Notes                                                                                     |
| -------------------------------- | --------------- | ----------------------------------------------------------------------------------------- |
| `net.listenUDP(address)`         | `net.UDPSocket` | Bind a UDP port. Usable as both client and server (send from wherever you bound).         |
| `net.sendTo($sock, peer, bytes)` | `null`          | Send one datagram to `peer` (`"host:port"`).                                              |
| `net.recvFrom($sock, n)`         | `net.Datagram`  | Block for one datagram, up to `n` bytes. A longer datagram comes back cut to its first `n` bytes with `truncated` set (the rest is gone: a datagram is read whole or not at all). |
| `net.setBroadcast($sock, on)`    | `null`          | Enable (`true`) / disable `SO_BROADCAST` so a `sendTo` may target a subnet or the limited broadcast address (`255.255.255.255`). Off by default; the kernel refuses a broadcast send without it (receiving a broadcast needs nothing). Linux/Unix only. |
| `net.bindToDevice($sock, iface)` | `null`          | Bind the socket to one network interface via `SO_BINDTODEVICE` (`""` clears the binding), so a `sendTo` leaves on that link and only datagrams arriving on it are received. A wildcard bind (`0.0.0.0:0`) otherwise leaves interface choice to the kernel routing table; this is the fix for a broadcast that must reach a device on a chosen NIC of a **multi-homed** host (e.g. a mactelnet bootstrap of a router with no IP yet). **Linux-only and needs `CAP_NET_RAW`** (typically root); it errors with `EPERM` when unprivileged. |

With one active interface `bindToDevice` is unnecessary. With several (wired +
wifi + a USB adapter), the kernel picks the egress link for a broadcast
regardless of intent, and a reply arriving on an unexpected NIC can look like a
silent timeout.

### Structs

```jennifer
def struct net.UDPSocket { id as int };

def struct net.Datagram {
    data as bytes,
    peer as string,     # "host:port" of the sender
    truncated as bool   # the datagram was longer than recvFrom's n; data holds its first n bytes
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

A client binds to `:0` (kernel picks a port) and sends to the remote peer.

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

`net.close` accepts a `net.Conn`, a `net.Listener`, or a
`net.UDPSocket` and dispatches on the struct tag; anything else
is an error.

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

Use the listener form after binding to `":0"` to discover the
ephemeral port.

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

## Concurrency composition

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

See also the parallel [server pattern](#server-pattern) above.

## Errors

Every error is positioned at the Jennifer call site with the
address or handle id in the message.

- **Missing host / unreachable peer**: `net.connect: nonexistent.invalid:9999: dial tcp: lookup nonexistent.invalid: no such host`.
- **Bind failure** (port in use, permission): `net.listen: :80: listen tcp :80: bind: permission denied`.
- **Use after close**: `net.readBytes: net.Conn id 3 is not open (already closed, or never opened)`.
- **Wrong type on polymorphic close**: `net.close: argument must be a net.Conn, net.Listener, or net.UDPSocket; got net.Datagram`.
- **DNS misconfig**: `net.lookup: whatever: lookup whatever: no such host`.
- **Peer address parse**: `net.sendTo: bogus: address bogus: missing port in address`.

Every error is catchable with `try` / `catch`:

```jennifer
try {
    def c as net.Conn init net.connect("possibly-down.host:80");
} catch (e) {
    io.printf("connect failed: %s\n", $e.message);
}
```

## TinyGo compatibility

The **stock** `jennifer-tiny` binary ships **without a network
stack**, so every `net` entry point on it returns a friendly
Jennifer-level error:

```
net.connect: `jennifer-tiny` (TinyGo build) does not include a
network stack; use the default `jennifer` binary for network I/O
```

This is a property of the **stock tiny build**, not a hard TinyGo
limitation. TinyGo compiles most of `net.Dial` / `net.Listen`; a
default target lacks a **netdev driver** registered at runtime (the
network device the `net` package dials through), and the stock build
registers none, so it compiles the `tinygo`-tagged stub. A
`jennifer-tiny` **rebuilt with a network stack** (a registered netdev
driver / a net-capable target, with the `tinygo` build tag dropped on
`net`) restores `net` and every net-backed module. "Use the default
`jennifer` binary" means "use a build that includes a network stack".
(UDP is the thinner spot:
`net.ListenPacket` isn't part of TinyGo's surface today, so a
rebuild covers TCP / DNS more readily than UDP.)

Same pattern as `os.run` / `os.spawn` on TinyGo. See
[../technical/tinygo.md](../technical/tinygo.md#net-on-tinygo-is-a-build-choice-not-a-hard-limit).

## What's left out

Each ships if a concrete workload needs it.

- **TLS extras: ALPN and session tickets.** Core TLS ships (see
  [TLS](#tls)); ALPN protocol negotiation and session-ticket resumption
  do not.
- **Unix domain sockets.**
- **More socket options** (SO_REUSEADDR, KEEPALIVE, NODELAY). Only
  `SO_BROADCAST` (`net.setBroadcast`) and `SO_BINDTODEVICE`
  (`net.bindToDevice`) ship, both for UDP.
- **DNS record-type helpers** (`net.lookupMX`,
  `net.lookupTXT`, `net.lookupSRV`). The current pair covers
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
- [../milestones.md](../milestones.md) - design spec.
