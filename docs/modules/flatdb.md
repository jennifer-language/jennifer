# `flatdb` - a file-backed JSON store

`import "flatdb.j" as flatdb;`

A small JSON document store: load a file once into a value-semantic handle,
query and edit it in memory through JSON Pointer, and write it back with a
crash-atomic whole-file replace. Built from `json` (the data) and `fs` (the
file); runs on either binary.

## What it is - and isn't

`flatdb` is **not a database engine**; it is *crash-atomic snapshotting of
small data*:

- **Atomicity** - whole-file, via a temp file + rename. A reader sees either the
  whole old file or the whole new one, never a torn write.
- **Consistency** - application-level (you decide what's valid).
- **Isolation** - optimistic, with an opt-in lock. Each writer reads the whole
  file; `save` **fails loudly** if it changed since `open` (no silent lost
  update), and `flatdb.update` adds a cross-process advisory lock so concurrent
  writers serialize with no loss. No multi-key transactions.
- **Durability** - the rename is atomic, but flush-to-disk is OS-buffered.

For a real database, use a client over `net` (e.g. [`redis`](redis.md)).
`flatdb` is for embedding a small store - config, a readable cache, a benchmark
history, a little app's saved state - where a single human-readable JSON file
fits.

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
a file. flatdb never imports `http` / `net` (it stays `fs`-only and builds on
both binaries), so you bring the bytes. To read a database published at a URL:

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
`append` / `remove`). After a `saveAs` you hold *both*; the next `save` writes
whichever handle you keep: reassign (`$db = flatdb.saveAs($db, path);`) to make
the new file current, or keep the old handle to keep writing the original file.
There is no hidden "active file" state.

Decoding untrusted input is safe against a nesting bomb: `json.decode` caps
container depth (a deeply-nested payload is a **catchable** error, not a fatal
stack overflow), and JSON has no entity/alias expansion to amplify size. Bound
the *download* itself with
`http`'s body cap (64 MiB by default; pass an explicit `maxBytes` to
`http.requestWith` for more or less).

## Concurrent writers

Crash-atomic is not write-safe: two writers that each `open`, edit different
parts, and `save` would clobber one change (last rename wins). `flatdb` handles
this two ways:

**`save` fails loudly on a conflict.** `open` records a token of the file's
on-disk state; `save` re-checks it and throws a `flatdb` error if the file moved
since, so a lost update is never *silent*. Catch it and re-open, or retry:

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
be broken after a stale window (120 s). Waiters that find the same stale lock
break it one at a time, under an exclusive `.lock.break` file, and remove the lock
only while it still holds the holder they judged stale - so a lock a faster waiter
has already re-taken is never removed. A waiter gives up after 5 s with a
`flatdb` error naming the current holder. A lock file that cannot be created at
all - a missing directory, a read-only mount, a data directory owned by another
user - has no holder to wait for, so `lock` / `update` fail **at once** with the
create's own error (`cannot create the lock file ...: permission denied`) instead
of reporting contention after the wait (a dangling symbolic link left at the
lock path is named as such). `flatdb.lock` / `flatdb.unlock` are the raw
primitives for a multi-step transaction, but prefer `update`, which releases on
every exit path.

A note on `save` returning a `DB`: `save` returns the document rebound to the
just-written file, so if you save the **same handle** more than once, thread the
return - `$db = flatdb.save($db);` - or the next `save` sees the file it wrote as
a conflict.

## Many readers: load once, share the parsed tree

An opened store is a parsed `json.Value` tree, which takes from about the
file's size (text-heavy data) up to roughly 15 times it (many small values) in
memory. A server should therefore **not** open the store in every request
handler (N requests in flight would hold N parsed copies). Open it **once** and
share it:

- **Read-only for the server's lifetime** - a top-level `def` is enough.
  `def store as flatdb.DB init flatdb.open("registry.json");` and handlers read
  `$store`. Passing the `DB` around, or capturing it in a `spawn`, copies only
  the small struct around the tree; the tree itself is immutable and shared.
- **Updated while serving** - publish each version through a `kv` store, which
  holds an opaque value without copying it, and rebuild the `DB` around it where
  you need one:

```jennifer
use kv;
import "flatdb.j" as flatdb;

def cache as kv.Store init kv.open();
def first as flatdb.DB init flatdb.open("registry.json");
kv.setValue($cache, "registry", $first.data, 0);

# Readers (any handler / task): the current tree, no parse, no copy.
func current() {
    return flatdb.DB{ path: "registry.json", data: kv.getValue($cache, "registry"), version: "" };
}

# The writer: update, save, and publish the new tree.
func publish(db as flatdb.DB) {
    def saved as flatdb.DB init flatdb.save($db);
    kv.setValue($cache, "registry", $saved.data, 0);
}
```

A reader `DB` built this way is for reading: its empty `version` makes a `save`
of it fail the conflict check, so writes go through a `DB` from `flatdb.open`
(or `flatdb.update`). See [`kv` values](../libraries/kv.md#values-shared-parsed-data).

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
