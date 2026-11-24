# `kv` - in-process key/value store

Enable with `use kv;`. An in-process key/value store with per-key **TTL**: the
local, no-server counterpart to the `memcache` / `redis` clients. Open a store,
then set / get / delete / touch / increment; values expire on their own. A store
is a handle (`kv.Store`) into a run-scoped registry, so a `kv.Store` value
**shares its backing map across copies, across `spawn`ed tasks, and across a
module boundary**: the shared mutable state a pure `.j` module cannot hold
itself. A store opened in your program is usable from a module you `import` (pass
the handle in), and vice versa: the registry is shared by every interpreter in a
run and minted fresh each run. It backs the in-process option of the
[`kvstore`](../modules/kvstore.md) backend selector (and so
[`session`](../modules/session.md) / [`ratelimit`](../modules/ratelimit.md)).
Pure Go stdlib, so it is on **both** binaries.

```jennifer
use io;
use kv;

def store as kv.Store init kv.open();
kv.set($store, "greeting", "hello", 60);   # expires in 60 s
io.printf("%s\n", kv.get($store, "greeting"));
```

## Surface

| Call | Returns | |
| ---- | ------- | - |
| `kv.open()` | `kv.Store` | a fresh, empty in-memory store (reset each run) |
| `kv.openFile(path)` | `kv.Store` | a store **persisted to `path`**: loaded on open, rewritten after every mutation, so it survives across `jennifer run` invocations |
| `kv.set(store, key, value, ttl)` | | store `value`, expiring in `ttl` seconds (`0` = never; a negative `ttl` is an error) |
| `kv.add(store, key, value, ttl)` | `bool` | store only if the key is absent; whether it stored |
| `kv.get(store, key)` | `string` | the value, or `""` when absent / expired |
| `kv.has(store, key)` | `bool` | whether the key is present and unexpired (tells `""` from absent) |
| `kv.keys(store[, prefix])` | `list of string` | the unexpired keys, sorted; only those starting with `prefix` when given. A snapshot: sweep a store whose keys you no longer track (a counter keyed by a vanished owner). O(n) in the store's size |
| `kv.delete(store, key)` | `bool` | remove the key; whether it existed |
| `kv.touch(store, key, ttl)` | `bool` | re-arm the key's expiry; whether it existed |
| `kv.incr(store, key, delta)` | `int` | add `delta` (signed: **negative decrements**, no separate `decr`) to the numeric value and return the new value; an absent or expired key starts at 0 and is created without expiry |
| `kv.setValue(store, key, value, ttl)` | | store an **opaque library value** (`json.Value`, `toml.Value`, `yaml.Value`, `xml.Value`, ...), shared, not copied; in-memory stores only (see [Values](#values-shared-parsed-data)) |
| `kv.getValue(store, key)` | the stored value | the value `setValue` stored, shared with every other reader; an error when absent / expired (check `kv.has` first) |
| `kv.close(store)` | | drop the handle; the store goes when its last handle is closed. A second `close` does nothing |

## Semantics

- **TTL: lazy + periodic sweep.** An expired entry is evicted on the next access
  to that key, **and** a full sweep of expired entries runs every 512 mutations. Without the sweep, a flood of distinct short-lived keys
  that are never accessed again (a rate limiter keyed by an untrusted IP) would
  pile up expired entries forever; the sweep bounds memory to the *unexpired*
  working set. A TTL so large it would overflow an internal duration is treated
  as no expiry (like `0`), never as an immediate expiry. A negative TTL is an
  error in every call that takes one.
- **No hard size cap on live data.** There is no ceiling on *unexpired* entries:
  storing unbounded live data runs the process out of memory, as an unbounded
  `list` would. For an **adversarial or
  unbounded working set**, use `memcache` / `redis` (a server-side `maxmemory`
  eviction policy) via the [`kvstore`](../modules/kvstore.md) selector; `close`
  frees the whole store.
- **`incr` creates an absent key at 0**, as Redis's `INCR`: `kv.incr($s, "n", 5)`
  on an absent key stores and returns 5, so a counter needs no `has` check and
  two tasks cannot race between a check and the increment. The created key has
  no expiry; for a counter with a TTL, `kv.add($s, $k, "0", ttl)` first (a
  no-op when the key exists), then `incr`, which keeps the expiry. A
  non-integer value throws. `delta` is **signed**: `kv.incr($s, $k, -1)`
  decrements, and -1 is an ordinary count. It does **not** floor at 0; clamp
  yourself if you need a non-negative counter.
- **Handles are shared, values are copies.** The `kv.Store` handle shares one
  backing map (across value-copies, `spawn`, and an `import`ed module boundary);
  the string values it stores are ordinary copies, and an opaque value stored
  with `setValue` is immutable, so sharing it is indistinguishable from copying
  it. The registry is created fresh each run and never leaks into the next.
- **Single process.** State lives in this process only. `open` is in-memory
  (gone when the program exits); `openFile` persists to disk and survives across
  runs. Each mutation rewrites and fsyncs the whole file (simple and correct,
  not fast), so cost grows with store size: about 0.3-0.4 ms per write at 2,000
  short keys on an SSD. A
  mutation whose flush fails raises an error and leaves the store
  unchanged in memory too, so memory and file never disagree (a delete that could
  not be written does not reappear after the next `openFile`). A new store file
  is created 0600; an existing file keeps its mode. A file that is not a kv
  store, or a damaged one (a malformed or unterminated line), fails to open
  with the file and line, instead of opening as an empty or partial store that
  the next write would save over. Opening a path that is already open in this
  run returns a handle to the same store (each `openFile` needs its own
  `close`), so the entry program and a module that open one file do not
  overwrite each other's writes. Both are safe for `spawn`ed tasks within one
  process (a per-store mutex; `incr` is atomic). Separate processes on one
  file never lose a write silently: see **Lost updates** below. `add`'s
  test-and-set checks the in-memory snapshot loaded at open, so `openFile` +
  `add` is **not** a cross-process lock. For state shared across processes
  (multiple workers, a web fleet), use `memcache` / `redis`; the
  [`kvstore`](../modules/kvstore.md) selector switches backends behind one API.

### Lost updates

A flush replaces the file only if it is still the version this store last
read (at `openFile`) or wrote (its previous flush), the rule `flatdb.save`
uses. Otherwise the mutation throws kind `"kv"`, the file is left as it is and
the store's memory is unchanged.

| Step | Detail |
|---|---|
| Version | whether the file exists, and the SHA-256 of its content |
| Check | under the write lock `<path>.lock`, held only for the check and the rename, so no writer can replace the file in between |
| Refused | the file changed, was deleted, or was created by another writer since this store's last read or write |
| Recover | close every handle to the path and `openFile` again, which loads the other writer's content |
| Lock | a holder older than 10 s (a crashed writer) is broken; one held for 15 s throws `"timeout"` |


## Values: shared parsed data

`kv.set` / `kv.get` move strings. `kv.setValue` / `kv.getValue` move an **opaque
library value**: a parsed `json.Value` and its kin (`toml.Value`, `yaml.Value`,
`xml.Value`, `asn1.Value`). Those values are immutable (every edit returns a new
handle), so the store keeps the one parsed tree and every `getValue` hands out
that same tree: **nothing is copied**, however large the document and however
many tasks read it at once. That makes a `kv` store the place to publish a
parsed document that concurrent workers (`web` handlers, `spawn`ed tasks) read
and an occasional writer replaces:

```jennifer
use fs;
use json;
use kv;

def cache as kv.Store init kv.open();
kv.setValue($cache, "registry", json.decode(fs.readString("registry.json")), 0);

# Any worker, any time; no parse, no copy:
def doc as json.Value init kv.getValue($cache, "registry");

# A writer publishes a new version; readers already holding the old one keep it,
# the next getValue sees the new one.
kv.setValue($cache, "registry", json.set($doc, "/updated", json.decode("true")), 0);
```

| Rule | Detail |
| ---- | ------ |
| Only opaque values | A list, map or struct would have to be deep-copied on the way in and out to keep value semantics, which is the cost this avoids; `setValue` refuses one (store text with `kv.set`, or wrap the data in a `json.Value`). |
| Strings and values stay apart | `kv.get` on a value entry, `kv.getValue` on a string entry, and `kv.incr` on a value entry are errors. `has`, `delete` and `touch` work on both, and either setter replaces whatever the key held. |
| In-memory stores only | A `kv.openFile` store persists text, so `setValue` on one is an error. |
| Absent is an error | `getValue` has no empty value of an unknown type to return, so an absent or expired key throws a catchable error; check `kv.has` when absence is expected. TTLs apply as for strings. |

## Errors and messages

### Errors

| Kind | Raised for |
|---|---|
| `"kv"` | a `kv.openFile` file that is not a kv store or is damaged (`kv.openFile: /dir/x.kv:3: malformed entry (want "key value expiry")`, `kv.openFile: /dir/x.kv: the last line is not terminated (a truncated write?)`); the file is left as it is |
| `"kv"` | a mutation of a `kv.openFile` store whose file another writer changed, deleted or created since this store last read or wrote it (`kv.set: /dir/x.kv changed since this store last read or wrote it (another writer); ...`); nothing is written |
| `"timeout"` | a flush waited 15 s for another writer's lock (`<path>.lock`) |
| `"closed"` | any call but `close` on a closed handle (`kv.get: kv.Store is closed or invalid`) |
| `"runtime"` | everything else: misuse (below) and a file that cannot be read or written |

### Messages

Avoid the misuse cases before the call (`kv.has`, strings and values under
separate keys) rather than telling them apart by the message:

| Call | Raises `"runtime"` when |
| ---- | ----------------------- |
| `kv.set` / `add` / `setValue` / `touch` | the `ttl` is negative (`kv.set: ttl must be >= 0 (0 = no expiry), got -1`) |
| `kv.incr` | the value is not an integer (`kv.incr: value at "k" is not a number`), the sum overflows `int64`, or the key holds a `setValue` value. An absent key is no error: it starts at 0 |
| `kv.get` | the key holds a `setValue` value (an absent key is no error: `""`) |
| `kv.getValue` | the key is absent or expired, or holds a string |
| `kv.setValue` | the value is no opaque library value (a string, list, map or struct), or the store is a `kv.openFile` store |
| any mutation of a `kv.openFile` store | the flush fails (`kv.set: open /dir/x.kv.tmp...: no such file or directory`); the store is left unchanged |

```jennifer
use io;
use kv;

def s as kv.Store init kv.open();
kv.set($s, "greeting", "hello", 0);
try {
    kv.incr($s, "greeting", 1);
} catch (e) {
    io.printf("%s\n", $e.kind);   # runtime
}
```

## See also

- [kvstore.md](../modules/kvstore.md) - the backend selector that puts `kv`,
  `memcache`, and `redis` behind one interface.
- [memcache.md](../modules/memcache.md) / [redis.md](../modules/redis.md) - the
  distributed stores with the same verb shape.
- [libraries/index.md](index.md) - the library catalog.

## Academy talks

| Talk | What it shows |
| ---- | ------------- |
| [Key-value stores: kv, Redis and memcached](https://academy.jennifer-lang.dev/talks/mplx/key-value/) | an in-process store with TTLs shared across tasks |
