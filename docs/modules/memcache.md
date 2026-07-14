# `memcache` - a memcached client

Import with `import "memcache.j" as memcache;`. A client for a **memcached**
server, speaking its classic **text protocol** over the `net` system library.
Store with an expiration (`set` / `add`), read (`get`), remove (`delete`),
count atomically (`incr` / `decr`), and re-arm a key's expiry (`touch`).
memcached is a **volatile cache** - keys expire on their `exptime` and the
server evicts under memory pressure - so it suits sessions, rate limits, and
derived data, not a system of record. Because it uses `net`, this module needs
the default **`jennifer`** binary.

> **On `jennifer-tiny`:** "needs the default `jennifer` binary" refers to the
> **stock** tiny build, which ships without a network driver - not a TinyGo
> limitation. A `jennifer-tiny` rebuilt with a network stack runs this module
> too; see the
> [note on `net` and TinyGo](../technical/tinygo.md#net-on-tinygo-is-a-build-choice-not-a-hard-limit).

```jennifer
import "memcache.j" as memcache;

def mc as memcache.Session init memcache.connect(memcache.Options{
    host: "127.0.0.1", port: 11211});
memcache.set($mc, "greeting", "hello", 60);        # 60-second TTL
io.printf("%s\n", memcache.get($mc, "greeting"));   # hello
memcache.quit($mc);
```

Runnable: [`examples/modules/memcache_demo.j`](https://github.com/mplx/jennifer-lang/blob/main/examples/modules/memcache_demo.j).

## Surface

A session is stateful: `connect`, issue commands, `quit`. Every store carries
an `exptime` in **seconds** (`0` = never expire, until evicted).

| Call / type                                  | Notes                                                              |
| -------------------------------------------- | ------------------------------------------------------------------ |
| `memcache.Options`                           | `host`, `port` (plaintext; the text protocol has no auth / TLS).   |
| `memcache.Session`                           | A live session over one connection (from `connect`).               |
| `memcache.connect(opts)`                     | Open a session.                                                    |
| `memcache.set(session, key, value, exptime)` | Store `value` (replacing any existing), TTL `exptime` seconds.     |
| `memcache.add(session, key, value, exptime)` | Store only if the key is **absent**; returns whether it stored.    |
| `memcache.get(session, key)`                 | The string value, or `""` when the key is absent / expired.        |
| `memcache.delete(session, key)`              | Remove the key; returns whether it existed.                        |
| `memcache.incr(session, key, delta)`         | Atomically add `delta`; the new value, or `-1` if the key is absent. |
| `memcache.decr(session, key, delta)`         | Atomically subtract `delta` (not below 0); `-1` if absent.         |
| `memcache.touch(session, key, exptime)`      | Re-arm the key's expiry; returns whether it existed.               |
| `memcache.quit(session)`                     | End the session and close.                                         |

## `add`, `incr`, and the primitives caches are built from

`add` stores **only if the key does not already exist** and reports which
happened - the atomic building block for a lock ("did I win the key?") or a
create-if-new. `incr` / `decr` are atomic server-side counters; memcached will
not create a missing counter, so `incr` on an absent key returns `-1` and the
caller decides whether to `add` an initial value:

```jennifer
def n as int init memcache.incr($mc, "hits", 1);
if ($n == -1) {                                  # first hit this window
    memcache.add($mc, "hits", "1", 60);
    $n = 1;
}
```

That `incr`-then-`add` shape is exactly what the planned `ratelimit` module
builds on, and `add` + a TTL is what the planned `session` module uses to mint
a session; both are small modules designed to sit on top of this client.

## Errors

A protocol error reply (`ERROR` / `CLIENT_ERROR` / `SERVER_ERROR`) throws a
catchable `Error` (kind `"memcache"`); `set` also throws if the server does not
answer `STORED`. A network failure surfaces as the underlying `net` error.

## Values are read as UTF-8 text

The stored byte count is exact on the wire, but the parser reads a value back
as UTF-8 text, so `get` is byte-exact for ASCII and UTF-8 values whose byte
length equals their rune length - the common case (JSON, numbers, identifiers).
A binary value, or one whose multi-byte runes make byte length differ from rune
length, is not yet byte-exact; store such values base64-encoded (via
[`encoding`](../libraries/encoding.md)) until a byte-native read lands. This is
the same limitation as [`redis`](redis.md).

## Out of scope

- **A working subset**, not the full command set: `gets` / `cas`,
  `append` / `prepend`, `stats`, and multi-key `get` are reachable later; the
  basics cover caches, sessions, counters, and locks.
- **No binary protocol and no SASL auth.** Classic text protocol only.
- **No connection pool.** One `Session` is one connection.

## Timeouts

Every read carries an idle timeout (default 30 s) so a hung server fails with a
catchable error instead of blocking the caller forever. `connect` sets
`Session.timeout` (milliseconds); lower it for a tighter bound, or set it to `0`
to disable:

```jennifer
def s as memcache.Session init memcache.connect($opts);
$s.timeout = 5000;   # fail a read that stalls for 5 s
```

## See also

- [net.md](../libraries/net.md) - the transport `memcache` builds on.
- [modules/index.md](index.md) - the module catalog and import rules.
