# `ntp` - SNTP network-time client

Import with `import "ntp.j" as ntp;`. Query a time server over UDP and get back
its time plus the local clock **offset** and round-trip **delay**. A one-shot
**SNTP** client (RFC 4330 / 5905): it speaks the standard NTP wire protocol
(the 48-byte packet on port 123) but does not discipline the clock or run as a
daemon. It reports the measurement; acting on it is the caller's job. Needs the default `jennifer` binary (`net`).

```jennifer
import "ntp.j" as ntp;
use io; use time;

def r as ntp.Result init ntp.query("pool.ntp.org");
io.printf("server time: %s  offset: %d ms\n", time.iso($r.serverTime), time.milliseconds($r.offset));
```

Runnable: [`examples/modules/ntp_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/ntp_demo.j).

## Result

| Field | Type | |
| ----- | ---- | - |
| `serverTime` | `time.Time` | the server's transmit time |
| `offset` | `time.Duration` | local clock offset (server minus local) |
| `delay` | `time.Duration` | measured round-trip delay |

`offset` is computed from the four SNTP timestamps as
`((T2 - T1) + (T3 - T4)) / 2` and `delay` as `(T4 - T1) - (T3 - T2)`, where T1/T4
are the local send/receive instants and T2/T3 are the server's receive/transmit
timestamps. A positive `offset` means the local clock is behind the server.

## Query

| Call | Returns | |
| ---- | ------- | - |
| `ntp.query(host)` | `Result` | query `host:123` with a 5-second timeout |
| `ntp.queryWith(address, timeoutMs)` | `Result` | query a full `host:port` with a receive timeout in milliseconds |

The receive timeout is a deadline on the UDP socket, so a lost reply fails fast
instead of hanging.

| Failure | Thrown |
| ------- | ------ |
| no reply within the timeout | `Error`, generic kind `"timeout"` |
| an unsound reply, a Kiss-o'-Death, or 64 stray datagrams | `ntp.NtpError`, kind `"ntp"` |
| a socket failure (an unresolvable host) | `net.NetError`, kind `"net"` |
| the task was cancelled | kind `"cancelled"` |

`NtpError` carries `kind`, `message` and `kissCode`: a Kiss-o'-Death's
four-letter code (RFC 5905 7.4), `""` for any other failure. `RATE` means poll
less often; `DENY` or `RSTR` mean stop querying that server.

A reply is checked as RFC 4330 asks: its originate timestamp must echo the
request's transmit timestamp, the mode must be server or broadcast, a stratum-0
Kiss-o'-Death is an error naming its code (checked first, since ntpd sends it
with leap indicator 3), and the server must be synchronised (leap indicator not
3). A datagram that does not answer this request (shorter than 48 bytes, or a
different originate timestamp: a late reply to another query, a blind spoof) is
discarded and the query keeps waiting until its deadline; after 64 such
datagrams it fails with kind `"ntp"`. Timestamps past the February 2036 NTP
rollover decode correctly.

## Scope

- **Query, not discipline.** This measures the offset and hands it back; it does
  not step or slew the system clock (that is the OS / a daemon's job), and it
  does no server selection or filtering. A Kiss-o'-Death is reported, not
  retried; honour `RATE` by backing off.
- **One server per call.** Poll several servers and combine the results yourself
  to guard against a single bad source.
- **No authentication.** Plain SNTP; no symmetric-key or NTS-secured exchange.

## See also

- [net.md](../libraries/net.md) - the UDP surface (`listenUDP` / `sendTo` /
  `recvFrom` / `setReadDeadline`) the client is built on.
- [time.md](../libraries/time.md) - the `time.Time` / `time.Duration` the result
  is expressed in.
- [modules/index.md](index.md) - the module catalog and import rules.

## Academy talks

| Talk | What it shows |
| ---- | ------------- |
| [Network toolbox: SNMP, NTP, LDAP and IP addresses](https://academy.jennifer-lang.dev/talks/mplx/nettools/) | querying a server and warning on clock drift |

## API reference

Every exported function, struct and constant of `ntp`, with its signature, generated
from the module's doc comments: [the `ntp` API reference](../api/ntp.md).
