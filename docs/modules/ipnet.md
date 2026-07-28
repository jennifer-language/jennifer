# `ipnet` - IP addresses and CIDR networks

Import with `import "ipnet.j" as ipnet;`. Parse and reason about **IPv4 and
IPv6** addresses and CIDR blocks: canonical formatting, membership tests, subnet
math (netmask, broadcast, split, aggregate, host iteration), and address
classification (private / loopback / global / ...). Addresses are held as raw
`bytes` (4 for IPv4, 16 for IPv6); the math is bitwise. Pure Jennifer over
`strings` + `convert`; runs on **both** binaries.

```jennifer
import "ipnet.j" as ipnet;

def net as ipnet.Network init ipnet.parse("192.168.1.0/24");
def ip as ipnet.Address init ipnet.parseAddress("192.168.1.42");
def inside as bool init ipnet.contains($net, $ip);   # true
```

Runnable: [`examples/modules/ipnet_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/ipnet_demo.j).

## Types

Both structs have public fields (read them directly), and the builder functions
are the conventional way to construct them.

```jennifer
def struct ipnet.Address { version as int, octets as bytes };   # version 4 or 6
def struct ipnet.Network { addr as Address, prefix as int };
```

- `Address.version` is `4` or `6`; `Address.octets` is the raw address bytes (4
  or 16), in network byte order.
- `Network.addr` is the base address with host bits zeroed; `Network.prefix` is
  the prefix length (`0..32` for IPv4, `0..128` for IPv6).

## Addresses

| Call | Returns | |
| ---- | ------- | - |
| `ipnet.parseAddress(s)` | `Address` | parse an IPv4 dotted-quad or IPv6 address |
| `ipnet.toString(addr)` | `string` | canonical text (RFC 5952 for IPv6) |
| `ipnet.version(addr)` | `int` | `4` or `6` |
| `ipnet.equal(a, b)` | `bool` | same version and bytes |
| `ipnet.unmap(addr)` | `Address` | fold a v4-mapped IPv6 address down to v4 |
| `ipnet.next(addr)` | `Address` | the next address (+1); throws at the last |
| `ipnet.prev(addr)` | `Address` | the previous address (-1); throws at the first |
| `ipnet.compare(a, b)` | `int` | `-1` / `0` / `1` (v4 orders before v6) |

`parseAddress` accepts IPv6 with `::` zero-compression and a trailing embedded
IPv4 (`::ffff:192.168.1.1`). `toString` renders IPv6 canonically per RFC 5952:
lowercase, no leading zeros, and the longest run of two-or-more zero groups
compressed to `::` (leftmost on a tie). Because `equal` compares bytes, two
different spellings of the same address compare equal. `next` / `prev` /
`compare` give a total order, so you can walk or sort addresses.

## CIDR networks

| Call | Returns | |
| ---- | ------- | - |
| `ipnet.parse(cidr)` | `Network` | parse `address/prefix` (host bits zeroed) |
| `ipnet.networkString(net)` | `string` | render as `address/prefix` |
| `ipnet.contains(net, addr)` | `bool` | is the address in the network? |
| `ipnet.netmask(net)` | `Address` | the netmask (e.g. `255.255.255.0`) |
| `ipnet.broadcast(net)` | `Address` | the last address (IPv4 broadcast) |

`parse` zeroes the host bits, so `ipnet.parse("192.168.1.42/24")` has base
`192.168.1.0`. `contains` returns `false` for a version mismatch (an IPv4
address is never inside an IPv6 network). `broadcast` sets every host bit: for
IPv4 that is the broadcast address, for IPv6 the last address in the block.

```jennifer
def allowed as list of ipnet.Network init [ipnet.parse("10.0.0.0/8"), ipnet.parse("192.168.0.0/16")];
def client as ipnet.Address init ipnet.parseAddress("10.4.5.6");
def ok as bool init false;
for (def net in $allowed) {
    if (ipnet.contains($net, $client)) { $ok = true; }
}
```

## Subnet math

| Call | Returns | |
| ---- | ------- | - |
| `ipnet.hostCount(net)` | `int` | total addresses in the block (`2 ^ host-bits`) |
| `ipnet.firstUsable(net)` | `Address` | first usable host (see below) |
| `ipnet.lastUsable(net)` | `Address` | last usable host (see below) |
| `ipnet.hosts(net)` | `list of Address` | every usable host, ascending (capped) |
| `ipnet.split(net, newPrefix)` | `list of Network` | divide into longer-prefix subnets |
| `ipnet.aggregate(nets)` | `list of Network` | collapse to the minimal covering set |
| `ipnet.overlaps(a, b)` | `bool` | do two networks intersect? |
| `ipnet.subnetOf(child, parent)` | `bool` | is `child` wholly inside `parent`? |

- **`hostCount`** counts every address in the block. IPv4 always fits an `int`;
  an IPv6 prefix must be `>= 66` (a shorter one overflows `int` and throws).
- **`firstUsable` / `lastUsable`** apply the usual host conventions. IPv4 excludes
  the network and broadcast addresses, except a `/31` (RFC 3021 point-to-point,
  both usable) and a `/32` (single host). IPv6 has no broadcast, so the last
  address is usable; the first is the base + 1 (skipping the subnet-router
  anycast), except `/127` and `/128`.
- **`hosts`** materializes the usable range - capped at **65536** addresses; a
  larger block throws (walk it with `next` instead), so this cannot exhaust
  memory.
- **`split`** divides a network into the equal subnets of a longer prefix (a
  `/24` into four `/26`s), capped at **65536** subnets (`newPrefix` at most 16
  bits longer).
- **`aggregate`** drops contained blocks and folds adjacent sibling pairs into
  their shorter parent (`192.168.0.0/25` + `192.168.0.128/25` ->
  `192.168.0.0/24`), cascading until stable. IPv4 and IPv6 entries aggregate
  independently; the result is sorted ascending.

```jennifer
def subnets as list of ipnet.Network init ipnet.split(ipnet.parse("192.168.1.0/24"), 26);
# -> 192.168.1.0/26, .64/26, .128/26, .192/26

def merged as list of ipnet.Network init ipnet.aggregate([
    ipnet.parse("10.0.0.0/9"), ipnet.parse("10.128.0.0/9")
]);   # -> [10.0.0.0/8]
```

## Classification

| Call | Returns | |
| ---- | ------- | - |
| `ipnet.scope(addr)` | `Scope` | the address's category (see below) |
| `ipnet.isGlobal(addr)` | `bool` | a normal globally-routable unicast address |
| `ipnet.isPrivate(addr)` | `bool` | RFC 1918 or IPv6 ULA (`fc00::/7`) |
| `ipnet.isLoopback(addr)` | `bool` | `127.0.0.0/8` or `::1` |
| `ipnet.isLinkLocal(addr)` | `bool` | `169.254.0.0/16` or `fe80::/10` |
| `ipnet.isMulticast(addr)` | `bool` | `224.0.0.0/4` or `ff00::/8` |
| `ipnet.isUnspecified(addr)` | `bool` | `0.0.0.0` or `::` |

`scope` is a **total, disjoint** classification: every address maps to exactly
one variant of the enum

```jennifer
def enum ipnet.Scope { Global, Private, Loopback, LinkLocal, Multicast, Unspecified, Reserved };
```

so the `is*` predicates are thin wrappers over it, and you can `match` the whole
set in one place. `Reserved` covers the special-purpose ranges that are neither a
normal global address nor one of the named categories (documentation
`192.0.2.0/24` / `2001:db8::/32`, shared CGNAT `100.64.0.0/10`, benchmarking,
`240.0.0.0/4`, and other reserved blocks). A v4-mapped IPv6 address is folded to
its v4 self first, so `::ffff:10.0.0.1` classifies as `Private`.

```jennifer
match (ipnet.scope($client)) {
    when Global { web.serve($client); }
    when Private, Loopback, LinkLocal { web.serveInternal($client); }
    else { web.reject($client); }
}
```

The classification follows the well-known special-purpose registries and is
**best-effort**: `isGlobal` means "not one of the categories above", not a
routing-table check.

## Errors

Malformed input throws `Error{kind: "ipnet"}` (a bad octet, too few / many
groups, multiple `::`, a bad hex digit, a missing `/prefix`, an out-of-range
prefix, an address overflow at `next` / `prev`, or a too-large `hosts` /
`hostCount` / `split`) - catch it with `try` / `catch`.

## Scope

- **Address and prefix math, not a resolver.** No DNS, no interface
  enumeration; hostname / interface lookups live in the `net` library.
- **Strict decimal octets.** `parseAddress` rejects a leading-zero IPv4 octet
  (`192.168.001.1`): many host stacks read a leading-zero octet as octal, so
  accepting it as decimal would let an allow-list disagree with the kernel (an
  SSRF / allow-list-bypass vector).
- **IPv4-mapped IPv6 folds to v4.** `parseAddress("::ffff:127.0.0.1")` returns a
  version-4 `Address` (via `unmap`), so it matches a v4 allow-list; `parse`
  likewise folds a v4-mapped CIDR (`::ffff:0:0/96` -> `0.0.0.0/0`), translating
  the prefix. The deprecated IPv4-compatible form (`::a.b.c.d`) is left as v6
  (ambiguous with low addresses like `::1`).
- **Sequence-number-free.** Iteration and aggregation work on the address bytes
  directly; there is no scope-id / zone-id parsing.

## See also

- [net.md](../libraries/net.md) - sockets, TLS, and DNS lookups.
- [strings.md](../libraries/strings.md) - the text surface the parser builds on.
- [modules/index.md](index.md) - the module catalog and import rules.
