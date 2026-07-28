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
| `redis.Message`                      | A pushed pub/sub message: `kind`, `channel`, `pattern`, `payload`.    |
| `redis.ScanResult`                   | One `scan` page: `cursor` (`int`), `keys` (`list of string`).         |
| `redis.connect(opts)`                | Open a session; `AUTH` when `password` set, `SELECT` when `db > 0`.   |
| `redis.command(session, args)`       | Send any command (`list of string`); returns the raw `Reply`.         |
| `redis.get(session, key)`            | `GET` - the string value, or `""` when the key is missing.            |
| `redis.set(session, key, value)`     | `SET`.                                                                 |
| `redis.getBytes(session, key)`       | `GET` returning raw `bytes` (byte-exact; empty when missing) - for a binary value. |
| `redis.setBytes(session, key, value)`| `SET` a raw `bytes` value byte-for-byte (a blob / compressed payload). |
| `redis.del(session, key)`            | `DEL` - number of keys removed (0 or 1).                              |
| `redis.exists(session, key)`         | `EXISTS` - `bool`.                                                    |
| `redis.incr(session, key)`           | `INCR` - the new value (`int`).                                       |
| `redis.decr(session, key)`           | `DECR` - the new value (`int`).                                       |
| `redis.keys(session, pattern)`       | `KEYS` - `list of string` matching a glob (production-unsafe; see below). |
| `redis.scan(session, cursor, pattern, count)` | `SCAN` - one `ScanResult` page (the production-safe walk).    |
| `redis.ping(session)`                | `PING` - the server's `"PONG"`.                                       |
| `redis.quit(session)`                | `QUIT` and close.                                                     |
| `redis.subscribe(session, channels)` | `SUBSCRIBE` to a `list of string` of channels.                        |
| `redis.psubscribe(session, patterns)`| `PSUBSCRIBE` to a `list of string` of glob patterns.                  |
| `redis.unsubscribe(session, channels)` | `UNSUBSCRIBE` (`[]` leaves all channels).                           |
| `redis.punsubscribe(session, patterns)` | `PUNSUBSCRIBE` (`[]` leaves all patterns).                         |
| `redis.publish(session, channel, message)` | `PUBLISH` - the number of subscribers that received it (`int`). |
| `redis.receiveMessage(session)`      | Block for the next `Message` push (pair with `spawn`).                |
| `redis.pipeline(session, commands)`  | Send `list of list of string`, read one `Reply` each (one round trip). |
| `redis.multi(session)`               | `MULTI` - begin a transaction.                                        |
| `redis.exec(session)`                | `EXEC` - run the queued commands; `list of Reply` (empty if aborted). |
| `redis.discard(session)`             | `DISCARD` - drop the queued transaction.                              |
| `redis.hset` / `hget` / `hgetAll` / `hdel` | Hash: `HSET` (new-field count), `HGET` (string), `HGETALL` (`map of string to string`), `HDEL` (removed count). |
| `redis.lpush` / `rpush` / `lrange` / `llen` / `lpop` | List: push (new length), `LRANGE start stop` (`list of string`, `0, -1` = all), `LLEN`, `LPOP` (string). |
| `redis.sadd` / `srem` / `smembers` / `sismember` / `scard` | Set: add / remove (count), `SMEMBERS` (`list of string`), `SISMEMBER` (`bool`), `SCARD` (`int`). |

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

## Pub/Sub

`publish` sends a message on an ordinary connection and returns the number of
subscribers that received it. On the receiving side, `subscribe` /
`psubscribe` put a connection into **subscribed mode**, and `receiveMessage`
blocks for the next push:

```jennifer
# Publisher (an ordinary connection).
def n as int init redis.publish($db, "news", "hello");   # subscribers reached

# Subscriber (a dedicated connection).
def sub as redis.Session init redis.connect($opts);
redis.subscribe($sub, ["news", "weather"]);
def msg as redis.Message init redis.receiveMessage($sub);
io.printf("[%s] %s\n", $msg.channel, $msg.payload);
```

A `Message` has `kind` (`"message"` for a channel push, `"pmessage"` for a
pattern push), `channel`, `pattern` (`""` for a plain message), and `payload`.
`receiveMessage` drains the interleaved `subscribe` / `unsubscribe`
confirmation frames itself and returns only real `message` / `pmessage`
pushes; a server error reply or a closed connection throws a catchable `Error`
(kind `"redis"`).

**A subscribed connection is restricted.** Once subscribed, the connection may
only run `(P)SUBSCRIBE` / `(P)UNSUBSCRIBE` / `PING` / `QUIT` until it is fully
unsubscribed - the server rejects any other command. Use a **separate**
connection for `publish` and normal commands. `unsubscribe($sub, [])` (an empty
list) leaves every channel; `punsubscribe($sub, [])` every pattern.

### The receive loop (with `spawn`)

`receiveMessage` is a plain blocking call - there are no handler callbacks.
The program opts into concurrency by wrapping the loop in its own `spawn`, so
it can keep working while messages arrive:

```jennifer
def worker as task of null init spawn {
    while (true) {
        def m as redis.Message init redis.receiveMessage($sub);
        handle($m.channel, $m.payload);
    }
};
```

