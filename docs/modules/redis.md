# `redis` - a Redis client (RESP2)

Import with `import "redis.j" as redis;`. A **Redis** client speaking
**RESP2** (the REdis Serialization Protocol) over the `net` system library.
Commands go out as RESP arrays of bulk strings; replies (`+OK`, `-ERR`,
`:int`, `$bulk`, `*array`) parse back into a `Reply`. Typed per-command
helpers (`get` / `set` / `incr` / `keys` / ...) keep the common path fully
typed; `command` is the generic escape hatch for everything else. Because it
uses `net`, this module needs the default **`jennifer`** binary.

> **On `jennifer-tiny`:** "needs the default `jennifer` binary" refers to the
> **stock** tiny build, which ships without a network driver - not a TinyGo
> limitation. A `jennifer-tiny` rebuilt with a network stack runs this module
> too; see the
> [note on `net` and TinyGo](../technical/tinygo.md#net-on-tinygo-is-a-build-choice-not-a-hard-limit).

```jennifer
import "redis.j" as redis;

def db as redis.Session init redis.connect(redis.Options{host: "127.0.0.1",
    port: 6379, security: "none", user: "", password: "", db: 0});
redis.set($db, "greeting", "hello");
io.printf("%s\n", redis.get($db, "greeting"));     # hello
io.printf("visits: %d\n", redis.incr($db, "visits"));
redis.quit($db);
```

Runnable: [`examples/modules/redis_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/redis_demo.j).

## Surface

A session is stateful: `connect`, issue commands, `quit`.

| Call / type                          | Notes                                                                 |
| ------------------------------------ | --------------------------------------------------------------------- |
| `redis.Options`                      | `host`, `port`, `security`, `user`, `password`, `db`.                 |
| `redis.Session`                      | A live session over one connection (from `connect`).                  |
| `redis.Reply`                        | A parsed reply: `kind`, `str`, `num`, `items` (see below).            |
| `redis.connect(opts)`                | Open a session; `AUTH` when `password` set, `SELECT` when `db > 0`.   |
| `redis.command(session, args)`       | Send any command (`list of string`); returns the raw `Reply`.         |
| `redis.get(session, key)`            | `GET` - the string value, or `""` when the key is missing.            |
| `redis.set(session, key, value)`     | `SET`.                                                                 |
| `redis.del(session, key)`            | `DEL` - number of keys removed (0 or 1).                              |
| `redis.exists(session, key)`         | `EXISTS` - `bool`.                                                    |
| `redis.incr(session, key)`           | `INCR` - the new value (`int`).                                       |
| `redis.decr(session, key)`           | `DECR` - the new value (`int`).                                       |
| `redis.keys(session, pattern)`       | `KEYS` - `list of string` matching a glob (`"*"`, `"user:*"`).        |
| `redis.ping(session)`                | `PING` - the server's `"PONG"`.                                       |
| `redis.quit(session)`                | `QUIT` and close.                                                     |

`Options.security` is `"none"` (plaintext, port 6379) or `"tls"` (implicit
TLS, `rediss`). `password` `""` skips `AUTH`; `db` `0` skips `SELECT`. When a
`user` is set alongside `password`, `AUTH user password` (ACL) is sent;
otherwise `AUTH password`.

## The generic `command` and `Reply`

Every typed helper is a thin wrapper over `command`, which sends an arbitrary
argument list and returns the raw `Reply` - use it for any command without a
helper:

```jennifer
def r as redis.Reply init redis.command($db, ["LPUSH", "queue", "job-1"]);
io.printf("list length now %d\n", $r.num);
def range as redis.Reply init redis.command($db, ["LRANGE", "queue", "0", "-1"]);
for (def item in $range.items) {
    io.printf("  %s\n", $item.str);
}
```

A `Reply` is walked by its `kind` and the matching field, the same shape a
[`json.Value`](../libraries/json.md) is walked with accessors:

| `kind`     | RESP source        | Read from    |
| ---------- | ------------------ | ------------ |
| `"string"` | `+simple` / `$bulk` | `.str`      |
| `"error"`  | `-ERR`             | `.str` (but see below) |
| `"int"`    | `:123`             | `.num`       |
| `"nil"`    | `$-1` / `*-1`      | (absent)     |
| `"array"`  | `*N`               | `.items` (a `list of Reply`) |

## Errors

A `-ERR` reply throws a catchable `Error` (kind `"redis"`) at the call site,
so a bad command surfaces like any other runtime error:

```jennifer
try {
    redis.command($db, ["INCR", "greeting"]);   # greeting holds "hello"
} catch (e) {
    io.printf("redis said: %s\n", $e.message);  # ERR value is not an integer...
}
```

`command` only throws on an error *reply*; a network failure surfaces as the
underlying `net` error.

## Bulk strings are read as UTF-8 text

RESP bulk-string lengths are byte counts, but the parser reads values as
UTF-8 text. This is byte-exact for ASCII and UTF-8 string values - the common
case (keys, JSON payloads, counters). A binary value whose byte length
differs from its rune length is not yet byte-exact; store such values
base64-encoded (via [`encoding`](../libraries/encoding.md)) until a
byte-native read lands.

## Testing

The pure protocol logic - the RESP command encoder and the simple-string /
error / integer / bulk / nil / array decoder, including the incomplete-buffer
and leftover-buffer cases - is unit-tested in the overlay
(`modules/redis_test.j`). The networked session is covered end to end by an
in-process RESP server in the Go test suite (`TestRedisCommands`), so it runs
in CI without a Redis install.

## Out of scope

- **A working subset**, not the full command set: strings, counters, keys,
  and the generic `command` for the rest. Lists / hashes / sets are reachable
  through `command`; typed helpers for them can follow.
- **No pipelining, pub/sub, or RESP3.** One request, one reply.
- **No connection pool.** One `Session` is one connection.
- **`rediss` TLS** rides `net`'s default certificate verification.

## Timeouts and limits

The initial connect is bounded by a connection-establishment timeout, so a slow
or unreachable server fails the dial instead of blocking it forever. A single
reply is also capped at **64 MiB**: a malicious or compromised server that
declares an enormous bulk-string length (or never terminates a reply) fails with
a catchable error rather than growing the read buffer without bound.

Every read carries an idle timeout (default 30 s) so a hung server fails with a
catchable error instead of blocking the caller forever. `connect` sets
`Session.timeout` (milliseconds); lower it for a tighter bound, or set it to `0`
to disable:

```jennifer
def s as redis.Session init redis.connect($opts);
$s.timeout = 5000;   # fail a read that stalls for 5 s
```

## See also

- [json.md](../libraries/json.md) - the same accessor-walked-reply shape.
- [net.md](../libraries/net.md) - the transport `redis` builds on.
- [modules/index.md](index.md) - the module catalog and import rules.
