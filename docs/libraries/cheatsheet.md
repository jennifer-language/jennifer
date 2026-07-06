# Cheatsheet - all builtins at a glance

Alphabetical index of every standard-library function and constant. Use
it when you know the *name* and want to know *which library* and *how to
call it*; use each library's own page when you want to read about a
topic. Each row's library prefix links to the per-library doc.

The table covers what ships with the interpreter today. New
entries land here at the same time as the per-library doc - it's a
flat lookup view, not authoritative.

## Functions

| Call                                                  | What it does                                                                                                                        |
| ----------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| [`convert`](convert.md)`.toBool(v)`                   | Canonical conversion to `bool` (`0`/`1`, `0.0`/`1.0`, `"true"`/`"false"`).                                                          |
| [`convert`](convert.md)`.toFloat(v)`                  | Convert to float (int→float, float identity, string parses, bool→1.0/0.0).                                                          |
| [`convert`](convert.md)`.toInt(v)`                    | Convert to int (float truncates toward zero, string parses, bool→1/0).                                                              |
| [`convert`](convert.md)`.toString(v)`                 | Convert to string (always succeeds; uses the value's display form).                                                                 |
| [`convert`](convert.md)`.typeOf(v)`                   | Runtime kind as string (`"int"`, `"float"`, `"string"`, `"bool"`, `"null"`, `"list"`, `"map"`).                                     |
| [`crc`](crc.md)`.compute(b, algo)`                    | One-shot checksum. `algo` is `"crc32"` or `"crc64"`. Returns big-endian bytes (4 or 8).                                             |
| [`crc`](crc.md)`.finalize($s)`                        | Final checksum as big-endian bytes; consumes the handle.                                                                            |
| [`crc`](crc.md)`.stream(algo)`                        | Allocate a `crc.Stream` for `algo`; feed chunks via `crc.update` then close with `crc.finalize`.                                    |
| [`crc`](crc.md)`.update($s, $bytes)`                  | Feed one chunk into a `crc.Stream` (mutates by side effect).                                                                        |
| [`encoding`](encoding.md)`.codecs()`                  | Canonical character-codec names in registration order.                                                                              |
| [`encoding`](encoding.md)`.decode(b, codec)`          | Decode `bytes` from a character codec to a Jennifer string.                                                                         |
| [`encoding`](encoding.md)`.encode(s, codec)`          | Encode a Jennifer string into a character codec's bytes.                                                                            |
| [`encoding`](encoding.md)`.fromText(s, format)`       | Decode a binary-to-text format. `format`: `"hex"`, `"base64"`, `"base64-url"`.                                                      |
| [`encoding`](encoding.md)`.isAscii(b)`                | True iff every byte in `b` is < 0x80.                                                                                               |
| [`encoding`](encoding.md)`.lenBytes(s)`               | UTF-8 byte length of `s` (pair with `len(s)` for rune count).                                                                       |
| [`encoding`](encoding.md)`.lenRunes(b)`               | Rune count of valid UTF-8 `bytes`; errors on invalid UTF-8.                                                                         |
| [`encoding`](encoding.md)`.toText(b, format)`         | Encode `bytes` as printable text. `format`: `"hex"`, `"base64"`, `"base64-url"`.                                                    |
| [`fs`](fs.md)`.appendBytes(path, content)`            | Append `bytes` to `path`; creates the file if missing.                                                                              |
| [`fs`](fs.md)`.appendString(path, content)`           | Append UTF-8 `string` to `path`; creates the file if missing.                                                                       |
| [`fs`](fs.md)`.close($f)`                             | Close an `fs.File` handle; removes it from the registry.                                                                            |
| [`fs`](fs.md)`.eof($f)`                               | True iff the next read on `$f` would error or return partial. Sticky.                                                               |
| [`fs`](fs.md)`.exists(path)`                          | True if `path` resolves; permission errors still surface.                                                                           |
| [`fs`](fs.md)`.isDir(path)`                           | True iff `path` exists and is a directory.                                                                                          |
| [`fs`](fs.md)`.isFile(path)`                          | True iff `path` exists and is a regular file.                                                                                       |
| [`fs`](fs.md)`.list(path)`                            | Sorted entry names in `path`. Non-recursive; returns `list of string`.                                                              |
| [`fs`](fs.md)`.mkdir(path)`                           | Create a single directory; errors if any parent is missing.                                                                         |
| [`fs`](fs.md)`.mkdirAll(path)`                        | Create `path` and every missing parent (like `mkdir -p`).                                                                           |
| [`fs`](fs.md)`.open(path, mode)`                      | Open `path` and return an `fs.File`. `mode`: `"read"`, `"write"`, `"append"`.                                                       |
| [`fs`](fs.md)`.readBytes(path)` / `.readBytes($f, n)` | Whole-file read (1 arg) or up to `n` bytes from handle (2 args). Partial + sticky-EOF on short handle reads.                        |
| [`fs`](fs.md)`.readChars($f, n)`                      | Up to `n` runes from handle, UTF-8 decoded. Partial + sticky-EOF on short reads.                                                    |
| [`fs`](fs.md)`.readLine($f)`                          | One line from handle, `\r\n` / `\n` stripped. Errors on EOF - check `fs.eof` first.                                                 |
| [`fs`](fs.md)`.readString(path)`                      | Whole file as UTF-8; invalid UTF-8 is a positioned runtime error.                                                                   |
| [`fs`](fs.md)`.remove(path)`                          | Delete one file or empty directory. Non-empty dir errors.                                                                           |
| [`fs`](fs.md)`.removeAll(path)`                       | Recursive delete. Explicit second verb (no-footguns stance).                                                                        |
| [`fs`](fs.md)`.rename(old, new)`                      | Same-filesystem rename; cross-fs is a boundary error.                                                                               |
| [`fs`](fs.md)`.stat(path)`                            | Returns `fs.Stat` (`path`, `size`, `isDir`, `mtimeNanos`, `mode`). Missing path errors.                                             |
| [`fs`](fs.md)`.walk(path)`                            | Depth-first, sorted, includes `path`. Returns `list of fs.Stat`. Skips symlinks.                                                    |
| [`fs`](fs.md)`.writeBytes(path, content)` / `.writeBytes($f, b)` | Whole-file overwrite (path form) or write via handle (fs.File form).                                                      |
| [`fs`](fs.md)`.writeString(path, content)` / `.writeString($f, s)` | Whole-file overwrite (path form) or write via handle (fs.File form).                                                    |
| [`hash`](hash.md)`.compute(b, algo)`                  | One-shot digest. `algo` is `"md5"`, `"sha1"`, or `"sha256"`. Returns raw bytes.                                                     |
| [`hash`](hash.md)`.finalize($s)`                      | Final digest as bytes; consumes the handle (later calls error).                                                                     |
| [`hash`](hash.md)`.stream(algo)`                      | Allocate a `hash.Stream` for `algo`; feed chunks via `hash.update` then close with `hash.finalize`.                                 |
| [`hash`](hash.md)`.update($s, $bytes)`                | Feed one chunk into a `hash.Stream` (mutates by side effect).                                                                       |
| [`io`](io.md)`.eof()`                                 | True if and only if the next `io.readLine()` would error. Pair with `while (not io.eof()) {...}`.                                   |
| [`io`](io.md)`.printf(format, args...)`               | Format-string write to stdout. Verbs: `%d %f %s %t %v %%`; per-verb `\|key=value` modifiers (`pad`, `prec`, `base`, `null=*`, ...). |
| [`io`](io.md)`.printf(value)`                         | Write a value's display form to stdout.                                                                                             |
| [`io`](io.md)`.readLine()`                            | Read one line from stdin (trailing newline stripped). Errors at EOF - check `io.eof()` first.                                       |
| [`io`](io.md)`.readLine(prompt)`                      | Same as `io.readLine()` but writes `prompt` to stdout first.                                                                        |
| [`io`](io.md)`.sprintf(format, args...)`              | Format-string version of `sprintf`. Same verbs and `\|key=value` modifiers as `printf`.                                             |
| [`io`](io.md)`.sprintf(value)`                        | Display-form of a value, returned as a string (doesn't write).                                                                      |
| `len(v)` *(language built-in)*                        | Structural length: rune count (string), element count (list), entry count (map), byte count (bytes).                                |
| [`lists`](lists.md)`.concat(a, b)`                    | New list with `a`'s elements followed by `b`'s.                                                                                     |
| [`lists`](lists.md)`.contains(xs, item)`              | True if `item` appears in `xs` (haystack, needle).                                                                                  |
| [`lists`](lists.md)`.first(xs)`                       | Element at index 0. Empty input errors.                                                                                             |
| [`lists`](lists.md)`.head(xs, n)`                     | New list of the first `n` elements.                                                                                                 |
| [`lists`](lists.md)`.last(xs)`                        | Element at the last index. Empty input errors.                                                                                      |
| [`lists`](lists.md)`.pop(xs)`                         | New list without the last element. Empty input errors.                                                                              |
| [`lists`](lists.md)`.push(xs, item)`                  | New list with `item` appended.                                                                                                      |
| [`lists`](lists.md)`.range(start, end[, step])`       | Half-open list of consecutive ints; `end` excluded; `step` must match direction.                                                    |
| [`lists`](lists.md)`.reverse(xs)`                     | New list with elements reversed.                                                                                                    |
| [`lists`](lists.md)`.shuffle(xs)`                     | Fisher-Yates; respects `math.randSeed`. Non-mutating.                                                                               |
| [`lists`](lists.md)`.slice(xs, start[, end])`         | New sublist `[start, end)`; `end` defaults to `len(xs)`.                                                                            |
| [`lists`](lists.md)`.sort(xs)`                        | New ascending-sorted list. Numeric / string / bool elements; mixed errors.                                                          |
| [`lists`](lists.md)`.tail(xs, n)`                     | New list of the last `n` elements.                                                                                                  |
| [`maps`](maps.md)`.delete(m, key)`                    | New map without `key`. Missing key errors (strict at boundaries).                                                                   |
| [`maps`](maps.md)`.has(m, key)`                       | True if map `m` contains `key`. The non-erroring companion to `$m[key]`.                                                            |
| [`maps`](maps.md)`.keys(m)`                           | List of keys in insertion order.                                                                                                    |
| [`maps`](maps.md)`.merge(a, b)`                       | New map; `b`'s entries layered on top of `a`.                                                                                       |
| [`maps`](maps.md)`.values(m)`                         | List of values in insertion order.                                                                                                  |
| [`math`](math.md)`.abs(x)`                            | Absolute value of `x` (int→int, float→float).                                                                                       |
| [`net`](net.md)`.accept($listener)`                   | Block until a client connects to `$listener`; return the new `net.Conn`.                                                            |
| [`net`](net.md)`.address($h)`                         | Polymorphic. Conn -> peer address; Listener / UDPSocket -> local bound address.                                                     |
| [`net`](net.md)`.close($h)`                           | Polymorphic. Closes a `net.Conn`, `net.Listener`, or `net.UDPSocket`.                                                               |
| [`net`](net.md)`.connect(address)`                    | TCP client: dial `"host:port"` and return a `net.Conn`.                                                                             |
| [`net`](net.md)`.eof($conn)`                          | True iff the next read on `$conn` would return partial or fail. Sticky.                                                             |
| [`net`](net.md)`.listen(address)`                     | Bind TCP `"host:port"` (use `":0"` for ephemeral). Returns a `net.Listener`.                                                        |
| [`net`](net.md)`.listenUDP(address)`                  | Bind a UDP socket. Returns a `net.UDPSocket`; usable as both client and server.                                                     |
| [`net`](net.md)`.lookup(host)`                        | DNS: resolve `host` to a `list of string` IPs.                                                                                      |
| [`net`](net.md)`.readBytes($conn, n)`                 | Read up to `n` bytes; blocks for at least one byte. Sticky-EOF on close.                                                            |
| [`net`](net.md)`.recvFrom($sock, n)`                  | Block for one UDP datagram, up to `n` bytes. Returns `net.Datagram{data, peer}`.                                                    |
| [`net`](net.md)`.reverseLookup(ip)`                   | Reverse DNS: IP address to a `list of string` of hostnames.                                                                         |
| [`net`](net.md)`.sendTo($sock, peer, bytes)`          | Send one UDP datagram to `peer` (`"host:port"`).                                                                                    |
| [`net`](net.md)`.writeBytes($conn, bytes)`            | Blocking write of every byte to a `net.Conn`.                                                                                       |
| [`math`](math.md)`.ceil(x)`                           | Smallest int ≥ `x`. Accepts int (identity) or float.                                                                                |
| [`math`](math.md)`.floor(x)`                          | Largest int ≤ `x`. Accepts int (identity) or float.                                                                                 |
| [`math`](math.md)`.max(a, b)`                         | Larger of two numbers; mixed int/float promotes to float.                                                                           |
| [`math`](math.md)`.min(a, b)`                         | Smaller of two numbers; mixed int/float promotes to float.                                                                          |
| [`math`](math.md)`.pow(x, y)`                         | `x` raised to `y`; always float. Errors on NaN/Inf-producing inputs.                                                                |
| [`math`](math.md)`.round(x)`                          | Round to nearest int (half away from zero).                                                                                         |
| [`math`](math.md)`.sqrt(x)`                           | Square root; always float. Errors on negative input.                                                                                |
| [`os`](os.md)`.flag(name)`                            | Value following `name` in `os.ARGS`, or `""` if absent / at end. Exact-match (no `--foo=bar` parsing).                              |
| [`os`](os.md)`.getEnv(name)`                          | Read environment variable `name`. Unset → empty string, no error.                                                                   |
| [`os`](os.md)`.hasFlag(name)`                         | True if `name` appears as an exact element of `os.ARGS`.                                                                            |
| [`os`](os.md)`.kill(p)`                               | Send SIGTERM to spawned process `$p`.                                                                                               |
| [`os`](os.md)`.poll(p)`                               | True if spawned process `$p` has exited (a following `os.wait` returns immediately).                                                |
| [`os`](os.md)`.run(argv)`                             | Blocking: run `argv` to completion, return `os.Result{exitCode, stdout, stderr}`.                                                   |
| [`os`](os.md)`.spawn(argv)`                           | Non-blocking: start `argv`, return `os.Process{pid}` handle.                                                                        |
| [`os`](os.md)`.wait(p)`                               | Block until spawned process `$p` exits; return `os.Result`. Idempotent.                                                             |
| [`strings`](strings.md)`.chars(s)`                    | Split `s` into a `list of string`, one entry per Unicode code point.                                                                |
| [`strings`](strings.md)`.contains(s, sub)`            | True if `s` contains the substring `sub`.                                                                                           |
| [`strings`](strings.md)`.endsWith(s, suffix)`         | True if `s` ends with `suffix`.                                                                                                     |
| [`strings`](strings.md)`.indexOf(s, sub)`             | Rune index of first `sub` in `s`, or `-1` if absent.                                                                                |
| [`strings`](strings.md)`.join(parts, sep)`            | Concatenate `list of string` `parts` separated by `sep`. Inverse of `strings.split`.                                                |
| [`strings`](strings.md)`.lower(s)`                    | Lowercase `s` (Unicode-aware).                                                                                                      |
| [`strings`](strings.md)`.repeat(s, n)`                | `n` non-negative copies of `s` concatenated.                                                                                        |
| [`strings`](strings.md)`.replace(s, old, new)`        | Replace **all** occurrences of `old` in `s` with `new`.                                                                             |
| [`strings`](strings.md)`.split(s, sep)`               | Split `s` on non-empty `sep`; returns `list of string`.                                                                             |
| [`strings`](strings.md)`.startsWith(s, prefix)`       | True if `s` starts with `prefix`.                                                                                                   |
| [`strings`](strings.md)`.substring(s, start)`         | Rune-indexed slice of `s` from `start` to end.                                                                                      |
| [`strings`](strings.md)`.substring(s, start, end)`    | Rune-indexed slice; **exclusive** `end`.                                                                                            |
| [`strings`](strings.md)`.trim(s)`                     | Strip leading and trailing Unicode whitespace.                                                                                      |
| [`strings`](strings.md)`.trimLeft(s)`                 | Strip leading whitespace.                                                                                                           |
| [`strings`](strings.md)`.trimRight(s)`                | Strip trailing whitespace.                                                                                                          |
| [`strings`](strings.md)`.upper(s)`                    | Uppercase `s` (Unicode-aware).                                                                                                      |
| [`task`](task.md)`.discard($t)`                       | Mark a `task of T` fire-and-forget; suppresses exit-time loud-fail. Returns null.                                                   |
| [`task`](task.md)`.poll($t)`                          | True if `$t` has finished (non-blocking).                                                                                           |
| [`task`](task.md)`.wait($t)`                          | Block until `$t` finishes; return its value or re-raise its error.                                                                  |
| [`task`](task.md)`.waitAll($ts)`                      | Block for all tasks in `$ts`; results in list order; re-raises the first error if any.                                              |
| [`task`](task.md)`.waitAny($ts)`                      | Block until any task in `$ts` is done; return its index.                                                                            |
| [`time`](time.md)`.add($t, $d)`                       | `time.Time` shifted by duration `$d`.                                                                                               |
| [`time`](time.md)`.after($a, $b)`                     | True if `$a` is strictly later than `$b`.                                                                                           |
| [`time`](time.md)`.before($a, $b)`                    | True if `$a` is strictly earlier than `$b`.                                                                                         |
| [`time`](time.md)`.day($t)`                           | Day of month, 1-31.                                                                                                                 |
| [`time`](time.md)`.equal($a, $b)`                     | True if `$a` and `$b` are the same UTC instant.                                                                                     |
| [`time`](time.md)`.format($t, layout)`                | Strftime-style format. Codes: `%Y %m %d %H %M %S %z %a %A %b %B %j %u %%`.                                                          |
| [`time`](time.md)`.fromHours(n)`                      | `time.Duration` of `n` hours.                                                                                                       |
| [`time`](time.md)`.fromIso(s)`                        | Parse RFC 3339; accepts `Z` or `+HH:MM`; optional fractional seconds.                                                               |
| [`time`](time.md)`.fromMilliseconds(n)`               | `time.Duration` of `n` milliseconds.                                                                                                |
| [`time`](time.md)`.fromMinutes(n)`                    | `time.Duration` of `n` minutes.                                                                                                     |
| [`time`](time.md)`.fromSeconds(n)`                    | `time.Duration` of `n` seconds.                                                                                                     |
| [`time`](time.md)`.fromUnix(seconds)`                 | `time.Time` at the given Unix second.                                                                                               |
| [`time`](time.md)`.fromUnixMillis(ms)`                | `time.Time` at the given Unix millisecond.                                                                                          |
| [`time`](time.md)`.fromUnixNanos(ns)`                 | `time.Time` at the given Unix nanosecond.                                                                                           |
| [`time`](time.md)`.hour($t)`                          | Hour 0-23.                                                                                                                          |
| [`time`](time.md)`.hours($d)`                         | Span as whole hours (int).                                                                                                          |
| [`time`](time.md)`.inZone($t, $z)`                    | Re-render `$t` in `$z`'s wall-clock; UTC instant is preserved.                                                                      |
| [`time`](time.md)`.iso($t)`                           | RFC 3339 string: `Z` for UTC, `+HH:MM` otherwise; fractional seconds when non-zero.                                                 |
| [`time`](time.md)`.local()`                           | Host's current `time.Zone` (name + offset).                                                                                         |
| [`time`](time.md)`.milliseconds($d)`                  | Span as whole milliseconds (int).                                                                                                   |
| [`time`](time.md)`.minute($t)`                        | Minute 0-59.                                                                                                                        |
| [`time`](time.md)`.minutes($d)`                       | Span as whole minutes (int).                                                                                                        |
| [`time`](time.md)`.month($t)`                         | Calendar month, January = 1.                                                                                                        |
| [`time`](time.md)`.nanosecond($t)`                    | Fractional second, 0-999_999_999.                                                                                                   |
| [`time`](time.md)`.now()`                             | Current instant in the host's local zone (`time.Time`).                                                                             |
| [`time`](time.md)`.parse(s, layout)`                  | Strict strftime-style parse. Same code set as format (`%j` / `%u` are format-only).                                                 |
| [`time`](time.md)`.second($t)`                        | Second 0-59.                                                                                                                        |
| [`time`](time.md)`.seconds($d)`                       | Span as whole seconds (int).                                                                                                        |
| [`time`](time.md)`.sleep($d)`                         | Block the running task for `$d`. Negative / zero returns immediately. Returns null.                                                 |
| [`time`](time.md)`.sub($a, $b)`                       | Signed `time.Duration` between two `time.Time` values.                                                                              |
| [`time`](time.md)`.unix($t)`                          | Unix-second instant of `$t` (int).                                                                                                  |
| [`time`](time.md)`.unixMillis($t)`                    | Unix-millisecond instant of `$t` (int).                                                                                             |
| [`time`](time.md)`.unixNanos($t)`                     | Unix-nanosecond instant of `$t` (int).                                                                                              |
| [`time`](time.md)`.utc()`                             | Current instant in UTC (`time.Time`).                                                                                               |
| [`time`](time.md)`.weekday($t)`                       | ISO 8601 weekday: Monday = 1 ... Sunday = 7.                                                                                        |
| [`time`](time.md)`.year($t)`                          | Calendar year (int).                                                                                                                |
| [`time`](time.md)`.zone(offset, name)`                | Build a `time.Zone` from an integer offset (seconds east of UTC) and a display name.                                                |

## Constants

| Name                                       | Type           | Value                                                                                            |
| ------------------------------------------ | -------------- | ------------------------------------------------------------------------------------------------ |
| [`math`](math.md)`.E`                      | `float`        | Euler's number, 2.718281828459045.                                                               |
| [`math`](math.md)`.PI`                     | `float`        | π, 3.141592653589793.                                                                            |
| [`meta`](meta.md)`.BUILD`                  | `string`       | Which Go toolchain compiled the interpreter: `"go"` / `"tinygo"`.                                |
| [`meta`](meta.md)`.VERSION`                | `string`       | The interpreter's build version (e.g. `"0.14.0"`).                                               |
| [`os`](os.md)`.ARCH`                       | `string`       | CPU architecture: `"amd64"`, `"arm64"`, `"wasm"`, ...                                            |
| [`os`](os.md)`.ARGS`                       | list of string | Argv. Index 0 is the script path, the rest are user args.                                        |
| [`os`](os.md)`.DIRSEP`                     | `string`       | Path-component separator: `"/"` Unix, `"\\"` Windows.                                            |
| [`os`](os.md)`.EOL`                        | `string`       | Platform line ending. `"\n"` Unix-likes, `"\r\n"` Windows.                                       |
| [`os`](os.md)`.PATHSEP`                    | `string`       | PATH-list separator: `":"` Unix, `";"` Windows.                                                  |
| [`os`](os.md)`.PLATFORM`                   | `string`       | OS tag: `"linux"`, `"darwin"`, `"windows"`, ...                                                  |
| [`time`](time.md)`.PROGRAM_START`          | `time.Time`    | Captured the moment the time library installed; "since program launched" anchor.                 |
| [`time`](time.md)`.UTC`                    | `time.Zone`    | Canonical UTC: `Zone{offset: 0, name: "UTC"}`.                                                   |

## Type-conversion calls

`int`, `float`, `string`, `bool` are also type keywords (used in `def x
as int`). The parser allows them in expression position **only** when
immediately followed by `(`, so `def x as int init convert.toInt("42");` works
but `def x as int init int;` errors. See
[convert.md](convert.md#notes-on-the-type-name-syntax) for the parser
detail.

## See also

- [index.md](index.md) - library catalog with code samples and the
  organizing principles.
- Per-library reference pages: [io.md](io.md), [convert.md](convert.md),
  [math.md](math.md), [strings.md](strings.md), [lists.md](lists.md),
  [maps.md](maps.md), [os.md](os.md), [meta.md](meta.md),
  [time.md](time.md), [hash.md](hash.md), [crc.md](crc.md),
  [encoding.md](encoding.md), [task.md](task.md), [fs.md](fs.md), [net.md](net.md).
- [../user-guide/imports.md](../user-guide/imports.md) - how to import a
  library in a Jennifer source file.