The per-read idle timeout (`Session.timeout`, milliseconds) bounds each wait:
lower it for a tighter poll and catch the `Error` a stalled read raises, or set
it to `0` to block indefinitely.

## Pipelining

`pipeline` sends several commands in one write and reads exactly one reply per
command, so N commands cost a single network round trip instead of N:

```jennifer
def replies as list of redis.Reply init redis.pipeline($db, [
    ["SET", "a", "1"],
    ["INCR", "a"],
    ["GET", "a"],
]);
io.printf("a is now %s\n", $replies[2].str);   # 2
```

Unlike `command`, `pipeline` does **not** throw on a `-ERR` reply - a pipeline
can mix succeeding and failing commands, so each command's outcome is returned
in order and the caller inspects each `Reply` (an error reply has
`kind == "error"`).

## Transactions

`multi` / `exec` / `discard` wrap `MULTI` / `EXEC` / `DISCARD`. Commands issued
between `multi` and `exec` are **queued** (each replies `+QUEUED`); `exec` runs
them atomically and returns one `Reply` per queued command, in order:

```jennifer
redis.multi($db);
redis.command($db, ["SET", "counter", "10"]);   # +QUEUED
redis.command($db, ["INCR", "counter"]);         # +QUEUED
def results as list of redis.Reply init redis.exec($db);
io.printf("counter is %d\n", $results[1].num);   # 11
```

An aborted transaction (a `nil` `EXEC` reply) yields an empty list. `discard`
drops the queued commands without running them.

## `scan` vs `keys`

`keys` returns every match in one shot, but it **blocks the whole server**
while it walks the keyspace - fine for a small dev database, unsafe in
production. `scan` is the cursor-based iterator: it returns a page of keys and
the next cursor, so the server stays responsive. Start at cursor `0` and repeat
until the returned cursor is `0` again:

```jennifer
def cursor as int init 0;
repeat {
    def page as redis.ScanResult init redis.scan($db, $cursor, "user:*", 100);
    for (def k in $page.keys) {
        io.printf("%s\n", $k);
    }
    $cursor = $page.cursor;
} until ($cursor == 0);
```

Pass `pattern` `""` to skip the `MATCH` filter and `count` `0` to skip the
`COUNT` per-page hint (`COUNT` is advisory - a page may hold more or fewer
keys, and an empty page with a non-zero cursor is normal).

## Binary values: `setBytes` / `getBytes`

`get` / `set` speak **text**: RESP bulk lengths are byte counts, but the string
verbs decode a value as UTF-8, which is exact for keys, JSON payloads, and
counters, and **throws** on a non-UTF-8 value (strict, by design). For an
arbitrary **binary** value - a serialized blob, a compressed payload, an image -
use `setBytes(session, key, value)` (a `bytes` value sent byte-for-byte) and
`getBytes(session, key)` (`bytes`, never decoded):

```jennifer
redis.setBytes($s, "blob", $rawBytes);         # any bytes, exact
def back as bytes init redis.getBytes($s, "blob");   # round-trips byte-for-byte
```

`getBytes` returns empty `bytes` for a missing key (use `exists` to tell that
from an empty value). The typed hash / list / set helpers are string-valued; for
a binary field / element / member, build the command with `command` and the same
byte-encoding the `setBytes` path uses, or store the blob at a plain key.

## Testing

The pure protocol logic - the RESP command encoder (text and the byte-exact
`encodeCommandBytes`) and the simple-string / error / integer / bulk / nil /
array decoder (including the incomplete-buffer and leftover-buffer cases), the
pub/sub command builders, the `message` / `pmessage` push classifier, the
pipeline encoder, and the SCAN reply reader - is unit-tested in the overlay
(`modules/redis_test.j`). The networked session, pub/sub delivery, pipelining,
transactions, SCAN, the binary `setBytes` / `getBytes` round-trip, and the typed
hash / list / set helpers are covered end to end by in-process RESP servers in
the Go test suite (`TestRedisCommands`, `TestRedisBinaryAndTyped`), so they run
in CI without a Redis install.

## Testing

The pure protocol logic - the RESP command encoder and the simple-string /
error / integer / bulk / nil / array decoder (including the incomplete-buffer
and leftover-buffer cases), the pub/sub command builders, the
`message` / `pmessage` push classifier, the pipeline encoder, and the SCAN
reply reader - is unit-tested in the overlay (`modules/redis_test.j`). The
networked session, pub/sub delivery, pipelining, transactions, and SCAN are
covered end to end by an in-process RESP server in the Go test suite
(`TestRedisCommands`), so it runs in CI without a Redis install.

## Out of scope

- **A working subset**, not the full command set: strings, counters, keys,
  pub/sub, pipelining, transactions, and SCAN, plus the generic `command` for
  the rest. Lists / hashes / sets are reachable through `command`; typed
  helpers for them can follow.
- **RESP2 only, no RESP3.** Pub/sub pushes are read from the same connection
  as replies (the RESP2 model), not over a RESP3 out-of-band push channel.
- **No connection pool.** One `Session` is one connection; a subscribed
  connection needs its own session, separate from the one issuing `publish`
  and ordinary commands.
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
