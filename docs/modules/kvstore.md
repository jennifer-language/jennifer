# `kvstore` - selectable key/value backend

Import with `import "kvstore.j" as kvstore;`. A key/value store with per-key
TTL behind one `Store` value: the in-process [`kv`](../libraries/kv.md)
library, or a `memcache` / `redis` connection, with a uniform `set` / `get` /
`delete` / `touch` / `incrWindow` surface over whichever you chose. It is the
shared backend layer under [`session`](session.md) and
[`ratelimit`](ratelimit.md), so each works over any backend.

The module holds the local backends and imports no network module, so it runs
on both binaries. The distributed backends are built by the client modules,
`memcache.kvStore(mc)` and `redis.kvStore(rc)`, which need the default binary.

```jennifer
import "kvstore.j" as kvstore;

# pick one backend:
def store as kvstore.Store init kvstore.inProcessStore();      # local, in memory
# def store as kvstore.Store init kvstore.fileStore("state.kv"); # local, persisted
# def store as kvstore.Store init redis.kvStore($rc);           # distributed (import "redis.j")
# def store as kvstore.Store init memcache.kvStore($mc);        # distributed (import "memcache.j")

kvstore.set($store, "greeting", "hello", 60);   # expires in 60 s
io.printf("%s\n", kvstore.get($store, "greeting"));
```

Runnable: [`examples/modules/kvstore_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/kvstore_demo.j) (in-process backend, so it needs no server).

## `Store` is an enum

`Store` is a sum type; every op is a `match` over its two variants:

```jennifer
export def enum Store {
    Local { local as kv.Store },
    Remote { backend as Backend }
};
```

A `Remote` store carries a `Backend`: the connection plus one `func` value per
verb, filled in by `memcache.kvStore` / `redis.kvStore` and run in that module.
This keeps `kvstore` free of the network modules (a module that imports one
carries its `net` capability and does not load on `jennifer-tiny`), while
`session` and `ratelimit` still take any backend.

## Surface

| Call | Backend | |
| ---- | ------- | - |
| `kvstore.inProcessStore()` | local | in memory, this process only (reset each run) |
| `kvstore.fileStore(path)` | local | persisted to `path`, survives across runs |
| `memcache.kvStore(mc)` | memcache | distributed; state on the server (default binary) |
| `redis.kvStore(rc)` | redis | distributed; state on the server (default binary); the session must not be subscribed |
| `kvstore.remoteStore(backend)` | remote | the `Store` of a `Backend`; called by the two above |
| `kvstore.set(store, key, value, ttlSec)` | | store with a `ttlSec`-second expiry (`0` = none; negative: kind `"kvstore"`, since the backends disagree on it) |
| `kvstore.get(store, key)` | | the value, or `""` when absent / expired |
| `kvstore.delete(store, key)` | | remove the key; `bool` (existed) |
| `kvstore.touch(store, key, ttlSec)` | | re-arm the expiry; `bool` (existed); negative `ttlSec`: kind `"kvstore"` |
| `kvstore.incrWindow(store, key, ttlSec)` | | atomic increment, creating the key at 1 with a `ttlSec` TTL on the first hit; the new value; negative `ttlSec`: kind `"kvstore"`. The fixed-window rate-limit primitive, portable across all backends. |

## Choosing a backend

- **Distributed** (`memcache` / `redis`) - state lives on the server, shared
  across every process and machine that connects: a web fleet, a worker pool,
  anything where more than one process must agree.
- **Local in-memory** (`inProcessStore`) - no server, fastest, but gone when the
  program exits and invisible to other processes. Good for a single-process
  cache or a test. Expired entries are swept periodically, so a rate limiter over
  it stays bounded; but there is no cap on unexpired data, so an adversarial,
  unbounded working set (millions of live sessions from an attacker) belongs on
  a distributed backend with a server-side eviction policy, not here.
- **Local persisted** (`fileStore`) - no server, and it survives across
  `jennifer run` invocations (for development and CLI tools that keep state).
  Single-process: safe for `spawn` within one process, but concurrent
  *separate* processes on one file can lose writes - use a distributed backend
  for cross-process coordination.

## See also

- [kv.md](../libraries/kv.md) - the in-process store behind the local backend.
- [memcache.md](memcache.md) / [redis.md](redis.md) - the distributed backends.
- [session.md](session.md) / [ratelimit.md](ratelimit.md) - the modules built on
  this selector.
- [modules/index.md](index.md) - the module catalog and import rules.

## Academy talks

| Talk | What it shows |
| ---- | ------------- |
| [Key-value stores: kv, Redis and memcached](https://academy.jennifer-lang.dev/talks/mplx/key-value/) | switching the backend under session and ratelimit |

## API reference

Every exported function, struct and constant of `kvstore`, with its signature, generated
from the module's doc comments: [the `kvstore` API reference](../api/kvstore.md).
