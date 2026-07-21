# `fs` - filesystem I/O

Enable with `use fs;`. Blocking whole-file reads and writes,
filesystem metadata, directory operations, and buffered file
handles for line-oriented reads. Non-blocking use composes with
[`spawn`](../user-guide/concurrency.md) rather than
duplicating each call as a `*Async` variant.

```jennifer
use io;
use fs;

fs.writeString("hello.txt", "hi, world\n");
def content as string init fs.readString("hello.txt");
io.printf("%s", $content);
```

## One-shot operations

Whole-file reads and writes. Cheap to write, cheap to read.

| Call                                  | Returns  | Notes                                                                       |
| ------------------------------------- | -------- | --------------------------------------------------------------------------- |
| `fs.readString(path)`                 | `string` | Whole file as UTF-8. Invalid UTF-8 is a positioned runtime error.           |
| `fs.readBytes(path)`                  | `bytes`  | Whole file as raw bytes. See below for the handle form.                     |
| `fs.writeString(path, content)`       | `null`   | Overwrites. Creates the file if missing. See below for the handle form.     |
| `fs.writeBytes(path, content)`        | `null`   | Overwrites. Creates the file if missing. See below for the handle form.     |
| `fs.appendString(path, content)`      | `null`   | Appends. Creates the file if missing.                                       |
| `fs.appendBytes(path, content)`       | `null`   | Appends. Creates the file if missing.                                       |

Reading a file that doesn't exist is a positioned runtime error;
use `fs.exists(path)` first when the "missing file is normal"
case matters.

## Metadata

Boundary-friendly predicates plus a full `fs.stat`.

| Call                | Returns   | Notes                                                                        |
| ------------------- | --------- | ---------------------------------------------------------------------------- |
| `fs.exists(path)`   | `bool`    | True if the path resolves. Permission errors still surface.                  |
| `fs.isFile(path)`   | `bool`    | True iff the path exists and is a regular file. False for missing.           |
| `fs.isDir(path)`    | `bool`    | True iff the path exists and is a directory. False for missing.              |
| `fs.stat(path)`     | `fs.Stat` | Missing path errors. Pair with `fs.exists` when tolerating absent files.     |

### `fs.Stat`

```jennifer
def struct fs.Stat {
    path as string,        # the path as passed to fs.stat / fs.walk
    size as int,           # bytes; -1 for directories
    isDir as bool,
    mtimeNanos as int,     # Unix nanoseconds
    mode as int            # POSIX permission bits (e.g. 0o644, 0o755)
};
```

`mtimeNanos` is deliberately an `int`, not a `time.Time` - that
keeps `fs` decoupled from the `time` library at the Go-package
level. Composition is one line:

```jennifer
use fs;
use time;

def stat as fs.Stat init fs.stat("hello.txt");
def modified as time.Time init time.fromUnixNanos($stat.mtimeNanos);
io.printf("modified: %s\n", time.iso($modified));
```

`size` is `-1` for directories so callers don't accidentally
interpret it as "empty directory."

### Permissions

The write verbs create files at a fixed `0644`; `fs.chmod` tightens (or
loosens) a file after the fact - e.g. to `0600` for a secret / token store
that must not be world-readable. `fs.chown` sets owner and group.

| Call                     | Returns | Notes                                                                                         |
| ------------------------ | ------- | --------------------------------------------------------------------------------------------- |
| `fs.chmod(path, mode)`   | `null`  | Sets the permission bits; `mode` is the low 12 bits (e.g. `0o600`, `0o755`), rejected outside `[0, 0o7777]`. |
| `fs.chown(path, uid, gid)` | `null` | Sets owner / group; `-1` leaves that id unchanged. Usually needs privilege.                   |

```jennifer
use fs;
fs.writeString("token.json", $json);
fs.chmod("token.json", 0o600);   # owner-only, like a gh / aws token store
```

Both are **Linux/Unix semantics** and match the platform target. On Windows
`fs.chmod` honours only the read-only bit and `fs.chown` always fails; the
error is catchable like any other `fs` failure.

