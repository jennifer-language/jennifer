# `flatdb` - a file-backed JSON store

`import "flatdb.j" as flatdb;`

A small JSON document store: load a file once into a value-semantic handle,
query and edit it in memory through JSON Pointer, and write it back with a
crash-atomic whole-file replace. Built from `json` (the data) and `fs` (the
file); runs on either binary.

## What it is - and isn't

`flatdb` is **not a database engine**. Honestly, it is *crash-atomic
snapshotting of small data*:

- **Atomicity** - whole-file, via a temp file + rename. A reader ever sees the
  whole old file or the whole new one, never a torn write.
- **Consistency** - application-level (you decide what's valid).
- **Isolation** - optimistic, with an opt-in lock. Each writer reads the whole
  file; `save` **fails loudly** if it changed since `open` (no silent lost
  update), and `flatdb.update` adds a cross-process advisory lock so concurrent
  writers serialize with no loss. No multi-key transactions.
- **Durability** - the rename is atomic, but flush-to-disk is OS-buffered.

For a real database, reach for a client over `net` (e.g. [`redis`](redis.md)),
not this. `flatdb` is the "embed a small store" need - config, a cache you can
read, a benchmark history, a little app's saved state - where a single
human-readable JSON file is exactly right.

## Handle, not a connection

A module holds no mutable state and `spawn` deep-copies scope, so a store can't
be a shared open connection - it's a **value** you hold:

```jennifer
export def struct DB { path as string, data as json.Value, version as string };
```

Reading verbs leave the `DB` untouched; writing verbs return a **fresh** `DB`
(thread it through, the same shape lists / maps / `json` use); `save` is the
only side effect.

## Surface

| Call | Returns | |
| ---- | ------- | - |
| `flatdb.open(path)` | `DB` | Load the file (an **empty** store if it's absent, so first run never fails). |
| `flatdb.openString(text)` | `DB` | Load from an in-memory JSON string - a **read-only** DB with no backing file (`save` throws). For a store fetched over the network or embedded in the program. |
| `flatdb.get(db, pointer)` | `json.Value` | The sub-document at a JSON Pointer (`""` = the whole document). |
| `flatdb.has(db, pointer)` | `bool` | Whether the pointer resolves. |
| `flatdb.keys(db, pointer)` | `list of string` | Keys of the object at the pointer, in document order. |
| `flatdb.length(db, pointer)` | `int` | Element / entry count at the pointer. |
| `flatdb.set(db, pointer, value)` | `DB` | Upsert an object key / replace a list index (strict: no auto-vivify). |
| `flatdb.append(db, pointer, value)` | `DB` | Push onto the list at the pointer (create it first with `set`). |
| `flatdb.remove(db, pointer)` | `DB` | Drop the key / element at the pointer. |
| `flatdb.save(db)` | `DB` | Write back atomically (temp + rename) and return a **rebound DB** (thread it - `$db = flatdb.save($db)` - to save again). Throws for a read-only DB, or a `flatdb` **conflict** if the file changed since `open`. |
| `flatdb.saveAs(db, path)` | `DB` | Write the document to `path` and return a **fresh DB bound to `path`** (`db` unchanged). First dump for an `openString` DB, or a copy / new version of an on-disk one. |
| `flatdb.update(path, transform)` | `DB` | **Locked** read-modify-write: take the lock, open the current doc, apply `transform` (a `func(db as DB) -> DB`), save, release (via `defer`). The safe path when concurrent writers may touch one store - no lost update. |
| `flatdb.lock(dbPath)` / `flatdb.unlock(lk)` | `Lock` / `null` | The raw advisory lock (cross-process, over `fs.writeNew`) behind `update`, for a multi-step transaction. Prefer `update`. |

`value` is any JSON value - a `json.Value`. Build objects and lists with
`json.map()` / `json.list()` (then `json.set` / `json.append` into them), and
scalars with `json.decode` (`json.decode("42")`, `json.decode("\"hi\"")`).
Addressing is [JSON Pointer](../libraries/json.md#json-pointer-rfc-6901),
identical to `json`'s.

## Example

```jennifer
use io;
use json;
import "flatdb.j" as flatdb;

def db as flatdb.DB init flatdb.open("state.json");   # empty on first run
$db = flatdb.set($db, "/runs", json.list());

def rec as json.Value init json.map();
$rec = json.set($rec, "/cpu", "Ryzen 5 7600X3D");
$rec = json.set($rec, "/ms", 118);
$db = flatdb.append($db, "/runs", $rec);

flatdb.save($db);                                     # atomic replace

def store as flatdb.DB init flatdb.open("state.json");
io.printf("%d runs; first on %s\n",
    flatdb.length($store, "/runs"),
    json.asString(flatdb.get($store, "/runs/0/cpu")));
```

A runnable version is [`examples/modules/flatdb_demo.j`](../../examples/modules/flatdb_demo.j).

## Read-only from a URL or a string

`flatdb.openString(text)` loads a store from an in-memory JSON string instead of
a file. flatdb stays transport-agnostic - it never imports `http` / `net`, so it
remains `fs`-only and builds on both binaries - and you bring the bytes. To read
a database published at a URL:

```jennifer
use io;
use json;
import "http.j" as http;
import "flatdb.j" as flatdb;

def resp as http.Response init http.get("https://example.com/config.json", {});
def db as flatdb.DB init flatdb.openString($resp.body);   # read-only
io.printf("theme: %s\n", json.asString(flatdb.get($db, "/ui/theme"), ""));
```

The result has no backing path, so every reader works and the mutating verbs
still return fresh in-memory copies, but `save` throws (`kind: "flatdb"`) - there
is nowhere local to write. To **persist** it (or fork any store to a new file),
use `saveAs`:

```jennifer
def db as flatdb.DB init flatdb.openString($resp.body);
def edited as flatdb.DB init flatdb.set($db, "/ui/theme", json.decode("\"dark\""));
def local as flatdb.DB init flatdb.saveAs($edited, "config.local.json");
flatdb.save($local);   # now writable - $local is bound to config.local.json
```

`saveAs(db, path)` writes to `path` and returns a **fresh DB bound to that
path**; the `db` you passed in is unchanged (value semantics, like `set` /
`append` / `remove`). So after a `saveAs` you hold *both* - which one is "current"
for the next `save` is simply the handle you keep: reassign
(`$db = flatdb.saveAs($db, path);`) to make the new file current, or keep the old
handle to keep writing the original file. There is no hidden "active file" state;
a `DB` is a value.

Decoding untrusted input is safe against a nesting bomb: `json.decode` caps
container depth (a deeply-nested payload is a **catchable** error, not a fatal
stack overflow), and JSON has no entity/alias expansion to amplify size - so a
malicious URL can't crash or blow up the process. Bound the *download* itself with
`http`'s body cap (64 MiB by default; pass an explicit `maxBytes` to
`http.requestWith` for more or less).

## Concurrent writers

Crash-atomic is **not** the same as write-safe. Two writers that each `open`,
edit different parts, and `save` would clobber one of the two changes (last
rename wins). `flatdb` closes that two ways:

**`save` fails loudly on a conflict.** `open` records a token of the file's
on-disk state; `save` re-checks it and throws a `flatdb` error if the file moved
since - so a lost update can never be *silent*. Catch it and re-open, or retry:

```jennifer
try {
    flatdb.save($db);
} catch (e) {
    # someone else wrote first - re-open, re-apply, retry
}
```

(This is best-effort - it rests on the filesystem's mtime granularity and has a
tiny check-then-rename window - so it makes loss *loud*, not impossible.)

**`flatdb.update` makes it impossible.** It brackets the whole read-modify-write
in a cross-process advisory lock (built on [`fs.writeNew`](../libraries/fs.md), an
atomic exclusive create), so concurrent writers serialize and no update is lost.
`transform` is a `func(db as DB) -> DB`:

```jennifer
func addVisit(db as flatdb.DB) {
    def n as int init 0;
    if (flatdb.has($db, "/visits")) { $n = json.asInt(flatdb.get($db, "/visits")); }
    return flatdb.set($db, "/visits", json.decode(convert.toString($n + 1)));
}
flatdb.update("state.json", addVisit);   # locked read -> edit -> save -> release
```

The lock coordinates across **separate processes** (a CLI and a server, two CLI
runs) as well as `spawn`ed tasks - a symlink-free lock file created with
`fs.writeNew`, whose holder id and timestamp let a lock leaked by a crashed writer
be broken after a stale window. `flatdb.lock` / `flatdb.unlock` are the raw
primitives for a multi-step transaction, but prefer `update`, which releases on
every exit path.

A note on `save` returning a `DB`: `save` returns the document rebound to the
just-written file, so if you save the **same handle** more than once, thread the
return - `$db = flatdb.save($db);` - or the next `save` sees the file it wrote as
a conflict.

## Atomic save, in detail

`save` writes the encoded document to a sibling `path + ".tmp"` and then
`fs.rename`s it over the target. On POSIX the rename is atomic, so a concurrent
reader never sees a half-written file. If the process dies mid-save (temp
written, rename not reached), the original file is untouched - only a stray
`.tmp` remains, which the next `save` overwrites. Durability past the rename is
the OS's call (there is no `fsync` today).

## See also

- [`json`](../libraries/json.md) - the value model and write surface `flatdb`
  layers over.
- [`fs`](../libraries/fs.md) - the file I/O (`readString` / `writeString` /
  `rename`) behind `open` / `save`.
- [`redis`](redis.md) - a real store, over the network, when you outgrow a
  single file.
