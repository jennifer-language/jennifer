# `archive` - tar / zip containers

Enable with `use archive;`. Bundle files into a tar or zip container and
read them back, entirely over `bytes` - no `fs` dependency,
value-semantic. Shares the `pack` / `unpack` verbs with
[`compress`](compress.md) (byte streams there, file bundles here); the
container format is a string argument. Backed by Go's archive/tar +
archive/zip; works on both binaries.

## Surface

| Call                            | Returns                 | Notes                                          |
| ------------------------------- | ----------------------- | ---------------------------------------------- |
| `archive.pack(entries, format)` | `bytes`                 | Bundle a `list of archive.Entry`.              |
| `archive.unpack(b, format)`     | `list of archive.Entry` | Read a bundle back (default caps).             |
| `archive.unpackWith(b, format, opts)` | `list of archive.Entry` | Read a bundle with caller-set caps (`archive.UnpackOptions`). |

`format` is `"tar"`, `"zip"`, or the gzip combo `"tar.gz"` (alias
`"tgz"`). An unknown format, or corrupt input to `unpack`, is a
positioned runtime error (catchable with `try` / `catch`).

`unpack` bounds untrusted input: the total decompressed payload of one
call (summed across every entry) is capped at 256 MiB and the member
count at 65536; past either cap it raises a catchable `Error{kind:
"limit"}` (resource exhaustion, distinct from a `"runtime"` bug)
instead of expanding a small "zip bomb" into gigabytes of memory. It
also rejects any member whose name is an absolute path or escapes with
`..` (a "zip-slip" name), so a naive extraction loop
(`fs.writeBytes($dir + "/" + $e.name, $e.data)`) can't be tricked into
writing outside the target directory.

### `archive.UnpackOptions`

`archive.unpackWith(b, format, opts)` is `unpack` with caller-set caps,
for an input whose trusted size differs from the defaults:

```jennifer
def opts as archive.UnpackOptions init archive.UnpackOptions{
    maxTotalBytes: 50 * 1024 * 1024,   # 50 MiB total across all entries
    maxEntryBytes: 10 * 1024 * 1024,   # 10 MiB per entry
    maxEntries: 1000
};
def entries as list of archive.Entry init archive.unpackWith($kmz, "zip", $opts);
```

Each field: **`0` takes the default** (so a bare `UnpackOptions{}` behaves
exactly like `unpack`), **a positive value** sets that cap, and **a
negative value disables it** (unlimited - for a fully trusted archive).
Exceeding any cap raises `Error{kind: "limit"}`, the same as `unpack`.

## `archive.Entry`

A struct `{ name as string, data as bytes, mode as int, mtime as int }`:

- **`name`** - the path within the archive (subdirectories with `/`).
- **`data`** - the file contents.
- **`mode`** - unix permission bits (e.g. `0o644`); `0` means the default `0o644`.
  On `unpack`, the special bits (setuid / setgid / sticky) and tar file-type bits
  are masked off to the permission bits, so an untrusted archive never yields a
  setuid `mode` an extraction loop would apply.
- **`mtime`** - modification time, unix seconds.

Only regular files map to an `Entry`; directory members are skipped on
`unpack`.

## Example

```jennifer
use io;
use archive;
use convert;

def readme as archive.Entry init archive.Entry{
    name: "README", data: convert.bytesFromString("hello", "utf-8"), mode: 0o644, mtime: 0
};
def blob as bytes init archive.pack([$readme], "tar.gz");   # one call: tar, then gzip
def back as list of archive.Entry init archive.unpack($blob, "tar.gz");
io.printf("%s = %s\n", $back[0].name, convert.stringFromBytes($back[0].data, "utf-8"));
```

`"tar.gz"` layers `compress`'s gzip over a tar internally, so the
everyday `.tar.gz` case is a single call rather than a `tar`-then-gzip
pair.

## See also

- [compress.md](compress.md) - `pack` / `unpack` for byte streams (`"gzip"` / `"zlib"` / `"deflate"`).
- [fs.md](fs.md) - write the packed `bytes` to disk, or read an archive off disk to unpack.
