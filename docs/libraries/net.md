# `net` - TCP + UDP sockets and DNS lookups

Enable with `use net;`. Blocking TCP and UDP sockets plus DNS
lookups. For non-blocking use, combine with
[`spawn`](../user-guide/concurrency.md); there is no `*Async` surface.

```jennifer
use encoding;
use io;
use net;

def c as net.Conn init net.connect("example.com:80");
net.writeBytes($c, encoding.encode(
    "utf-8", "GET / HTTP/1.0\r\nHost: example.com\r\n\r\n"));
def reply as bytes init net.readBytes($c, 4096);
net.close($c);
io.printf("%s\n", encoding.decode("utf-8", $reply));
```

## TCP

| Call                             | Returns         | Notes                                                                                     |
| -------------------------------- | --------------- | ----------------------------------------------------------------------------------------- |
| `net.connect(address)`           | `net.Conn`      | TCP client. `address` is `"host:port"` (Go's convention). The connect gives up after the default 30 s (kind `"timeout"`). Same as `connectWith(address, net.ConnectOptions{})`. |
| `net.connectWith(address, net.ConnectOptions)` | `net.Conn` | Connect with options: a deadline other than the default, and TLS on connect. See [Connect options](#connect-options). |
| `net.listen(address)`            | `net.Listener`  | Bind and listen. `":0"` selects an ephemeral port.                                        |
| `net.accept($listener)`          | `net.Conn`      | Blocking accept. Non-blocking use pairs with `spawn`.                                     |
| `net.readBytes($conn, n)`        | `bytes`         | Blocks for at least one byte; returns whatever's available, capped at `n`. Once the peer has closed it returns **empty** `bytes` (length 0), and keeps doing so (sticky EOF) - how a reader tells "peer closed" from "nothing yet". |
| `net.readAll($conn[, maxBytes])` | `bytes`         | Read until EOF and return the whole remaining stream as one `bytes`. `maxBytes` omitted or `0` uses the default **256 MiB** cap (so a hostile endless stream is a catchable error, not an OOM); `> 0` sets an explicit cap (a stream past it is an error of kind `"limit"`); a **negative** value is the explicit "no limit" opt-in. See [Whole-stream reads](#whole-stream-reads). |
| `net.readN($conn, n)`            | `bytes`         | Read **exactly** `n` bytes (a length-prefixed frame). A peer that closes before `n` bytes is a catchable error of kind `"closed"`, never a truncated return. `n` above 256 MiB is an error of kind `"limit"`; the buffer grows as bytes arrive, so a large `n` costs nothing up front. In a spawned task, `task.cancel` ends a blocked `readBytes` / `readN` / `readAll` (and UDP `recvFrom`) at once with "task cancelled". |
| `net.writeBytes($conn, b)`       | `null`          | Blocking write of every byte.                                                             |
| `net.setReadDeadline($conn, ms)`  | `null`         | Arm a **read** deadline `ms` out; `0` clears it. A read past it fails with a catchable `Error{kind: "timeout"}` (`... read timed out`); the write side is untouched. The poll-with-timeout primitive - see [Deadlines](#deadlines-single-threaded-poll-with-timeout). Accepts a `net.Conn` or a `net.UDPSocket`. |
| `net.setIdleTimeout($conn, ms)`  | `null`         | While set, every read (`readBytes`, `readAll`, `readN`) throws `Error{kind: "timeout"}` when no byte arrives for `ms`; re-armed per underlying read, so a slow but steady transfer is never cut. `0` or `math.MAX_INT` clears it; negative throws `"runtime"`. `net.Conn` only. See [Idle timeout](#idle-timeout). |
| `net.setWriteDeadline($conn, ms)` | `null`         | Arm a **write** deadline `ms` out; `0` clears it. Bounds a slow / stalled send; reads are untouched. Accepts a `net.Conn` or a `net.UDPSocket`. |
| `net.abort($conn)`               | `null`          | Close with a TCP reset (RST) instead of an orderly close; the peer sees a `NetError` of reason `"reset"`. For tests and fakes - see [`net.abort`](#netabort). |
| `net.address($conn)`             | `string`        | Peer's `"host:port"` (for logs). Polymorphic - see [Address helpers](#address-helpers).  |

### `net.Conn`

```text
def struct net.Conn { id as int };
```

Copies of a handle share the underlying connection via the integer
id (like `task of T` and `fs.File`); `net.close($c)` closes it for
every copy. A second `net.close` (or `net.abort`) of a closed handle does
nothing, so a `defer net.close($c);` may run after an explicit close.

### Canonical read loop

```jennifer
use io;
use net;
use convert;

def c as net.Conn init net.connect("some.host:1234");
def chunk as bytes init net.readBytes($c, 4096);
while (len($chunk) > 0) {
    io.printf("got %d bytes\n", len($chunk));
    $chunk = net.readBytes($c, 4096);
}
net.close($c);
```

`readBytes` returning empty bytes means the peer closed: the definitive end.

### Whole-stream reads

Accumulating a large body in a Jennifer loop pays the interpreter's per-step
cost. `net.readAll` and `net.readN` do the accumulation in a single Go loop
instead:

```jennifer
# The whole response body to EOF, in one call (an HTTP / object download):
net.setIdleTimeout($c, 30000);                   # fail on a 30 s stall
def body as bytes init net.readAll($c);           # default 256 MiB cap

# Exactly n bytes of a length-prefixed frame (n came off the wire first):
def frame as bytes init net.readN($c, $n);
```

`net.readAll` reads until the peer closes (or `maxBytes` is hit); `net.readN`
reads a known count and treats an early close as an error rather than a short
read. Both grow one Go slice, so a megabyte-scale transfer runs at native
speed. To search or split the result, use [`binary`](binary.md) (`indexOf` /
`split`) and slices (`$b[a..b]`).

Both read under the connection's idle timeout and read deadline (see
[Idle timeout](#idle-timeout)).

### Idle timeout

`net.setIdleTimeout($conn, ms)` bounds a stall: before each underlying read,
the read deadline is re-armed to `ms` from now, so a read throws
`Error{kind: "timeout"}` (`net.readN: read timed out`) once no byte has
arrived for `ms`, and a long transfer that keeps moving runs to the end. It
stays set for every later read until changed; `0` or `math.MAX_INT` clears it.
It applies to `readBytes`, `readAll` and `readN` on a `net.Conn`; a UDP
`recvFrom` reads one datagram, which `setReadDeadline` already bounds.

| Set | Bounds | Fires |
| --- | --- | --- |
| `setIdleTimeout($c, ms)` | a stall: `ms` without a byte | per underlying read |
| `setReadDeadline($c, ms)` | the whole exchange: `ms` from the call | once, at a fixed time |
| both | each read by the earlier of the two | whichever passes first |

After a read the deadline is the one `setReadDeadline` armed (none when
cleared), so the idle re-arm never leaks into the fixed deadline.

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
it behaves as unbounded. This applies to `setReadDeadline` / `setWriteDeadline`,
`setIdleTimeout` and `ConnectOptions.timeoutMs`.

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
connect, TLS handshake or lookup past its `timeoutMs` (or the default) all
raise `Error{kind: "timeout"}` with one message shape,
`<call>: <operation> timed out` (`net.recvFrom: read timed out`,
`net.connect: 10.0.0.1:80: connect timed out`), on TCP and UDP alike.

**A closed socket has `kind: "closed"`.** A call other than `close` / `abort`
on a handle that was closed (`net.Conn id 2 is not open ...`) and a call blocked on one that another task
closes (`net.accept: accept on a closed socket`) raise `Error{kind: "closed"}`.
Closing is how a program stops a task parked in `net.accept`, which
`task.cancel` cannot reach inside the system call (it does wake a blocked
`readBytes` / `readN` / `readAll` / `recvFrom`): close the listener from another
task, and the parked call throws, so the task ends.

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

Every other socket failure - a reset connection, a refused connect - is a
`net.NetError` (`kind: "net"`) carrying the OS text, so a loop can tell "nothing
arrived yet" (`timeout`), "we closed it" (`closed`) and "the network failed"
(`net`) by `kind` alone; see [Errors](#errors).

### Connect options

`net.connectWith(address, net.ConnectOptions)`, `net.startTLSWith($conn,
net.ConnectOptions)`, `net.lookupWith(host, net.ConnectOptions)` and
`net.reverseLookupWith(ip, net.ConnectOptions)` take one options struct; its
zero value is the plain call (`connect`, `startTLS`, `lookup`,
`reverseLookup`).

```text
def struct net.ConnectOptions {
    timeoutMs as int,           # bound on the connect, the TLS handshake or the lookup
    tls as bool,                # connectWith: TLS handshake on connect (as connectTLS)
    tlsOptions as net.TLSOptions,
    dnsServers as list of string # lookupWith: ask these DNS servers only
};
```

| Field | Zero value | Read by | Notes |
| --- | --- | --- | --- |
| `timeoutMs` | the default, 30000 (30 s) | all four | `math.MAX_INT` is no deadline; a negative value throws `"runtime"`. A deadline past is kind `"timeout"`. |
| `tls` | `false`: plaintext | `connectWith` | `startTLSWith` ignores it (the call is the request for TLS); the lookups ignore it. |
| `tlsOptions` | verify against the system roots | `connectWith` with `tls`, `startTLSWith` | See [`net.TLSOptions`](#nettlsoptions). |
| `dnsServers` | `[]`: the [configured resolver](#resolver-configuration) (hosts file, then the resolv.conf nameservers, with the search list) | `lookupWith` | Each entry is `"ip"` or `"ip:port"` (port 53 by default); at most 16. `lookupWith` asks only these, each in turn (UDP, TCP when the answer is truncated, at most 5 s each, all within `timeoutMs`): no hosts file, no search list, the IPv4 addresses then the IPv6 ones. A host name instead of an address, or more than 16 entries, throws `"runtime"` in every call that takes the options. `connectWith`, `startTLSWith` and `reverseLookupWith` ignore it and resolve through the configured resolver. |

The deadline bounds the connect together with its TLS handshake, a
`startTLS` handshake, or a lookup; it never carries over to the reads and
writes on the connection, which `setReadDeadline` / `setWriteDeadline` /
`setIdleTimeout` bound. `startTLS` restores the deadlines armed before it.

```jennifer
use fs;
use math;
use net;

# No deadline at all: math.MAX_INT.
def slow as net.Conn init net.connectWith("tape.internal:9000", net.ConnectOptions{timeoutMs: math.MAX_INT});

# A 5 s connect + handshake to a server with a private CA:
def o as net.ConnectOptions init net.ConnectOptions{timeoutMs: 5000, tls: true};
$o.tlsOptions.caCert = fs.readBytes("server-ca.pem");
def c as net.Conn init net.connectWith("db.internal:5433", $o);
```

### Read errors are catchable - including inside `spawn`

Every read error (`read timed out`, `connection reset by peer`, a closed
peer) is an ordinary catchable error (kind `"timeout"`, `"net"`,
`"closed"`; see [Errors](#errors)). It unwinds through nested
function calls, `import` module boundaries, and `defer` / `errdefer` cleanup
into the nearest `try` that dynamically encloses the read.

The exception comes from the concurrency model: an error raised **inside a
`spawn` body** does not propagate to code around the `spawn`; it belongs to
the task. Observe it with `task.wait` (which re-raises it at the wait site,
catchable there) or `task.discard`. A task that errors and is **never** observed is reported
at program exit by the loud-fail scan, which prints
`<prog>: unwaited spawn error: ...` and bumps the exit code (distinct from
a top-level uncaught error, `<prog>: runtime error at ...`). If a
background reader task can hit a dropped connection, wrap the
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
the server compiles but every net call raises the "not available"
error - see [TinyGo compatibility](#tinygo-compatibility) below.

## TLS

Both entry points yield the **same `net.Conn` handle** as plaintext TCP,
so `readBytes` / `writeBytes` / `close` / `address` work unchanged.

| Call                            | Returns    | Notes                                                                                          |
| ------------------------------- | ---------- | ---------------------------------------------------------------------------------------------- |
| `net.connectTLS(address[, net.TLSOptions])` | `net.Conn` | Dial `address` (`"host:port"`, same as `net.connect`) and complete a TLS handshake (implicit TLS: 465 / 993 / 995, HTTPS), both within the default 30 s. Same as `connectWith(address, net.ConnectOptions{tls: true, tlsOptions: ...})`. |
| `net.startTLS($conn[, net.TLSOptions])` | `net.Conn` | Upgrade an open plaintext connection to TLS in place (STARTTLS: 587 / 143 / 110); same handle. The handshake gives up after the default 30 s. |
| `net.startTLSWith($conn, net.ConnectOptions)` | `net.Conn` | `startTLS` with the options' `timeoutMs` and `tlsOptions`. |

`connectTLS` and `startTLS` take an optional trailing `net.TLSOptions` argument. The certificate
is verified against the connection's host: for `connectTLS`, the host in
`address` (a missing port errors); for `startTLS`, the host the
connection was opened with via `net.connect`. TLS is built only into the
default `jennifer` binary; `jennifer-tiny` raises the no-network error.

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

A struct literal names every field, so set both, or default-construct
and assign only the field needed (the zero value is the secure default):

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
| `net.listenUDP(address)`         | `net.UDPSocket` | Bind a UDP port. Usable as both client and server.                                        |
| `net.sendTo($sock, peer, bytes)` | `null`          | Send one datagram to `peer` (`"host:port"`).                                              |
| `net.recvFrom($sock, n)`         | `net.Datagram`  | Block for one datagram, up to `n` bytes; in a spawned task `task.cancel` wakes it with "task cancelled". A longer datagram comes back cut to its first `n` bytes with `truncated` set (the rest is gone: a datagram is read whole or not at all). |
| `net.setBroadcast($sock, on)`    | `null`          | Enable (`true`) / disable `SO_BROADCAST` so a `sendTo` may target a subnet or the limited broadcast address (`255.255.255.255`). Off by default; the kernel refuses a broadcast send without it (receiving a broadcast needs nothing). Linux/Unix only. |
| `net.bindToDevice($sock, iface)` | `null`          | Bind the socket to one network interface via `SO_BINDTODEVICE` (`""` clears the binding), so a `sendTo` leaves on that link and only datagrams arriving on it are received. A wildcard bind (`0.0.0.0:0`) otherwise leaves interface choice to the kernel routing table; this is the fix for a broadcast that must reach a device on a chosen NIC of a **multi-homed** host (e.g. a `@jennifer/mactelnet` bootstrap of a router with no IP yet). **Linux only.** Unprivileged on Linux 5.7 and later for a socket not yet bound to a device; changing that binding, or any call on an older kernel, needs `CAP_NET_RAW` and fails with `EPERM`. |

With one active interface `bindToDevice` is unnecessary. With several (wired +
wifi + a USB adapter), the kernel picks the egress link for a broadcast
regardless of intent, and a reply arriving on an unexpected NIC can look like a
silent timeout.

### Structs

```text
def struct net.UDPSocket { id as int };

def struct net.Datagram {
    data as bytes,
    peer as string,     # "host:port" of the sender
    truncated as bool   # the datagram was longer than recvFrom's n; data holds its first n bytes
};
```

### Example: minimal client

```jennifer
use encoding;
use net;

def s as net.UDPSocket init net.listenUDP(":0");
net.sendTo($s, "1.2.3.4:53", encoding.encode("utf-8", "query"));
def reply as net.Datagram init net.recvFrom($s, 4096);
# $reply.data is the payload; $reply.peer is who sent it
net.close($s);
```

A client binds to `:0` (kernel picks a port) and sends to the remote peer.

## DNS

| Call                             | Returns          | Notes                                                                     |
| -------------------------------- | ---------------- | ------------------------------------------------------------------------- |
| `net.lookup(host)`               | `list of string` | Resolve `host` to a list of IP addresses (v4 and v6 mixed), within the default 30 s. |
| `net.lookupWith(host, net.ConnectOptions)` | `list of string` | `lookup` within the options' `timeoutMs`; with `dnsServers`, ask those DNS servers only (no hosts file, no search list, the IPv4 addresses then the IPv6 ones). `tls` and `tlsOptions` are ignored. |
| `net.reverseLookup(ip)`          | `list of string` | Reverse DNS: IP address to a list of hostnames, within the default 30 s. |
| `net.reverseLookupWith(ip, net.ConnectOptions)` | `list of string` | `reverseLookup` within the options' `timeoutMs`, through the configured resolver; the other fields are ignored. |

```jennifer
use io;
use net;

def ips as list of string init net.lookup("example.com");
for (def ip in $ips) {
    io.printf("%s\n", $ip);
}
```

DNS lookups block until the answer or the deadline (kind `"timeout"`).
Compose with `spawn` to overlap resolution with other work.

`dnsServers` asks named servers only, for a diagnostic or a split-horizon
name (see [Connect options](#connect-options)):

```jennifer
use io;
use net;

# What does this server answer?
def ips as list of string init net.lookupWith("intranet.example",
    net.ConnectOptions{dnsServers: ["192.0.2.53", "[2001:db8::53]:5353"], timeoutMs: 2000});
io.printf("%v\n", $ips);
```

### Resolver configuration

The default `jennifer` binary is built without cgo, so it resolves names
itself, from a resolv.conf (the nameservers) and a hosts file. Each file is
the first of:

| Order | resolv.conf | hosts |
| --- | --- | --- |
| 1 | `$JENNIFER_RESOLV_CONF` | `$JENNIFER_HOSTS` |
| 2 | `<PREFIX>/etc/resolv.conf` | `<PREFIX>/etc/hosts` |
| 3 | `/etc/resolv.conf` | `/etc/hosts` |

`<PREFIX>` is the install prefix the binary was built for (`make
PREFIX=<dir>`); the default `/usr` keeps its configuration in `/etc`, so a
default build reads exactly the system files. `jennifer version -v` shows the
files in effect and the ones looked for.

- A variable that names a missing file fails every lookup with a `NetError`
  of reason `"dns"` naming the variable; it is never skipped.
- With no resolv.conf at all, lookups go to the loopback default
  (`127.0.0.1:53`, `[::1]:53`), as Go's resolver does; the interpreter never
  falls back to a public resolver. A failed lookup's message then names the
  files looked for and `JENNIFER_RESOLV_CONF`.
- From a file other than the system's only the `nameserver` lines and the
  hosts entries are used, and every lookup in the process follows them
  (`net`, `http` and the other network modules, `sql`'s drivers). The search
  list, `ndots`, `timeout` and `attempts` still come from `/etc/resolv.conf`
  (or Go's defaults without it), and `/etc/hosts`, when it exists, is read
  before `$JENNIFER_HOSTS`.
- `lookupWith` with `dnsServers` reads none of these files.
- On Windows only the variables apply; the adapters' DNS settings are the
  system configuration. `jennifer-tiny` has no network library and reads none
  of the files.

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

### `net.abort`

`net.abort(conn)` closes a TCP connection with a reset instead of the
orderly close: it sets `SO_LINGER` to 0, so the kernel drops any unsent
data and answers with RST, and the peer's next read or write fails with
a `NetError` of reason `"reset"` (after `net.close` it reads end of
stream). It is how a test or a fake server produces what a crashed or
overloaded peer does. A TLS connection is aborted at its TCP layer,
without a `close_notify`. The handle is retired as by `net.close`; a
handle already closed is kind `"closed"`. On `jennifer-tiny` it throws
the same no-network-stack error as every other `net` call.

```jennifer
def c as net.Conn init net.accept($listener);
net.readBytes($c, 4096);   # take the request ...
net.abort($c);             # ... and reset the connection without answering
```

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
An IPv6 host is bracketed: `"[::1]:8080"`. `host`
may be a hostname (resolved via the system's DNS at
connect/bind time) or a literal IP address; `:port` alone
binds all interfaces.

```text
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
address or handle id in the message. A handler dispatches on the kind:

| Kind | When |
| --- | --- |
| `"timeout"` | A deadline (`setReadDeadline` / `setWriteDeadline`, the idle timeout, the connect, handshake or lookup deadline) passed: `net.readBytes: read timed out` |
| `"closed"` | The handle was closed before or under the call, or the peer closed in the middle of a `readN` |
| `"limit"` | A cap was reached: `readAll`'s `maxBytes`, a per-call count over 256 MiB (`readBytes` / `readN` / `recvFrom`), or 4096 open connections, listeners or UDP sockets (each kind counted on its own) |
| `"cancelled"` | `task.cancel` woke a read blocked in a spawned task |
| `"net"` | The network failed; always a `net.NetError` (below) |
| `"runtime"` | A bad argument (a negative `timeoutMs` too), a malformed address (`missing port in address`), or an option the socket does not support |

```text
def struct net.NetError {
    kind as string,     # "net"
    message as string,  # the OS text, as below
    op as string,       # "connect", "handshake", "read", "write", "accept", "listen", "lookup"
    reason as string,   # "refused", "reset", "unreachable", "dns", "tls", "closed", "other"
    sent as bool,       # the whole request was written (set by http; false from net)
    replied as bool     # some of the answer arrived (set by http; false from net)
};
```

`reason` is `"refused"` (nothing listens), `"reset"` (the peer reset the
connection, also a broken pipe or an aborted connection), `"unreachable"`
(no route to the network or host), `"dns"` (the name does not resolve),
`"tls"` (a certificate that does not verify, a TLS protocol error, or
anything else that fails a handshake), `"closed"` (only from a protocol
module: the peer closed before any byte of its reply) or `"other"`.
`connectTLS` reports `op` `"connect"` when the TCP dial failed and
`"handshake"` when the TLS handshake did. `"net"` is a generic kind: a
module that rewraps the errors it catches passes it on unchanged (see the
[control-flow guide](../user-guide/control-flow.md#error-kinds)). On
Windows the Winsock error numbers map to the same reasons.

| Case | Message |
| --- | --- |
| Missing host / unreachable peer | `net.connect: nonexistent.invalid:9999: dial tcp: lookup nonexistent.invalid: no such host` |
| Bind failure (port in use, permission) | `net.listen: :80: listen tcp :80: bind: permission denied` |
| Use after close | `net.readBytes: net.Conn id 3 is not open (already closed, or never opened)` |
| Wrong type on polymorphic close | `net.close: argument must be a net.Conn, net.Listener, or net.UDPSocket; got net.Datagram` |
| DNS misconfig | `net.lookup: whatever: lookup whatever: no such host` |
| No resolver configuration | `net.lookup: example.com: lookup example.com on 127.0.0.1:53: ...: connection refused (no resolver configuration: JENNIFER_RESOLV_CONF is not set and /etc/resolv.conf does not exist, so lookups go to the loopback default 127.0.0.1:53 / [::1]:53; set JENNIFER_RESOLV_CONF to a resolv.conf naming your DNS servers)` |
| Peer address parse | `net.sendTo: bogus: address bogus: missing port in address` |

Every error is catchable with `try` / `catch`:

```jennifer
try {
    def c as net.Conn init net.connect("possibly-down.host:80");
} catch (e) {
    if ($e.kind != "net") {
        throw $e;
    }
    def ne as net.NetError init $e;
    io.printf("connect failed (%s): %s\n", $ne.reason, $ne.message);
}
```

## Deprecated

Each still works and is removed in 0.29.0; `jennifer lint` reports a call
(L301).

| Deprecated | Use instead |
| --- | --- |
| `net.connect(address, timeoutMs)` | `net.connectWith(address, net.ConnectOptions{timeoutMs: ...})` |
| `net.connectTLS(address[, net.TLSOptions], timeoutMs)` | `net.connectWith(address, net.ConnectOptions{timeoutMs: ..., tls: true, tlsOptions: ...})` |
| `net.startTLS($conn[, net.TLSOptions], timeoutMs)` | `net.startTLSWith($conn, net.ConnectOptions{timeoutMs: ..., tlsOptions: ...})` |
| `net.readAll($conn, maxBytes, idleTimeoutMs)` | `net.setIdleTimeout($conn, ms)`, then `net.readAll($conn, maxBytes)` |
| `net.readN($conn, n, idleTimeoutMs)` | `net.setIdleTimeout($conn, ms)`, then `net.readN($conn, n)` |
| `net.eof($conn)` | `net.readBytes` returning empty `bytes`: the peer closed |

The deprecated `timeoutMs` follows the `ConnectOptions` rule (0 is the
default, `math.MAX_INT` none, negative throws). The deprecated
`idleTimeoutMs` is an idle timeout for that call only, taking the place of
`setIdleTimeout`'s. `net.eof` probes with a
20 ms read deadline: `true` once the peer has closed and everything it sent
has been read, `false` on an idle open connection or while another task is
blocked reading it.

## TinyGo compatibility

The **stock** `jennifer-tiny` binary ships **without a network
stack**, so every `net` entry point on it raises an error:

```text
net.connect: `jennifer-tiny` (TinyGo build) does not include a
network stack; use the default `jennifer` binary for network I/O
```

This is a property of the **stock tiny build**, not a hard TinyGo
limitation. TinyGo compiles most of `net.Dial` / `net.Listen`, but a
default target lacks a **netdev driver** registered at runtime (the
network device the `net` package dials through); the stock build
registers none, so it compiles the `tinygo`-tagged stub. A
`jennifer-tiny` **rebuilt with a network stack** (a registered netdev
driver or a net-capable target, with the `tinygo` build tag dropped on
`net`) restores `net` and every net-backed module. UDP is the thinner
spot: `net.ListenPacket` is not part of TinyGo's surface, so a rebuild
covers TCP / DNS more readily than UDP.

Same pattern as `os.run` / `os.spawn` on TinyGo. See
[../technical/tinygo.md](../technical/tinygo.md#net-on-tinygo-is-a-build-choice-not-a-hard-limit).

## What's left out

Each ships if a concrete workload needs it.

- **TLS extras: ALPN and session tickets.** Core TLS ships (see
  [TLS](#tls)); ALPN protocol negotiation and session-ticket resumption
  do not.
- **Unix domain sockets** and file-descriptor passing: planned as M30.15.
  `httpd` already listens on a `unix:` address.
- **More socket options** (SO_REUSEADDR, KEEPALIVE, NODELAY). Only
  `SO_BROADCAST` (`net.setBroadcast`) and `SO_BINDTODEVICE`
  (`net.bindToDevice`) ship, both for UDP.
- **DNS record-type helpers** (`net.lookupMX`, `net.lookupTXT`,
  `net.lookupSRV`). `lookupWith`'s `dnsServers` asks for A and AAAA records
  only.
- **Explicit IPv6 control.** The resolver picks the family; force one
  by writing `"[::1]:port"` or `"127.0.0.1:port"`.

## See also

- [../user-guide/concurrency.md](../user-guide/concurrency.md) -
  the `spawn`-and-compose story `net` builds on.
- [`task`](task.md) - observe results from `spawn`ed network
  workers.
- [`fs`](fs.md) - the parallel design for filesystem I/O.
- [`encoding`](encoding.md) - `encode` / `decode` bridge network payloads
  and Jennifer strings.
- [../technical/tinygo.md](../technical/tinygo.md) - the
  netdev-driver row in the restrictions table.
- [../milestones.md](../milestones.md) - design spec.

## Academy talks

| Talk | What it shows |
| ---- | ------------- |
| [Errors done right in Jennifer](https://academy.jennifer-lang.dev/talks/mplx/errors/) | retry decisions from the fields of a net error |
| [Jennifer facing hostile input](https://academy.jennifer-lang.dev/talks/mplx/security/) | peers that stall or drip, read timeouts and byte caps |
