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
| `kv.setValue(store, key, value, ttl)` | | store an **opaque library value** (`json.Value`, `toml.Value`, `yaml.Value`, `xml.Value`, ...) - shared, not copied; in-memory stores only (see [Values](#values-shared-parsed-data)) |
| `kv.getValue(store, key)` | the stored value | the value `setValue` stored, shared with every other reader; an error when absent / expired (check `kv.has` first) |
| `kv.close(store)` | | drop the store and free its handle |

## Semantics

- **TTL: lazy + periodic sweep.** An expired entry is evicted on the next access
  to that key, **and** a full sweep of expired entries runs periodically (every
  few hundred mutations). Without the sweep, a flood of distinct short-lived keys
  that are never accessed again (a rate limiter keyed by an untrusted IP) would
  pile up expired entries forever; the sweep bounds memory to the *unexpired*
  working set. A TTL so large it would overflow an internal duration is treated
  as no expiry (like `0`), never as an immediate expiry.
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
  ordinary copies, and an opaque value stored with `setValue` is immutable, so
  sharing it is indistinguishable from copying it. The sharing is run-scoped: the registry is created fresh each
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

## Values: shared parsed data

`kv.set` / `kv.get` move strings. `kv.setValue` / `kv.getValue` move an **opaque
library value** - a parsed `json.Value` and its kin (`toml.Value`, `yaml.Value`,
`xml.Value`, `asn1.Value`). Those values are immutable (every edit returns a new
handle), so the store keeps the one parsed tree and every `getValue` hands out
that same tree: **nothing is copied**, however large the document and however
many tasks read it at once. That makes a `kv` store the place to publish a
parsed document that concurrent workers - `web` handlers, `spawn`ed tasks - read
and an occasional writer replaces:

```jennifer
use fs;
use json;
use kv;

def cache as kv.Store init kv.open();
kv.setValue($cache, "registry", json.decode(fs.readString("registry.json")), 0);

# Any worker, any time - no parse, no copy:
def doc as json.Value init kv.getValue($cache, "registry");

# A writer publishes a new version; readers already holding the old one keep it,
# the next getValue sees the new one.
kv.setValue($cache, "registry", json.set($doc, "/updated", json.decode("true")), 0);
```

- **Only opaque values.** A list, map or struct would have to be deep-copied on
  the way in and out to keep value semantics, which is the cost this avoids;
  `setValue` refuses one (store text with `kv.set`, or wrap the data in a
  `json.Value`).
- **Strings and values stay apart.** `kv.get` on a value entry, `kv.getValue` on
  a string entry, and `kv.incr` on a value entry are errors. `has`, `delete` and
  `touch` work on both, and either setter replaces whatever the key held.
- **In-memory stores only.** A `kv.openFile` store persists text, so `setValue`
  on one is an error.
- **Absent is an error.** `getValue` has no empty value of an unknown type to
  return, so an absent or expired key throws a catchable error; check `kv.has`
  when absence is expected. TTLs apply as for strings.

## See also

- [kvstore.md](../modules/kvstore.md) - the backend selector that puts `kv`,
  `memcache`, and `redis` behind one interface.
- [memcache.md](../modules/memcache.md) / [redis.md](../modules/redis.md) - the
  distributed stores with the same verb shape.
- [libraries/index.md](index.md) - the library catalog.
