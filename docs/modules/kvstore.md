# `kvstore` - selectable key/value backend

Import with `import "kvstore.j" as kvstore;`. A **backend selector** for a
key/value store with per-key TTL: one `Store` value is a `memcache` connection, a
`redis` connection, or the in-process [`kv`](../libraries/kv.md) library, and a
uniform `set` / `get` / `delete` / `touch` / `incrWindow` surface dispatches to
whichever you chose. It is the shared backend layer under
[`session`](session.md) and [`ratelimit`](ratelimit.md), so each works over any
backend without duplicating the plumbing.

```jennifer
import "kvstore.j" as kvstore;

# pick one backend:
def store as kvstore.Store init kvstore.redisStore($rc);        # distributed (redis)
# def store as kvstore.Store init kvstore.memcacheStore($mc);  # distributed (memcache)
# def store as kvstore.Store init kvstore.inProcessStore();    # local, in memory
# def store as kvstore.Store init kvstore.fileStore("state.kv"); # local, persisted

kvstore.set($store, "greeting", "hello", 60);   # expires in 60 s
io.printf("%s\n", kvstore.get($store, "greeting"));
```

## `Store` is an enum, not a tagged struct

`Store` is a sum type - each variant carries only its own backend handle, and
every op is a `match` the compiler checks for **exhaustiveness**:

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
| `kvstore.set(store, key, value, ttl)` | | store with a `ttl`-second expiry (`0` = none) |
| `kvstore.get(store, key)` | | the value, or `""` when absent / expired |
| `kvstore.delete(store, key)` | | remove the key; `bool` (existed) |
| `kvstore.touch(store, key, ttl)` | | re-arm the expiry; `bool` (existed) |
| `kvstore.incrWindow(store, key, ttl)` | | atomic increment, creating the key at 1 with a `ttl` TTL on the first hit; the new value. The fixed-window rate-limit primitive, portable across all backends. |

## Choosing a backend

- **Distributed** (`memcache` / `redis`) - state lives on the server, shared
  across every process and machine that connects. The right choice for a web
  fleet, a worker pool, or anything where more than one process must agree.
- **Local in-memory** (`inProcessStore`) - no server, fastest, but gone when the
  program exits and invisible to other processes. Good for a single-process
  cache or a test. Expired entries are swept periodically, so a rate limiter over
  it stays bounded; but there is no cap on *unexpired* data, so an **adversarial,
  unbounded working set** (millions of live sessions from an attacker) belongs on
  a distributed backend with a server-side eviction policy, not here.
- **Local persisted** (`fileStore`) - no server, and it survives across
  `jennifer run` invocations (handy for development and CLI tools that keep
  state). Single-process: safe for `spawn` within one process, but concurrent
  *separate* processes on one file can lose writes - use a distributed backend
  for cross-process coordination.

## See also

- [kv.md](../libraries/kv.md) - the in-process store behind the local backend.
- [memcache.md](memcache.md) / [redis.md](redis.md) - the distributed backends.
- [session.md](session.md) / [ratelimit.md](ratelimit.md) - the modules built on
  this selector.
- [modules/index.md](index.md) - the module catalog and import rules.
