# `session` - server-side sessions

Import with `import "session.j" as session;`. Server-side sessions over a
**selectable** key/value backend: a session is a [`json.Value`](../libraries/json.md)
held under a `sess:ID` key with a sliding TTL, so it expires on its own when
idle. The store is a [`kvstore.Store`](kvstore.md) - back it with `memcache` or
`redis` (distributed, shared across processes) or the in-process
[`kv`](../libraries/kv.md) library (single process, no server), all behind one
API. Session values are a `json.Value`, so a session holds **structured** data
(nested objects, arrays, numbers), not just flat strings.

```jennifer
import "session.j" as session;
import "kvstore.j" as kvstore;
use json;

def store as kvstore.Store init kvstore.redisStore($rc);   # or memcacheStore / inProcessStore / fileStore
def id as string init session.create($store, 1800);        # a 30-minute session
def data as json.Value init session.load($store, $id);
$data = json.set($data, "/user", "ada");
$data = json.set($data, "/prefs/theme", "dark");           # nested is fine
session.save($store, $id, $data, 1800);
```

## Surface

| Call | Returns | |
| ---- | ------- | - |
| `session.create(store, ttl)` | `string` | mint a new session ID; store an empty session with a `ttl`-second expiry |
| `session.load(store, id)` | `json.Value` | the session's data, or an **empty object** when absent / expired |
| `session.save(store, id, data, ttl)` | | write the `json.Value` and re-arm the expiry |
| `session.touch(store, id, ttl)` | `bool` | re-arm the expiry without rewriting; whether it still existed |
| `session.destroy(store, id)` | `bool` | remove the session; whether it existed |

## Session IDs and storage

- **IDs are UUID v4** from the [`crypto`](../libraries/crypto.md) crypto-grade
  random source (122 unguessable bits), so an ID is safe to hand a client as the
  session's bearer token. An ID arriving from a client cookie is **validated** (1
  to 250 chars, `[A-Za-z0-9-]` only) before it reaches a store key, so it cannot
  inject a backend command.
- **Values are stored base64-wrapped JSON**, so any UTF-8 (or structured) value
  round-trips exactly, and the store only ever holds ASCII.

## Caveats

- **Volatile.** Sessions are a cache of soft state, not a store of record: a
  memcache / in-process backend evicts, so treat expiry as expected (re-auth, do
  not lose money on a dropped session).
- **Pick the backend for your topology.** More than one process (a web fleet)
  must see the same session -> use `redis` / `memcache`. A single-process app can
  use `inProcessStore` (in memory) or `fileStore` (persisted across runs).

## See also

- [kvstore.md](kvstore.md) - the backend selector; [kv.md](../libraries/kv.md) -
  the in-process backend.
- [json.md](../libraries/json.md) - the value type a session holds.
- [modules/index.md](index.md) - the module catalog and import rules.
