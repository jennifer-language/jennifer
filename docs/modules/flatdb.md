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
- **Isolation** - none. One process, reload-the-whole-file, no concurrent
  transactions. Single-writer by construction.
- **Durability** - the rename is atomic, but flush-to-disk is OS-buffered.

For a real database, reach for a client over `net` (e.g. [`redis`](redis.md)),
not this. `flatdb` is the "embed a small store" need - config, a cache you can
read, a benchmark history, a little app's saved state - where a single
human-readable JSON file is exactly right.

## Handle, not a connection

A module holds no mutable state and `spawn` deep-copies scope, so a store can't
be a shared open connection - it's a **value** you hold:

```jennifer
export def struct DB { path as string, data as json.Value };
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
| `flatdb.save(db)` | `null` | Write the document back to its own file, atomically (temp + rename). Throws for a read-only DB (no backing file). |
| `flatdb.saveAs(db, path)` | `DB` | Write the document to `path` and return a **fresh DB bound to `path`** (`db` unchanged). First dump for an `openString` DB, or a copy / new version of an on-disk one. |

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