## Directory operations

The library ships **two verbs each** for create and delete
(`mkdir` / `mkdirAll`, `remove` / `removeAll`). The safe
non-recursive form keeps the short name; the recursive form
gets an explicit second name so a code review can grep for
the risky sites. This is Jennifer's "no footguns" stance
applied at the API level - a `bool` parameter or a mode
string would obscure the recursive intent at the call site.

| Call                    | Returns           | Notes                                                          |
| ----------------------- | ----------------- | -------------------------------------------------------------- |
| `fs.mkdir(path)`        | `null`            | Errors if any parent is missing (matches POSIX `mkdir`).       |
| `fs.mkdirAll(path)`     | `null`            | Creates every missing parent (matches POSIX `mkdir -p`).       |
| `fs.remove(path)`       | `null`            | Removes one file or one empty directory. Non-empty dir errors. |
| `fs.removeAll(path)`    | `null`            | Recursive delete. Explicit second verb.                        |
| `fs.rename(old, new)`   | `null`            | Same-filesystem rename; cross-fs is a boundary error.          |
| `fs.list(path)`         | `list of string`  | Sorted entry names. Non-recursive.                             |
| `fs.walk(path)`         | `list of fs.Stat` | Depth-first, sorted, includes `path` itself as the first entry. Skips symlinks. |

```jennifer
use fs;

# Safe: mkdir refuses to create with missing parents.
fs.mkdirAll("build/output/cache");        # explicit intent

# Safe: remove refuses to delete non-empty directories.
fs.removeAll("build/output/cache");       # explicit intent
```

## Temporary files and directories

`fs.makeTempFile` and `fs.makeTempDir` create a fresh, uniquely-named entry and
return its **path**. Creation is atomic - the OS reserves the name, so two
concurrent callers never collide - and restrictive (`0600` for a file, `0700`
for a directory), so scratch data holding secrets is not world-readable. They
return a path (not an open handle) so the result drops straight into the ordinary
path-based verbs.

| Call | Returns | Notes |
| ---- | ------- | ----- |
| `fs.makeTempFile([dir[, prefix[, suffix]]])` | `string` | Create an empty unique file; returns its path. |
| `fs.makeTempDir([dir[, prefix]])` | `string` | Create a unique directory; returns its path. |

Arguments are positional and trailing-optional, `""` meaning the default:

- `dir` - where to create it. `""` (or omitted) uses the system temp directory
  (`os.tempDir()` - `$TMPDIR` else `/tmp` on Linux). An explicit `dir` **must
  already exist**: only the final unique component is created (like `fs.mkdir`,
  not `fs.mkdirAll`), so run `fs.mkdirAll` first if the parent is missing.
- `prefix` / `suffix` - name the entry around the random component; `suffix`
  gives a file a real extension. Neither may contain a path separator or NUL
  (that would escape the target directory), which is a positioned error.

```jennifer
use fs;

# A scratch file in the system temp dir, cleaned up when done.
def report as string init fs.makeTempFile("", "report-", ".json");
defer fs.remove($report);              # cleanup runs however the block exits
fs.writeString($report, $payload);
# ... use it ...

# A scratch directory under an existing build tree (create the parent first).
fs.mkdirAll("build");
def work as string init fs.makeTempDir("build", "job-");
defer fs.removeAll($work);
# ... write files under $work ...
```

