# Cheatsheet - all builtins at a glance

Alphabetical index of every standard-library function and constant. Use
it when you know the *name* and want to know *which library* and *how to
call it*; use each library's own page when you want to read about a
topic.

The table covers what ships with the interpreter today. New
entries land here at the same time as the per-library doc - it's a
flat lookup view, not authoritative.

## Functions

| Call                               | Library                         | What it does                                                                                                                        |
| ---------------------------------- | ------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| `convert.toBool(v)`                | [convert](convert.md)           | Canonical conversion to `bool` (`0`/`1`, `0.0`/`1.0`, `"true"`/`"false"`).                                                          |
| `convert.toFloat(v)`               | [convert](convert.md)           | Convert to float (int→float, float identity, string parses, bool→1.0/0.0).                                                          |
| `convert.toInt(v)`                 | [convert](convert.md)           | Convert to int (float truncates toward zero, string parses, bool→1/0).                                                              |
| `convert.toString(v)`              | [convert](convert.md)           | Convert to string (always succeeds; uses the value's display form).                                                                 |
| `convert.typeOf(v)`                | [convert](convert.md)           | Runtime kind as string (`"int"`, `"float"`, `"string"`, `"bool"`, `"null"`, `"list"`, `"map"`).                                     |
| `crc.compute(b, algo)`             | [crc](crc.md)                   | One-shot checksum. `algo` is `"crc32"` or `"crc64"`. Returns big-endian bytes (4 or 8).                                             |
| `crc.finalize($s)`                 | [crc](crc.md)                   | Final checksum as big-endian bytes; consumes the handle.                                                                            |
| `crc.stream(algo)`                 | [crc](crc.md)                   | Allocate a `crc.Stream` for `algo`; feed chunks via `crc.update` then close with `crc.finalize`.                                    |
| `crc.update($s, $bytes)`           | [crc](crc.md)                   | Feed one chunk into a `crc.Stream` (mutates by side effect).                                                                        |
| `encoding.codecs()`                | [encoding](encoding.md)         | Canonical character-codec names in registration order.                                                                              |
| `encoding.decode(b, codec)`        | [encoding](encoding.md)         | Decode `bytes` from a character codec to a Jennifer string.                                                                         |
| `encoding.encode(s, codec)`        | [encoding](encoding.md)         | Encode a Jennifer string into a character codec's bytes.                                                                            |
| `encoding.fromText(s, format)`     | [encoding](encoding.md)         | Decode a binary-to-text format. `format`: `"hex"`, `"base64"`, `"base64-url"`.                                                      |
| `encoding.isAscii(b)`              | [encoding](encoding.md)         | True iff every byte in `b` is < 0x80.                                                                                               |
| `encoding.lenBytes(s)`             | [encoding](encoding.md)         | UTF-8 byte length of `s` (pair with `len(s)` for rune count).                                                                       |
| `encoding.lenRunes(b)`             | [encoding](encoding.md)         | Rune count of valid UTF-8 `bytes`; errors on invalid UTF-8.                                                                         |
| `encoding.toText(b, format)`       | [encoding](encoding.md)         | Encode `bytes` as printable text. `format`: `"hex"`, `"base64"`, `"base64-url"`.                                                    |
| `hash.compute(b, algo)`            | [hash](hash.md)                 | One-shot digest. `algo` is `"md5"`, `"sha1"`, or `"sha256"`. Returns raw bytes.                                                     |
| `hash.finalize($s)`                | [hash](hash.md)                 | Final digest as bytes; consumes the handle (later calls error).                                                                     |
| `hash.stream(algo)`                | [hash](hash.md)                 | Allocate a `hash.Stream` for `algo`; feed chunks via `hash.update` then close with `hash.finalize`.                                 |
| `hash.update($s, $bytes)`          | [hash](hash.md)                 | Feed one chunk into a `hash.Stream` (mutates by side effect).                                                                       |
| `io.eof()`                         | [io](io.md)                     | True if and only if the next `io.readLine()` would error. Pair with `while (not io.eof()) {...}`.                                   |
| `io.printf(format, args...)`       | [io](io.md)                     | Format-string write to stdout. Verbs: `%d %f %s %t %v %%`; per-verb `\|key=value` modifiers (`pad`, `prec`, `base`, `null=*`, ...). |
| `io.printf(value)`                 | [io](io.md)                     | Write a value's display form to stdout.                                                                                             |
| `io.readLine()`                    | [io](io.md)                     | Read one line from stdin (trailing newline stripped). Errors at EOF - check `io.eof()` first.                                       |
| `io.readLine(prompt)`              | [io](io.md)                     | Same as `io.readLine()` but writes `prompt` to stdout first.                                                                        |
| `io.sprintf(format, args...)`      | [io](io.md)                     | Format-string version of `sprintf`. Same verbs and `\|key=value` modifiers as `printf`.                                             |
| `io.sprintf(value)`                | [io](io.md)                     | Display-form of a value, returned as a string (doesn't write).                                                                      |
| `len(v)`                           | *(language built-in)*           | Structural length: rune count (string), element count (list), entry count (map), byte count (bytes).                                |
| `lists.concat(a, b)`               | [lists](lists.md)               | New list with `a`'s elements followed by `b`'s.                                                                                     |
| `lists.contains(xs, item)`         | [lists](lists.md)               | True if `item` appears in `xs` (haystack, needle).                                                                                  |
| `lists.first(xs)`                  | [lists](lists.md)               | Element at index 0. Empty input errors.                                                                                             |
| `lists.head(xs, n)`                | [lists](lists.md)               | New list of the first `n` elements.                                                                                                 |
| `lists.last(xs)`                   | [lists](lists.md)               | Element at the last index. Empty input errors.                                                                                      |
| `lists.pop(xs)`                    | [lists](lists.md)               | New list without the last element. Empty input errors.                                                                              |
| `lists.push(xs, item)`             | [lists](lists.md)               | New list with `item` appended.                                                                                                      |
| `lists.range(start, end[, step])`  | [lists](lists.md)               | Half-open list of consecutive ints; `end` excluded; `step` must match direction.                                                    |
| `lists.reverse(xs)`                | [lists](lists.md)               | New list with elements reversed.                                                                                                    |
| `lists.shuffle(xs)`                | [lists](lists.md)               | Fisher-Yates; respects `math.randSeed`. Non-mutating.                                                                               |
| `lists.slice(xs, start[, end])`    | [lists](lists.md)               | New sublist `[start, end)`; `end` defaults to `len(xs)`.                                                                            |
| `lists.sort(xs)`                   | [lists](lists.md)               | New ascending-sorted list. Numeric / string / bool elements; mixed errors.                                                          |
| `lists.tail(xs, n)`                | [lists](lists.md)               | New list of the last `n` elements.                                                                                                  |
| `maps.delete(m, key)`              | [maps](maps.md)                 | New map without `key`. Missing key errors (strict at boundaries).                                                                   |
| `maps.has(m, key)`                 | [maps](maps.md)                 | True if map `m` contains `key`. The non-erroring companion to `$m[key]`.                                                            |
| `maps.keys(m)`                     | [maps](maps.md)                 | List of keys in insertion order.                                                                                                    |
| `maps.merge(a, b)`                 | [maps](maps.md)                 | New map; `b`'s entries layered on top of `a`.                                                                                       |
| `maps.values(m)`                   | [maps](maps.md)                 | List of values in insertion order.                                                                                                  |
| `math.abs(x)`                      | [math](math.md)                 | Absolute value of `x` (int→int, float→float).                                                                                       |
| `math.ceil(x)`                     | [math](math.md)                 | Smallest int ≥ `x`. Accepts int (identity) or float.                                                                                |
| `math.floor(x)`                    | [math](math.md)                 | Largest int ≤ `x`. Accepts int (identity) or float.                                                                                 |
| `math.max(a, b)`                   | [math](math.md)                 | Larger of two numbers; mixed int/float promotes to float.                                                                           |
| `math.min(a, b)`                   | [math](math.md)                 | Smaller of two numbers; mixed int/float promotes to float.                                                                          |
| `math.pow(x, y)`                   | [math](math.md)                 | `x` raised to `y`; always float. Errors on NaN/Inf-producing inputs.                                                                |
| `math.round(x)`                    | [math](math.md)                 | Round to nearest int (half away from zero).                                                                                         |
| `math.sqrt(x)`                     | [math](math.md)                 | Square root; always float. Errors on negative input.                                                                                |
| `os.flag(name)`                    | [os](os.md)                     | Value following `name` in `os.ARGS`, or `""` if absent / at end. Exact-match (no `--foo=bar` parsing).                              |
| `os.getEnv(name)`                  | [os](os.md)                     | Read environment variable `name`. Unset → empty string, no error.                                                                   |
| `os.hasFlag(name)`                 | [os](os.md)                     | True if `name` appears as an exact element of `os.ARGS`.                                                                            |
| `os.kill(p)`                       | [os](os.md)                     | Send SIGTERM to spawned process `$p`.                                                                                               |
| `os.poll(p)`                       | [os](os.md)                     | True if spawned process `$p` has exited (a following `os.wait` returns immediately).                                                |
| `os.run(argv)`                     | [os](os.md)                     | Blocking: run `argv` to completion, return `os.Result{exitCode, stdout, stderr}`.                                                   |
| `os.spawn(argv)`                   | [os](os.md)                     | Non-blocking: start `argv`, return `os.Process{pid}` handle.                                                                        |
| `os.wait(p)`                       | [os](os.md)                     | Block until spawned process `$p` exits; return `os.Result`. Idempotent.                                                             |
| `strings.chars(s)`                 | [strings](strings.md)           | Split `s` into a `list of string`, one entry per Unicode code point.                                                                |
| `strings.contains(s, sub)`         | [strings](strings.md)           | True if `s` contains the substring `sub`.                                                                                           |
| `strings.endsWith(s, suffix)`      | [strings](strings.md)           | True if `s` ends with `suffix`.                                                                                                     |
| `strings.indexOf(s, sub)`          | [strings](strings.md)           | Rune index of first `sub` in `s`, or `-1` if absent.                                                                                |
| `strings.join(parts, sep)`         | [strings](strings.md)           | Concatenate `list of string` `parts` separated by `sep`. Inverse of `strings.split`.                                                |
| `strings.lower(s)`                 | [strings](strings.md)           | Lowercase `s` (Unicode-aware).                                                                                                      |
| `strings.repeat(s, n)`             | [strings](strings.md)           | `n` non-negative copies of `s` concatenated.                                                                                        |
| `strings.replace(s, old, new)`     | [strings](strings.md)           | Replace **all** occurrences of `old` in `s` with `new`.                                                                             |
| `strings.split(s, sep)`            | [strings](strings.md)           | Split `s` on non-empty `sep`; returns `list of string`.                                                                             |
| `strings.startsWith(s, prefix)`    | [strings](strings.md)           | True if `s` starts with `prefix`.                                                                                                   |
| `strings.substring(s, start)`      | [strings](strings.md)           | Rune-indexed slice of `s` from `start` to end.                                                                                      |
| `strings.substring(s, start, end)` | [strings](strings.md)           | Rune-indexed slice; **exclusive** `end`.                                                                                            |
| `strings.trim(s)`                  | [strings](strings.md)           | Strip leading and trailing Unicode whitespace.                                                                                      |
| `strings.trimLeft(s)`              | [strings](strings.md)           | Strip leading whitespace.                                                                                                           |
| `strings.trimRight(s)`             | [strings](strings.md)           | Strip trailing whitespace.                                                                                                          |
| `strings.upper(s)`                 | [strings](strings.md)           | Uppercase `s` (Unicode-aware).                                                                                                      |
| `time.add($t, $d)`                 | [time](time.md)                 | `time.Time` shifted by duration `$d`.                                                                                               |
| `time.after($a, $b)`               | [time](time.md)                 | True if `$a` is strictly later than `$b`.                                                                                           |
| `time.before($a, $b)`              | [time](time.md)                 | True if `$a` is strictly earlier than `$b`.                                                                                         |
| `time.day($t)`                     | [time](time.md)                 | Day of month, 1-31.                                                                                                                 |
| `time.equal($a, $b)`               | [time](time.md)                 | True if `$a` and `$b` are the same UTC instant.                                                                                     |
| `time.format($t, layout)`          | [time](time.md)                 | Strftime-style format. Codes: `%Y %m %d %H %M %S %z %a %A %b %B %j %u %%`.                                                          |
| `time.fromHours(n)`                | [time](time.md)                 | `time.Duration` of `n` hours.                                                                                                       |
| `time.fromIso(s)`                  | [time](time.md)                 | Parse RFC 3339; accepts `Z` or `+HH:MM`; optional fractional seconds.                                                               |
| `time.fromMilliseconds(n)`         | [time](time.md)                 | `time.Duration` of `n` milliseconds.                                                                                                |
| `time.fromMinutes(n)`              | [time](time.md)                 | `time.Duration` of `n` minutes.                                                                                                     |
| `time.fromSeconds(n)`              | [time](time.md)                 | `time.Duration` of `n` seconds.                                                                                                     |
| `time.fromUnix(seconds)`           | [time](time.md)                 | `time.Time` at the given Unix second.                                                                                               |
| `time.fromUnixMillis(ms)`          | [time](time.md)                 | `time.Time` at the given Unix millisecond.                                                                                          |
| `time.fromUnixNanos(ns)`           | [time](time.md)                 | `time.Time` at the given Unix nanosecond.                                                                                           |
| `time.hour($t)`                    | [time](time.md)                 | Hour 0-23.                                                                                                                          |
| `time.hours($d)`                   | [time](time.md)                 | Span as whole hours (int).                                                                                                          |
| `time.inZone($t, $z)`              | [time](time.md)                 | Re-render `$t` in `$z`'s wall-clock; UTC instant is preserved.                                                                      |
| `time.iso($t)`                     | [time](time.md)                 | RFC 3339 string: `Z` for UTC, `+HH:MM` otherwise; fractional seconds when non-zero.                                                 |
| `time.local()`                     | [time](time.md)                 | Host's current `time.Zone` (name + offset).                                                                                         |
| `time.milliseconds($d)`            | [time](time.md)                 | Span as whole milliseconds (int).                                                                                                   |
| `time.minute($t)`                  | [time](time.md)                 | Minute 0-59.                                                                                                                        |
| `time.minutes($d)`                 | [time](time.md)                 | Span as whole minutes (int).                                                                                                        |
| `time.month($t)`                   | [time](time.md)                 | Calendar month, January = 1.                                                                                                        |
| `time.nanosecond($t)`              | [time](time.md)                 | Fractional second, 0-999_999_999.                                                                                                   |
| `time.now()`                       | [time](time.md)                 | Current instant in the host's local zone (`time.Time`).                                                                             |
| `time.parse(s, layout)`            | [time](time.md)                 | Strict strftime-style parse. Same code set as format (`%j` / `%u` are format-only).                                                 |
| `time.second($t)`                  | [time](time.md)                 | Second 0-59.                                                                                                                        |
| `time.seconds($d)`                 | [time](time.md)                 | Span as whole seconds (int).                                                                                                        |
| `time.sleep($d)`                   | [time](time.md)                 | Block the running task for `$d`. Negative / zero returns immediately. Returns null.                                                 |
| `time.sub($a, $b)`                 | [time](time.md)                 | Signed `time.Duration` between two `time.Time` values.                                                                              |
| `time.unix($t)`                    | [time](time.md)                 | Unix-second instant of `$t` (int).                                                                                                  |
| `time.unixMillis($t)`              | [time](time.md)                 | Unix-millisecond instant of `$t` (int).                                                                                             |
| `time.unixNanos($t)`               | [time](time.md)                 | Unix-nanosecond instant of `$t` (int).                                                                                              |
| `time.utc()`                       | [time](time.md)                 | Current instant in UTC (`time.Time`).                                                                                               |
| `time.weekday($t)`                 | [time](time.md)                 | ISO 8601 weekday: Monday = 1 ... Sunday = 7.                                                                                        |
| `time.year($t)`                    | [time](time.md)                 | Calendar year (int).                                                                                                                |
| `time.zone(offset, name)`          | [time](time.md)                 | Build a `time.Zone` from an integer offset (seconds east of UTC) and a display name.                                                |

## Constants

| Name                 | Library         | Type           | Value                                                                                            |
| -------------------- | --------------- | -------------- | ------------------------------------------------------------------------------------------------ |
| `math.E`             | [math](math.md) | `float`        | Euler's number, 2.718281828459045.                                                               |
| `math.PI`            | [math](math.md) | `float`        | π, 3.141592653589793.                                                                            |
| `meta.BUILD`         | [meta](meta.md) | `string`       | Which Go toolchain compiled the interpreter: `"go"` / `"tinygo"`.                                |
| `meta.VERSION`       | [meta](meta.md) | `string`       | The interpreter's build version (e.g. `"0.14.0"`).                                               |
| `os.ARCH`            | [os](os.md)     | `string`       | CPU architecture: `"amd64"`, `"arm64"`, `"wasm"`, ...                                            |
| `os.ARGS`            | [os](os.md)     | list of string | Argv. Index 0 is the script path, the rest are user args.                                        |
| `os.DIRSEP`          | [os](os.md)     | `string`       | Path-component separator: `"/"` Unix, `"\\"` Windows.                                            |
| `os.EOL`             | [os](os.md)     | `string`       | Platform line ending. `"\n"` Unix-likes, `"\r\n"` Windows.                                       |
| `os.PATHSEP`         | [os](os.md)     | `string`       | PATH-list separator: `":"` Unix, `";"` Windows.                                                  |
| `os.PLATFORM`        | [os](os.md)     | `string`       | OS tag: `"linux"`, `"darwin"`, `"windows"`, ...                                                  |
| `time.PROGRAM_START` | [time](time.md) | `time.Time`    | Captured the moment the time library installed; "since program launched" anchor.                 |
| `time.UTC`           | [time](time.md) | `time.Zone`    | Canonical UTC: `Zone{offset: 0, name: "UTC"}`.                                                   |

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
  [encoding.md](encoding.md).
- [../user-guide/imports.md](../user-guide/imports.md) - how to import a
  library in a Jennifer source file.
