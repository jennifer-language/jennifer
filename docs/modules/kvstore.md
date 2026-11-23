# `kvstore` - selectable key/value backend

Import with `import "kvstore.j" as kvstore;`. A backend selector for a
key/value store with per-key TTL: one `Store` value is a `memcache` connection, a
`redis` connection, or the in-process [`kv`](../libraries/kv.md) library, and a
uniform `set` / `get` / `delete` / `touch` / `incrWindow` surface dispatches to
whichever you chose. It is the shared backend layer under
[`session`](session.md) and [`ratelimit`](ratelimit.md), so each works over any
backend.

```jennifer
import "kvstore.j" as kvstore;

# pick one backend:
def store as kvstore.Store init kvstore.redisStore($rc);         # distributed (redis)
# def store as kvstore.Store init kvstore.memcacheStore($mc);    # distributed (memcache)
# def store as kvstore.Store init kvstore.inProcessStore();      # local, in memory
# def store as kvstore.Store init kvstore.fileStore("state.kv"); # local, persisted

kvstore.set($store, "greeting", "hello", 60);   # expires in 60 s
io.printf("%s\n", kvstore.get($store, "greeting"));
```

Runnable: [`examples/modules/kvstore_demo.j`](https://github.com/jennifer-language/jennifer/blob/main/examples/modules/kvstore_demo.j) (in-process backend, so it needs no server).

## `Store` is an enum, not a tagged struct

`Store` is a sum type: each variant carries only its own backend handle, and
every op is a `match` the compiler checks for exhaustiveness:

```jennifer
export def enum Store {
    Memcache { mc as memcache.Session },
    Redis { rc as redis.Session },
    Local { local as kv.Store }
};
```

## Surface

| Call | Backend | |
| ---- | ------- | - |
| `kvstore.memcacheStore(mc)` | memcache | distributed; state on the server |
| `kvstore.redisStore(rc)` | redis | distributed; state on the server |
| `kvstore.inProcessStore()` | local | in memory, this process only (reset each run) |
| `kvstore.fileStore(path)` | local | persisted to `path`, survives across runs |
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
