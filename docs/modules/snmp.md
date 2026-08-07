# `snmp` - SNMP v1 / v2c client

Import with `import "snmp.j" as snmp;`. An SNMP manager (client) that queries an
agent over UDP: `get`, `getNext`, `set`, and subtree `walk`, with community-string
authentication. The wire messages are ASN.1 BER, built and parsed with the
[`asn1`](../libraries/asn1.md) library; the transport is [`net`](../libraries/net.md)
UDP. Needs the default `jennifer` binary (`net`).

```jennifer
import "snmp.j" as snmp;
use io;

def c as snmp.Client init snmp.client("192.0.2.10", "public");
def vbs as list of snmp.Varbind init snmp.get($c, ["1.3.6.1.2.1.1.1.0"]);
io.printf("%s = %s\n", $vbs[0].oid, $vbs[0].value);   # sysDescr

# Walk a subtree.
for (def vb in snmp.walk($c, "1.3.6.1.2.1.1")) {
    io.printf("%s [%s] %s\n", $vb.oid, $vb.type, $vb.value);
}
```

Runnable: [`examples/modules/snmp_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/snmp_demo.j).

## Client

```jennifer
export def struct Client {
    address as string, community as string,
    version as int, timeoutMs as int, retries as int
};
```

| Call | Returns | |
| ---- | ------- | - |
| `snmp.client(host, community)` | `Client` | Defaults: UDP port 161, SNMP v2c, 2s timeout, 1 retry. |
| `snmp.clientWith(address, community, version, timeoutMs, retries)` | `Client` | Full control; `address` is `"host:port"`, `version` is `snmp.VERSION1` or `snmp.VERSION2C`. |

Constants: `snmp.VERSION1` (`0`) and `snmp.VERSION2C` (`1`). v2c is the default;
many older or embedded devices (some printers, sensors) answer **v1 only** - if a
query times out, try `clientWith(..., snmp.VERSION1, ...)`.

## Requests

Each call sends one PDU and returns the response bindings, throwing a `snmp`-kind
`Error` if the agent reports an error status or does not answer within
`timeoutMs` (after `retries` extra attempts).

| Call | Returns | |
| ---- | ------- | - |
| `snmp.get(c, oids)`       | `list of Varbind` | GET the given OIDs (a `list of string`). |
| `snmp.getNext(c, oids)`   | `list of Varbind` | GETNEXT: the binding lexically after each OID. |
| `snmp.set(c, varbinds)`   | `list of Varbind` | SET the bindings (build them with `intVar` / `stringVar` / `oidVar`); returns the agent's echo. |
| `snmp.walk(c, rootOid)`   | `list of Varbind` | Repeated GETNEXT from `rootOid` until the OID leaves the subtree or the agent signals `endOfMibView`. |

## Bindings

```jennifer
export def struct Varbind { oid as string, type as string, value as string, number as int };
```

A returned `Varbind` carries the OID, the SNMP value `type`, a string `value`
rendering, and `number` (the integer for a numeric type, else `0`):

| `type` | `value` | `number` |
| ------ | ------- | -------- |
| `integer`      | decimal string | the value |
| `octetString`  | UTF-8 text if all printable, else `0x`-prefixed hex | `0` |
| `oid`          | dotted OID | `0` |
| `null`         | `""` | `0` |
| `counter32` / `gauge32` / `timeTicks` / `counter64` | decimal string (or `0x`-hex if `>= 2^63`) | the value (`0` if it exceeds int64) |
| `ipAddress`    | dotted `a.b.c.d` | `0` |
| `opaque`       | hex | `0` |
| `noSuchObject` / `noSuchInstance` / `endOfMibView` | `""` | `0` |

For a SET, build typed bindings:

| Call | Builds |
| ---- | ------ |
| `snmp.intVar(oid, n)`      | an `INTEGER` binding |
| `snmp.stringVar(oid, s)`   | an `OCTET STRING` binding |
| `snmp.oidVar(oid, target)` | an `OBJECT IDENTIFIER` binding |

## Agent (server)

The module is also an **agent** (server): it answers GET / GETNEXT / SET for a
MIB you supply. This is a hardware simulator for testing an SNMP tool, a
self-contained example (no external device), or a way to expose your own app's
metrics to an SNMP-native monitoring system.

```jennifer
import "snmp.j" as snmp;
use net;
use task;

# A MIB: one binding per OID, any SNMP value type.
def a as snmp.Agent init snmp.agent("public", snmp.VERSION2C, [
    snmp.stringVar("1.3.6.1.2.1.1.1.0", "Jennifer SNMP agent"),
    snmp.varbind("1.3.6.1.2.1.1.3.0", "timeTicks", "", 424242),
    snmp.intVar("1.3.6.1.2.1.1.7.0", 72)
]);

# Serve on a loopback socket in a spawned goroutine, then query it.
use channel;
def sock as net.UDPSocket init net.listenUDP("127.0.0.1:0");
def stop as channel of bool init channel.make(1);
def server as task of null init spawn { snmp.serveOn($a, $sock, $stop); };
def c as snmp.Client init snmp.clientWith(net.address($sock), "public", snmp.VERSION2C, 2000, 3);
snmp.get($c, ["1.3.6.1.2.1.1.1.0"]);      # -> "Jennifer SNMP agent"
channel.send($stop, true);                # graceful shutdown ...
task.wait($server);                       # ... then join the agent
```

| Call | Returns | |
| ---- | ------- | - |
| `snmp.agent(community, version, bindings)` | `Agent` | Build an agent from a `list of Varbind` MIB (build entries with `varbind` / `intVar` / `stringVar` / `oidVar`). |
| `snmp.serve(a, address)`  | *(blocks)* | Bind `address` (`":161"`) and serve forever. Run as the program's main loop, or in a `spawn`. |
| `snmp.serveOn(a, socket, stop)` | *(blocks)* | Serve on an already-bound `net.UDPSocket` until shut down (bind first, then `spawn` this, to avoid a start-up race). Stop it with `channel.send(stop, true)` on a `channel of bool` (capacity >= 1); the loop returns within ~250 ms, so `task.wait` the handle for a graceful join. |

The agent answers **GET** (a missing OID is a v2c `noSuchObject` exception, or a
v1 `noSuchName` error-status), **GETNEXT** (the next OID in numeric order - so
`walk` works; past the end is `endOfMibView`), and **SET** (updates an existing
binding; an unknown OID errors). A request with the wrong community, or a
malformed datagram, is dropped silently, as a real agent does.

> **Security.** Like any SNMP agent, this one is a UDP responder: a request can
> elicit a larger response (amplification), and the reply goes to the datagram's
> source address, which can be spoofed (reflection). The community string is
> plaintext (v1 / v2c has no real authentication). Treat the agent as a simulator
> / test tool or an internal-metrics endpoint - **do not expose it on an untrusted
> network** without a firewall or source allowlist in front of it.

Runnable:
[`examples/modules/snmp_agent_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/snmp_agent_demo.j)
runs an agent and a client in one process.

