# `kv` - in-process key/value store

Enable with `use kv;`. An in-process key/value store with per-key **TTL** - the
local, no-server counterpart to the `memcache` / `redis` clients. Open a store,
then set / get / delete / touch / increment; values expire on their own. A store
is a handle (`kv.Store`) into a run-scoped registry, so a `kv.Store` value
**shares its backing map across copies, across `spawn`ed tasks, and across a
module boundary** - the shared mutable state a pure `.j` module cannot hold
itself. A store opened in your program is usable from a module you `import` (pass
the handle in), and one a module hands back is usable in your program: the
registry is shared by every interpreter in a run and minted fresh each run. This
is what backs the
in-process option of the [`kvstore`](../modules/kvstore.md) backend selector (and
so [`session`](../modules/session.md) / [`ratelimit`](../modules/ratelimit.md)).
Pure Go stdlib, so it is on **both** binaries.

```jennifer
use kv;

def store as kv.Store init kv.open();
kv.set($store, "greeting", "hello", 60);   # expires in 60 s
io.printf("%s\n", kv.get($store, "greeting"));
```

## Surface

| Call | Returns | |
| ---- | ------- | - |
| `kv.open()` | `kv.Store` | a fresh, empty in-memory store (reset each run) |
| `kv.openFile(path)` | `kv.Store` | a store **persisted to `path`** - loaded on open, rewritten after every mutation, so it survives across `jennifer run` invocations |
| `kv.set(store, key, value, ttl)` | | store `value`, expiring in `ttl` seconds (`0` = never) |
| `kv.add(store, key, value, ttl)` | `bool` | store only if the key is absent; whether it stored |
| `kv.get(store, key)` | `string` | the value, or `""` when absent / expired |
| `kv.has(store, key)` | `bool` | whether the key is present and unexpired (tells `""` from absent) |
| `kv.delete(store, key)` | `bool` | remove the key; whether it existed |
| `kv.touch(store, key, ttl)` | `bool` | re-arm the key's expiry; whether it existed |
| `kv.incr(store, key, delta)` | `int` | add `delta` (signed - **negative decrements**, no separate `decr`) to the numeric value; the new value, or `-1` when the key is absent (it is **not** created) |
| `kv.close(store)` | | drop the store and free its handle |

## Semantics

- **TTL: lazy + periodic sweep.** An expired entry is evicted on the next access
  to that key, **and** a full sweep of expired entries runs periodically (every
  few hundred mutations). Without the sweep, a flood of distinct short-lived keys
  that are never accessed again (a rate limiter keyed by an untrusted IP) would
  pile up expired entries forever; the sweep bounds memory to the *unexpired*
  working set.
- **No hard size cap on live data.** There is no ceiling on *unexpired* entries -
  storing unbounded live data OOMs the process, exactly as an unbounded `list`
  would. That is the program's own responsibility. For an **adversarial or
  unbounded working set**, use `memcache` / `redis` (a server-side `maxmemory`
  eviction policy) via the [`kvstore`](../modules/kvstore.md) selector; `close`
  frees the whole store.
- **`incr` mirrors `memcache`.** It does not create the key (returns `-1` when
  absent) and errors on a non-numeric value, so it composes into the same
  increment-then-add window pattern the distributed backends use. `delta` is
  **signed** - `kv.incr($s, $k, -1)` decrements, so there is no separate `decr`
  (one verb, both directions). Unlike memcached's `DECR` it does **not** floor at
  0; clamp yourself if you need a non-negative counter.
- **Handles are shared, values are copies.** The `kv.Store` handle shares one
  backing map (that is the point - it survives value-copies, `spawn`, and being
  passed into or out of an `import`ed module); the string values it stores are
  ordinary copies. The sharing is run-scoped: the registry is created fresh each
  run and never leaks into the next.
- **Single process.** State lives in this process only. `open` is in-memory
  (gone when the program exits); `openFile` persists to disk and survives across
  runs (a rewrite-the-whole-file flush after every mutation - simple and correct,
  not fast). Both are safe for `spawn`ed tasks within one process (a per-store
  mutex; `incr` is atomic). Concurrent *separate* processes on one file get
  **last-write-wins** - a lost write, never a crash or a torn file (the flush
  renames a uniquely-named temp over the target). Note `add`'s test-and-set checks
  the in-memory snapshot loaded at open, so `openFile` + `add` is **not** a
  cross-process lock. For state shared across processes (multiple workers, a web
  fleet), use `memcache` / `redis` - the [`kvstore`](../modules/kvstore.md)
  selector switches backends behind one API.

## See also

- [kvstore.md](../modules/kvstore.md) - the backend selector that puts `kv`,
  `memcache`, and `redis` behind one interface.
- [memcache.md](../modules/memcache.md) / [redis.md](../modules/redis.md) - the
  distributed stores with the same verb shape.
- [libraries/index.md](index.md) - the library catalog.
