# Jennifer libraries

Jennifer's standard library is split into topic-based libraries. Each
is enabled explicitly with `use NAME;`; nothing is auto-loaded. This
page catalogs every library that ships with the interpreter and
links to the reference doc for each.

> **Looking for one specific function?** See the
> [cheatsheet](cheatsheet.md) - alphabetical list of every builtin
> with its library and a one-line description.

> **`len`** is a language built-in primary, not a library
> function. Use it from any program with no `use` statement; it's
> polymorphic over string / list / map / bytes.

The **TinyGo** column reports whether the library runs in full on
the constrained `jennifer-tiny` binary (TinyGo-built). A `partial`
entry links to [../technical/tinygo.md](../technical/tinygo.md) for
the restriction list; the default `jennifer` binary (standard Go)
always supports the full surface.

`net`'s "stubs only" is about the **stock** `jennifer-tiny` build,
which ships no network driver - not a TinyGo limitation. A tiny build
rebuilt with a network stack runs `net` too; see the
[note on `net` and TinyGo](../technical/tinygo.md#net-on-tinygo-is-a-build-choice-not-a-hard-limit).

| Library                   | Enable with     | TinyGo                                                  | Contents                                                                                                                                                                       |
| ------------------------- | --------------- | ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| [`convert`](convert.md)   | `use convert;`  | full                                                    | `convert.toInt`, `convert.toFloat`, `convert.toString`, `convert.toBool`, `convert.typeOf` - explicit casts; canonical-only `toBool` conversion                                |
| [`archive`](archive.md)   | `use archive;`  | full                                                    | tar / zip containers over `bytes` (no `fs`). `archive.pack`/`unpack` with format `"tar"`/`"zip"`/`"tar.gz"`; bundle is a `list of archive.Entry` `{name, data, mode, mtime}` |
| [`compress`](compress.md) | `use compress;` | full                                                    | byte-stream compression. `compress.pack`/`unpack` for `"gzip"`/`"zlib"`/`"deflate"` (`bytes` in/out, optional `"fast"`/`"default"`/`"best"` level) + streaming (`compress.stream`/`update`/`finalize`); struct `compress.Stream` |
| [`crc`](crc.md)           | `use crc;`      | full                                                    | `crc.compute(b, algo)` + streaming (`crc.stream`/`update`/`finalize`) for `"crc32"`, `"crc64"`; output is big-endian bytes; struct `crc.Stream`                          |
| [`encoding`](encoding.md) | `use encoding;` | full                                                    | introspection (`isAscii`, `lenBytes`, `lenRunes`); binary-to-text `toText`/`fromText` for `"hex"`, `"base64"`, `"base64-url"`; character codecs `encode`/`decode` for `"ascii"`, `"iso-8859-1"`, `"windows-1252"`, `"ebcdic"` |
| [`fs`](fs.md)             | `use fs;`       | full                                                    | filesystem I/O. Whole-file `readString`/`readBytes`/`writeString`/`writeBytes`/`appendString`/`appendBytes`; metadata `exists`/`isFile`/`isDir`/`stat`; dir ops `mkdir`/`mkdirAll`/`remove`/`removeAll`/`rename`/`list`/`walk`; handles `open`/`readLine`/`readChars`/`readBytes`/`writeString`/`writeBytes`/`eof`/`close`; structs `fs.Stat`, `fs.File` |
| [`hash`](hash.md)         | `use hash;`     | full                                                    | `hash.compute(b, algo)` + streaming (`hash.stream`/`update`/`finalize`) for `"md5"`, `"sha1"`, `"sha256"`; struct `hash.Stream`                                          |
| [`io`](io.md)             | `use io;`       | full                                                    | `io.printf`, `io.sprintf`, `io.readLine`, `io.eof`, plus the format-verb mini-language                                                                                         |
| [`json`](json.md)         | `use json;`     | full                                                    | RFC 8259 `json.encode`/`encodePretty`/`decode`. Structs and `map of string to V` map to objects, `bytes` to base64, integral numbers decode to `int` else `float`. Decode yields generic values (no map-to-struct coercion). |
| [`lists`](lists.md)       | `use lists;`    | full                                                    | `lists.push`, `pop`, `first`, `last`, `head`, `tail`, `reverse`, `sort`, `contains`, `concat`, `slice`, `shuffle`, `range` - all return a new list                             |
| [`maps`](maps.md)         | `use maps;`     | full                                                    | `maps.keys`, `values`, `has`, `delete`, `merge` - all return a new map / list / bool                                                                                           |
| [`math`](math.md)         | `use math;`     | full                                                    | `math.abs`, `min`, `max`, `sqrt`, `pow`, `floor`, `ceil`, `round`, `rand`, `randInt`, `randSeed`; constants `math.PI`, `math.E`                                                |
| [`meta`](meta.md)         | `use meta;`     | full                                                    | `meta.VERSION`, `meta.BUILD` - interpreter-self-identity constants                                                                                                             |
| [`net`](net.md)           | `use net;`      | [stubs only](../technical/tinygo.md#tinygo-restrictions) | TCP `connect`/`listen`/`accept`/`readBytes`/`writeBytes`/`eof`/`address`, UDP `listenUDP`/`sendTo`/`recvFrom`, DNS `lookup`/`reverseLookup`, polymorphic `close`/`address`; structs `net.Conn`, `net.Listener`, `net.UDPSocket`, `net.Datagram`. `jennifer-tiny` returns friendly errors; use the default `jennifer` binary for real net I/O. |
| [`os`](os.md)             | `use os;`       | [partial](../technical/tinygo.md#tinygo-restrictions)   | `os.getEnv`, `os.hasFlag`, `os.flag`, `os.run`, `os.spawn`, `os.wait`, `os.poll`, `os.kill`; constants `os.PLATFORM`, `os.ARCH`, `os.EOL`, `os.DIRSEP`, `os.PATHSEP`, `os.ARGS` |
| [`regex`](regex.md)       | `use regex;`    | full                                                    | regular expressions over `string` (RE2 syntax). `regex.matches`/`find`/`findAll`/`replace`/`split`/`escape` + `regex.Match` struct with positional and named captures. Implicit LRU cache for compiled patterns. |
| [`strings`](strings.md)   | `use strings;`  | full                                                    | `strings.upper`, `lower`, `contains`, `startsWith`, `endsWith`, `indexOf`, `trim`, `trimLeft`, `trimRight`, `replace`, `repeat`, `substring`, `split`, `chars`, `join`         |
| [`task`](task.md)         | `use task;`     | full                                                    | observe and join `task of T` handles produced by `spawn { ... }`. `task.wait`, `task.poll`, `task.discard`, `task.waitAll`, `task.waitAny`; pairs with the [user-guide concurrency tour](../user-guide/concurrency.md) |
| [`testing`](testing.md)   | `use testing;`  | full                                                    | test-runner primitives. `testing.run`/`results`/`reset`/`report` + `testing.Result` struct. Catches runtime errors, throws, and (uniquely) `exit` inside test bodies. Three report formats: `"text"`, `"tap"`, `"junit"`. Foundation for the .j-side test framework. |
| [`time`](time.md)         | `use time;`     | full                                                    | instant/duration arithmetic, calendar + Unix accessors, fixed-offset zones (`time.zone`, `time.inZone`, `time.UTC`, `time.local`), strftime format/parse, ISO round-trip; structs `time.Time`, `time.Duration`, `time.Zone` |
| [`toml`](toml.md)         | `use toml;`     | full                                                    | RFC-conformant TOML 1.0 `toml.encode`/`encodePretty`/`decode`. Same opaque `toml.Value` + read / walk / write surface as `json`, name for name, addressed by JSON Pointer; adds `toml.asDatetime` (backed by `time.Time`) for TOML's native date-times. |
| [`uuid`](uuid.md)         | `use uuid;`     | full                                                    | RFC 9562 UUIDs. `uuid.generate("v4")` (random) / `generate("v7")` (time-ordered) + `parse`/`isValid`/`version` + constant `NIL`. Version tag is a string arg; draws from `math`'s seedable RNG (not crypto-grade). |

A quick taste:

```jennifer
use io;
use math;
use meta;
use strings;

io.printf("Jennifer %s\n", meta.VERSION);
io.printf("pi is roughly %f\n", math.PI);
io.printf("math.sqrt(2) = %f\n", math.sqrt(2));
io.printf("upper: %s\n", strings.upper("hello"));
io.printf("len: %d\n", len("hello"));           # language built-in, no import
```

## Namespace-first registration

Every library is **namespaced**: each name is reachable as
`lib.name(...)` (call) or `lib.NAME` (constant). The library's name
doubles as the namespace prefix at the use site.

Aliasing (`use lib as alias;`) is a **rename**, not an addition:
after the alias the canonical name no longer resolves at call sites
(it errors with a "did you mean *alias*?" hint). The canonical name
is also freed for use as an ordinary identifier, just like Python's
`import foo as bar`.

(`core` used to be auto-loaded and exposed `len` /
`JENNIFER_VERSION` as bare globals via `RegisterGlobal` /
`RegisterGlobalConst`. `len` was promoted to a language built-in
keyword, version constants moved to `meta`,
and `core` was deleted. The `RegisterGlobal*` API surface remains on
`Interpreter` but is unused by any shipping library; it gets removed
in a later cleanup pass.)

## How libraries are organized

The standard library favors many small, focused libraries over a few
large ones. The organizing principle, captured for future extensions:

- Touches I/O (stdin/stdout/files/network/clock) -> `io` (which can
  later split into `fs`, `net`, `time` as it grows).
- Pure value transformation across kinds -> `convert`.
- Pure numeric -> `math` (includes the non-crypto random helpers).
- String manipulation -> `strings`.
- List manipulation -> `lists`.
- Map manipulation -> `maps`.
- Operating-system glue (env, args, host info) -> `os`.
- Interpreter-self-identity constants (version, build, future
  build-time / git-sha / GC stats) -> `meta`.
- Time / instants / durations -> `time` (formatting,
  parsing, and fixed-offset zones).
- Cryptographic-style digests (MD5, SHA-1, SHA-256) -> `hash`.
  Non-cryptographic checksums (CRC-32, CRC-64) -> `crc`.
  The split keeps "transport integrity" and "content
  addressing" visible at the import line.
- Byte / string introspection and character-set codecs (ASCII,
  ISO-8859-1, Windows-1252, EBCDIC IBM-1047) plus hex / base64
  binary-to-text -> `encoding` (long-tail codecs parked for later).
- Observing / joining background computations launched with `spawn`
  -> `task`. Concurrency itself is a language feature
  (the `spawn` keyword + `task of T` type); the library is just the
  observation surface.
- Filesystem I/O (whole-file reads/writes, metadata, directory
  operations, buffered file handles) -> `fs`. Blocking on
  purpose; non-blocking use composes with `spawn`.
- Network I/O (TCP + UDP sockets, DNS lookups) -> `net`.
  Blocking calls, same spawn-composition story as `fs`. The
  **stock** `jennifer-tiny` binary returns friendly "use the
  default `jennifer`" errors because it ships no network driver -
  a build choice, not a TinyGo limitation; a tiny build with a
  network stack runs `net` too.
- Regular expressions over `string` -> `regex`. RE2
  syntax (Go's `regexp` engine); implicit LRU cache. Pure
  string processing, no other library dependencies.
- Test-runner primitives (name-based method dispatch,
  per-process result accumulator, format dispatcher for
  text/TAP/JUnit) -> `testing`. Lives here because
  Jennifer has no function references yet; the `.j`-side
  assertion vocabulary and CLI harness ship on top
  of these primitives.
- A genuinely new topic with **five or more** functions / constants
  -> a new library. Fewer than five names fold into the most-related
  existing library (the non-crypto random helpers were the first
  case the rule caught - they live under `math.rand*` rather than
  getting their own library).
- A single function with no clear topic -> the most-related existing
  library.
- Genuinely polymorphic structural primitives that every program
  needs (`len`) -> language built-in keyword, not a library. The
  bar is intentionally high; `len` is the only one.

## Naming convention

Library names look mixed at first glance - `strings` is plural but
`math` is singular. The rule:

- **Plural for count nouns**: when the library operates on instances of
  something you can have multiples of. `strings`, `lists`, `maps`,
  `bytes`, `files`.
- **Singular for mass nouns and conceptual wholes**: `math`, `meta`,
  `time`, `regex` (planned).
- **Bare verb when the library is named for what it does**, not what
  it touches: `convert`.
- **Idiomatic abbreviations are fine**: `os`, `fs`, `net`, `regex`.

Three practical constraints reinforce the count/mass rule:

1. Type keywords are reserved. `string`, `int`, `float`, `bool`,
   `list`, `map`, `null` cannot be library names because they
   tokenize as type tokens, not IDENTs. The plural form (`strings`,
   `lists`, `maps`) sidesteps this naturally.
2. The rule matches Go's stdlib: `strings` and `bytes` are plural;
   `math`, `io`, `os` are singular. Since the interpreter is written
   in Go, the convention transfers cleanly to library author
   intuition.
3. Within a library, function names are lowercase / camelCase
   (`upper`, `startsWith`, `typeOf`). Constants are uppercase
   (`PI`, `E`, `VERSION`, `PLATFORM`).

For implementation notes on how libraries register themselves with the
interpreter (`RegisterNamespaced`, the `use`-gated lookup), see
[../technical/interpreter.md > Builtins and libraries](../technical/interpreter.md#builtins-and-libraries).

For canonical terminology (library vs module, function vs method,
list vs array, ...), see [../glossary.md](../glossary.md). This page
uses the terms in that table.