## Scope

SNMP v1 / v2c only - **no SNMPv3 / USM** (user-based security), which is the
authentication / privacy model a later tier would add. v1 / v2c has no privacy or
integrity either: the community travels in plaintext, and a response is matched
only by request-id (a random 31-bit value), not by source address - so a
network-level observer can read the community and a well-placed attacker can spoof
a reply. Treat the community as a filter, not a secret. GETBULK is not built (a
`walk` uses GETNEXT). `number` is `int64`-bounded, so a `Counter64` (or other
unsigned type) whose value is `>= 2^63` is returned as a `0x`-prefixed **hex**
`value` string with `number` `0`, rather than a wrong negative number. There is no
MIB compiler: OIDs are dotted numeric strings, and values are returned by SNMP
type, not resolved to named objects. The agent answers GET / GETNEXT / SET but
does not send **traps** / notifications, and does not implement **GETBULK**
(`walk` uses GETNEXT); its MIB is a flat `list of Varbind`, so lookup is linear
(fine for the hundreds of OIDs a simulator holds, not a huge tree). Every OID in
the MIB is **writable** by a client with the community - there is no read-only
distinction, so filter out any OID you do not want settable when you build the
agent's bindings.

## See also

The [`asn1`](../libraries/asn1.md) library (the BER/DER codec underneath) and
[`net`](../libraries/net.md) (the UDP transport); the module [index](index.md).
