# `session` - server-side sessions

Import with `import "session.j" as session;`. Server-side sessions over a
**selectable** key/value backend: a session is a [`json.Value`](../libraries/json.md)
held under a `sess:ID` key with a sliding TTL, so it expires on its own when
idle. The store is a [`kvstore.Store`](kvstore.md) - back it with `memcache` or
`redis` (distributed, shared across processes) or the in-process
[`kv`](../libraries/kv.md) library (single process, no server), all behind one
API. A session can hold structured data (nested objects, arrays, numbers), not
only flat strings.

```jennifer
import "session.j" as session;
import "kvstore.j" as kvstore;
use json;

def store as kvstore.Store init kvstore.inProcessStore();  # or redisStore / memcacheStore / fileStore
def id as string init session.create($store, 1800);        # a 30-minute session
def data as json.Value init session.load($store, $id);
$data = json.set($data, "/user", "ada");
$data = json.set($data, "/prefs", json.map());
$data = json.set($data, "/prefs/theme", "dark");           # nested is fine
session.save($store, $id, $data, 1800);                    # false once destroyed
```

## Surface

| Call | Returns | |
| ---- | ------- | - |
| `session.create(store, ttlSec)` | `string` | mint a new session ID; store an empty session with a `ttlSec`-second expiry |
| `session.load(store, id)` | `json.Value` | the session's data, or an **empty object** when absent / expired |
| `session.save(store, id, data, ttlSec)` | `bool` | write the `json.Value` and re-arm the expiry while the session exists; `false` (nothing written) when it was destroyed, rotated away, expired or never created |
| `session.touch(store, id, ttlSec)` | `bool` | re-arm the expiry without rewriting; whether it still existed |
| `session.destroy(store, id)` | `bool` | remove the session; whether it existed |
| `session.exists(store, id)` | `bool` | whether the session exists (not expired or destroyed) |
| `session.rotate(store, id, ttlSec)` | `string` | move the data to a fresh ID, destroy the old one; the new ID |

`ttlSec` is in seconds and must be >= 1 (kvstore reads <= 0 as "no expiry");
`create`, `save`, `touch` and `rotate` throw kind `session` otherwise. Stored
data that does not decode throws kind `session` (kind `limit` when it is past
`json`'s decoding caps).

**Only live sessions are saved.** `save` writes only while the session exists,
so a request still in flight when another one logs the user out (`destroy`) or
rotates the ID cannot write the old ID back to life, and an ID taken from a
request is never adopted. Mint sessions with `create`; when `save` returns
`false`, the session is gone and the request's changes are dropped. `save`
re-arms the expiry and then writes, two store calls: a `destroy` that lands
between the two still loses to the write. kvstore has no write-if-present call
(redis `SET XX`, memcache `replace`) that would close this window on every
backend.

**Fixation.** Call `rotate` whenever the privilege level changes (login,
logout), so an ID known before the change is worthless after it.

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

## API reference

Every exported function, struct and constant of `session`, with its signature, generated
from the module's doc comments: [the `session` API reference](../api/session.md).
