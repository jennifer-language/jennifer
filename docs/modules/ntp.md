# `ntp` - SNTP network-time client

Import with `import "ntp.j" as ntp;`. Query a time server over UDP and get back
its time plus the local clock **offset** and round-trip **delay**. This is the
one-shot query half of NTP - a simple **SNTP** client (RFC 4330 / 5905): it
speaks the standard NTP wire protocol (the 48-byte packet on port 123) but does
not discipline the clock or run as a daemon. It reports the measurement; acting
on it is the caller's job. Needs the default `jennifer` binary (`net`).

```jennifer
import "ntp.j" as ntp;
use io; use time;

def r as ntp.Result init ntp.query("pool.ntp.org");
io.printf("server time: %s  offset: %d ms\n", time.iso($r.serverTime), time.milliseconds($r.offset));
```

Runnable: [`examples/modules/ntp_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/ntp_demo.j).

## Result

```jennifer
def struct ntp.Result {
    serverTime as time.Time,     # the server's transmit time
    offset as time.Duration,     # local clock offset (server minus local)
    delay as time.Duration       # measured round-trip delay
};
```

`offset` is computed from the four SNTP timestamps as
`((T2 - T1) + (T3 - T4)) / 2` and `delay` as `(T4 - T1) - (T3 - T2)`, where T1/T4
are the local send/receive instants and T2/T3 are the server's receive/transmit
timestamps. A positive `offset` means the local clock is behind the server.

## Query

| Call | Returns | |
| ---- | ------- | - |
| `ntp.query(host)` | `Result` | query `host:123` with a 5-second timeout |
| `ntp.queryWith(address, timeoutMs)` | `Result` | query a full `host:port` with a custom timeout |

`query` is the common form (`ntp.query("time.example.com")`); `queryWith` takes a
full `host:port` address and an explicit receive timeout in milliseconds. A query
that gets no reply within the timeout, or a short / malformed packet, throws
`Error{kind: "ntp"}` - catch it with `try` / `catch`. The receive timeout is a
real deadline on the UDP socket, so a lost reply fails fast instead of hanging.

## Scope

- **Query, not discipline.** This measures the offset and hands it back; it does
  not step or slew the system clock (that is the OS / a daemon's job), and it
  does no server selection, filtering, or Kiss-o'-Death handling.
- **One server per call.** Poll several servers and combine the results yourself
  if you want robustness against a single bad source.
- **No authentication.** Plain SNTP; no symmetric-key or NTS-secured exchange.

## See also

- [net.md](../libraries/net.md) - the UDP surface (`listenUDP` / `sendTo` /
  `recvFrom` / `setReadDeadline`) the client is built on.
- [time.md](../libraries/time.md) - the `time.Time` / `time.Duration` the result
  is expressed in.
- [modules/index.md](index.md) - the module catalog and import rules.
