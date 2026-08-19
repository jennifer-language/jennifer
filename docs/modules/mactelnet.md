# `mactelnet` - MikroTik MAC-Telnet (Layer-2) console

Import with `import "mactelnet.j" as mactelnet;`. Open a **Layer-2 console** to a
MikroTik RouterOS device **by MAC address**, with no IP configured on either side
- the same reach Winbox's "Neighbors" console gives you. This is how you set the
first IP address on a fresh or just-reset router, before switching to the
IP-based [`mikrotik`](mikrotik.md) API.

The transport is **UDP broadcast on port 20561** with the source and destination
MAC carried inside the packet payload, so a datagram reaches a router that has no
address yet. Built on [`net`](../libraries/net.md) (broadcast UDP, Linux-only),
[`crypto`](../libraries/crypto.md) (EC-SRP), and `hash` (MD5). Needs the default
`jennifer` binary. Errors throw `Error{kind: "mactelnet"}`.

```jennifer
import "mactelnet.j" as mactelnet;

# "eth0" is the local interface facing the router; its MAC is read from sysfs.
def s as mactelnet.Session init mactelnet.connect("eth0", "e4:8d:8c:11:22:33", "admin", "");
def banner as string init mactelnet.recv($s, 1000);

mactelnet.send($s, "/ip address add address=192.168.88.10/24 interface=ether1\r\n");
def out as string init mactelnet.recv($s, 1500);

mactelnet.close($s);
```

Runnable: [`examples/modules/mactelnet_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/mactelnet_demo.j).

## The provisioning flow

A brand-new or freshly-reset RouterOS device has no usable IP, so you cannot
reach its API over the network. MAC-Telnet is the bootstrap door:

1. `mactelnet.connect(...)` opens the L2 console and logs in.
2. `mactelnet.send(...)` an `/ip address add ...\r\n` to give it an address (and,
   for a reset device, whatever base config you need - a `/user` password, an
   `/ip service` tweak).
3. `mactelnet.close(...)`, then continue over IP with the
   [`mikrotik`](mikrotik.md) API (`mikrotik.optionsTLS` on any untrusted link).

It drives the router's **text CLI** (send a command line ending in `\r\n`, read
the echoed output), not the binary API - so you compose the same commands you
would type at the console.

## Authentication (auto-detected)

The router announces its auth generation by the length of the salt it sends, and
`connect` picks the algorithm to match - no flag needed:

| Salt | Algorithm | RouterOS |
| --- | --- | --- |
| 16 bytes | **MD5** (`0x00` + `MD5(0x00 + password + salt)`) | before 6.43 |
| 49 bytes | **EC-SRP** over Curve25519 (`crypto.mtwei*`) | 6.43+ and all v7 |

Modern devices - including anything factory-fresh - use EC-SRP, an Elliptic-Curve
Secure Remote Password exchange. Jennifer implements it dependency-free in the
[`crypto`](../libraries/crypto.md) library (`crypto.mtweiKeygen` /
`crypto.mtweiId` / `crypto.mtweiClientKey`), so no external crypto is pulled in and
the password never crosses the wire.

A blank password is valid (a factory-default `admin`). A failed login throws
`Error{kind: "mactelnet"}` carrying the router's own failure text.

## API

| Function | Returns | Description |
| --- | --- | --- |
| `connect(iface, mac, user, password)` | `Session` | run the full session-start + auth handshake; `iface` is the local interface (its MAC is read from `/sys/class/net/<iface>/address`), `mac` the router's MAC. |
| `send(s, text)` | `null` | send console input; end a command line with `"\r\n"`. |
| `recv(s, timeoutMs)` | `string` | read whatever output has arrived (waiting up to `timeoutMs` for the first byte); call again for more. ANSI escapes pass through verbatim. |
| `closed(s)` | `bool` | whether the router has ended the session. |
| `close(s)` | `null` | end the session and release the socket. |
| `parseMac(s)` | `bytes` | parse `"aa:bb:cc:dd:ee:ff"` (or `-`/none separators) to 6 bytes. |
| `formatMac(b)` | `string` | format 6 bytes as `"aa:bb:cc:dd:ee:ff"`. |

`Session` fields: `sock` (the broadcast `net.UDPSocket`), `state` (a `kv.Store`
holding the mutable send/receive counters and pending output), `srcmac` /
`dstmac` (the local and router MACs), `seskey` (the session id).

## Requirements and caveats

- **Linux + default binary only.** Broadcast UDP needs the `net` stack (absent on
  `jennifer-tiny`) and `SO_BROADCAST` (Linux). The local MAC is read from Linux
  sysfs.
- **Same broadcast segment.** Client and router must share a Layer-2 segment (no
  router in between) - it is a link-local protocol.
- **`mac-server` must allow the interface.** RouterOS enables MAC-Telnet on all
  interfaces by default; a hardened or narrowly-configured device may have
  disabled it (`/tool mac-server`), in which case `connect` times out. A
  no-defaults reset restores the default, so provisioning normally works.
- **After a reset the router reboots** and the session drops - reconnect once it
  is back.

> **SECURITY.** MAC-Telnet is a cleartext Layer-2 protocol: the password proof is
> hashed, but the session itself is not encrypted and any host on the segment can
> read it. Use it only on a trusted wire to bootstrap a device, then move to the
> certificate-or-TLS IP path.

## See also

- [`mikrotik`](mikrotik.md) - the IP-based RouterOS API you switch to once an
  address is set.
- [`crypto`](../libraries/crypto.md) - the `mtwei*` EC-SRP primitives this module
  builds on.