Both are **strict**: any OS failure - a read-only filesystem, no permission, a
missing parent directory, or a name the filesystem cannot hold - is a catchable
error, never a fabricated directory tree or a name mangled to fit. Nothing is
cleaned up automatically: pair every `makeTemp*` with an `fs.remove` /
`fs.removeAll`, ideally as a `defer` right after the create (as above) so the
cleanup also runs when an error unwinds through the block
([control-flow > `defer`](../user-guide/control-flow.md#defer-deterministic-cleanup);
see the concurrency and error notes below).

## File handles

For line-oriented reads and files that don't fit comfortably in
memory, `fs.open` returns an `fs.File` handle backed by the
integer-registry pattern also used by `hash.Stream` and
`crc.Stream`.

```jennifer
def struct fs.File { id as int };
```

Mode strings use the codec-table shape:

| Mode        | Semantics                                                          |
| ----------- | ------------------------------------------------------------------ |
| `"read"`    | Read-only. `fs.readLine`, `fs.readChars`, `fs.readBytes` allowed. |
| `"write"`   | Write, create+truncate. `fs.writeString`, `fs.writeBytes` allowed. |
| `"append"`  | Write, create+append. Same write ops as `"write"`.                 |

Unknown mode strings error with the known set listed.

### Handle surface

| Call                       | Returns    | Notes                                                                     |
| -------------------------- | ---------- | ------------------------------------------------------------------------- |
| `fs.open(path, mode)`      | `fs.File`  | Opens per the mode string.                                                |
| `fs.close($f)`             | `null`     | Removes the handle from the registry; later ops on the id error.         |
| `fs.readLine($f)`          | `string`   | One line; `\r\n` / `\n` stripped. Errors on EOF - check `fs.eof` first.  |
| `fs.readChars($f, n)`      | `string`   | Up to `n` runes, UTF-8 decoded. Partial result on EOF, sticky-EOF flip.  |
| `fs.readBytes($f, n)`      | `bytes`    | Up to `n` bytes. Partial result on EOF, sticky-EOF flip.                 |
| `fs.writeString($f, s)`    | `null`     | Read-mode handle errors.                                                  |
| `fs.writeBytes($f, b)`     | `null`     | Read-mode handle errors.                                                  |
| `fs.sync($f)`              | `null`     | Flush written data to the storage device (fsync); handle stays open. Read-mode handle errors. |
| `fs.eof($f)`               | `bool`     | Looks ahead: true iff the next read would error / return partial.        |

The canonical read-loop:

```jennifer
use io;
use fs;

def f as fs.File init fs.open("input.txt", "read");
while (not fs.eof($f)) {
    def line as string init fs.readLine($f);
    io.printf("[%s]\n", $line);
}
fs.close($f);
```

`fs.readLine` on a handle whose next read is EOF errors with
`end of input`; the `fs.eof($f)` guard is what keeps the loop
tight. `fs.eof` looks one byte ahead (through the buffered
reader) so a file ending cleanly with `\n` still trips the
guard after the last line comes out.

**Durability: `sync` vs `close`.** `fs.close` guarantees the bytes reach the
*operating system*, not the *disk* - the OS may still hold them in its cache
after close returns. `fs.sync($f)` forces the file's written data all the way to
the storage device (an `fsync`) and leaves the handle open. This is the "safe to
remove the USB stick" step: sync explicitly and let a failure surface *before*
you tell the user it is safe, rather than discovering the write never landed:

```jennifer
use fs;
use io;

def f as fs.File init fs.open("/media/usb/report.csv", "write");
fs.writeString($f, $payload);
fs.sync($f);                           # push it to the device; errors if the write failed
io.printf("safe to remove the stick\n");
fs.close($f);
```

`fs.sync` requires a write- or append-mode handle (a read handle has nothing to
flush). Note `fsync` covers the OS side; a removable device's own internal cache
is the job of the OS eject / unmount.

### Polymorphic verbs (path vs handle)

Three verbs are polymorphic on the first argument's kind:

| Verb              | 1-arg form        | 2-arg form                    |
| ----------------- | ----------------- | ----------------------------- |
| `fs.readBytes`    | `(path)` -> whole | `($f, n)` -> partial handle read |
| `fs.writeString`  | -                 | `(path, content)` or `($f, s)` |
| `fs.writeBytes`   | -                 | `(path, content)` or `($f, b)` |

The dispatcher picks based on whether the first argument is a
`string` (path form) or an `fs.File` (handle form). Any other
kind is a positioned boundary error.

### Handles share state between copies

An `fs.File{id}` value is small; copies share the underlying
Go `*os.File` state via the integer id. This mirrors the
`task of T` carve-out to the "value semantics everywhere"
rule:

```jennifer
def a as fs.File init fs.open("x.txt", "read");
def b as fs.File init $a;             # `b` and `a` reference the same file
fs.close($a);                          # closes for both
def s as string init fs.readLine($b);  # errors: id is not open
```

Handles are the second "handles wrap shared state" carve-out
in the language, sitting alongside `task of T`. Every other
type keeps whole-value semantics.

## Concurrency composition

`fs` is blocking on purpose. Non-blocking use is a one-line
composition with `spawn`:

```jennifer
use fs;
use task;

def t as task of string init spawn {
    return fs.readString("/etc/hosts");
};
def content as string init task.wait($t);
```

Multiple files in parallel:

```jennifer
use fs;
use task;

func loadOne(path as string) {
    return fs.readString($path);
}

def paths as list of string init ["a.txt", "b.txt", "c.txt"];
def tasks as list of task of string init [];
for (def p in $paths) {
    $tasks[] = spawn { return loadOne($p); };
}
def contents as list of string init task.waitAll($tasks);
```

Under the default `jennifer` binary this actually parallelises across
cores; under `jennifer-tiny` (TinyGo, cooperative single-threaded
0.41) the composition is correct but sequential. See
[../technical/tinygo.md](../technical/tinygo.md).

## Errors

Every error is positioned at the Jennifer call site with the
path or handle id in the message.

- **Missing files** on `fs.readString` / `fs.readBytes` /
  `fs.stat`: `fs.readString: PATH: open PATH: no such file or directory`.
- **Non-empty directory** given to `fs.remove`:
  `fs.remove: PATH: directory not empty`. Use `fs.removeAll` for
  recursive delete.
- **Missing parent** given to `fs.mkdir`:
  `fs.mkdir: PATH: no such file or directory`. Use `fs.mkdirAll`
  for `mkdir -p`.
- **Wrong mode for op**: `fs.readLine: fs.File "x.txt" was opened
  in mode "write"; open with mode "read" to read`. Same message
  shape in reverse for read-mode handles given a write op.
- **Unknown mode string**: `fs.open: unknown mode "rw"; known:
  "read", "write", "append"`.
- **Use after close**: `fs.readLine: fs.File id 3 is not open
  (already closed, or never opened)`.
- **Non-negative int required**: `fs.readChars: n must be
  non-negative, got -1`.

Every error is catchable with `try` / `catch`:

```jennifer
try {
    def s as string init fs.readString("optional-config.toml");
} catch (e) {
    io.printf("no config, using defaults: %s\n", $e.message);
}
```

## What's not in v1

Recorded so the design decisions stay visible; ships if a
concrete workload forces it.

- **Streaming line iterator** (`for (def line in fs.lines(path))`).
  Compose with `fs.open` + `while (not fs.eof)`.
- **`fs.copy(src, dst)`** and **`fs.chmod(path, mode)`**.
- **Symlink ops** (`fs.readlink`, `fs.symlink`).
- **`fs.stat($f)` on an open handle**. Only path-based `fs.stat`
  in v1.
- **Watch / notify** (inotify / kqueue / FSEvents).
- **Temp file / dir creation helpers** (`fs.tempFile`,
  `fs.tempDir`). Use `os.getEnv("TMPDIR")` plus your own naming.
- **Follow symlinks in `fs.walk`**. Symlink-loop protection is
  the reason for the deferral; a `follow=true` flag or a
  distinct `fs.walkFollowing` verb ships once the story is clear.

## See also

- [../user-guide/concurrency.md](../user-guide/concurrency.md) -
  the `spawn`-and-compose story `fs` builds on.
- [`task`](task.md) - observe handles produced by `spawn`.
- [`time`](time.md) - `time.fromUnixNanos` converts
  `fs.Stat.mtimeNanos` into a `time.Time`.
- [`os`](os.md) - process-level operations that pair with `fs`:
  env vars, argv, external commands.
- [../milestones.md](../milestones.md) - design spec.
