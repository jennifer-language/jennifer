# Jennifer libraries

Jennifer's standard library is split into topic-based libraries. Each
is enabled explicitly with `use NAME;`; nothing is auto-loaded. This
page catalogs every library that ships with the interpreter today and
links to the reference doc for each.

> **Looking for one specific function?** See the
> [cheatsheet](cheatsheet.md) - alphabetical list of every builtin
> with its library and a one-line description.

> **`len`** is a language built-in primary (M15.4+), not a library
> function. Use it from any program with no `use` statement; it's
> polymorphic over string / list / map / bytes.

The **TinyGo** column reports whether the library runs in full on
the shipping `jennifer` binary (TinyGo-built). A `partial` entry
links to [../technical/tinygo.md](../technical/tinygo.md) for the
restriction list; `jennifer-go` always supports the full surface.

| Library                  | Enable with     | TinyGo                                                | Contents                                                                                                                                                                       |
| ------------------------ | --------------- | ----------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| [`io`](io.md)            | `use io;`       | full                                                  | `io.printf`, `io.sprintf`, `io.readLine`, `io.eof`, plus the format-verb mini-language                                                                                         |
| [`convert`](convert.md)  | `use convert;`  | full                                                  | `convert.toInt`, `convert.toFloat`, `convert.toString`, `convert.toBool`, `convert.typeOf` - explicit casts; canonical-only `toBool` conversion                                |
| [`math`](math.md)        | `use math;`     | full                                                  | `math.abs`, `min`, `max`, `sqrt`, `pow`, `floor`, `ceil`, `round`, `rand`, `randInt`, `randSeed`; constants `math.PI`, `math.E`                                                |
| [`strings`](strings.md)  | `use strings;`  | full                                                  | `strings.upper`, `lower`, `contains`, `startsWith`, `endsWith`, `indexOf`, `trim`, `trimLeft`, `trimRight`, `replace`, `repeat`, `substring`, `split`, `chars`, `join`         |
| [`lists`](lists.md)      | `use lists;`    | full                                                  | `lists.push`, `pop`, `first`, `last`, `head`, `tail`, `reverse`, `sort`, `contains`, `concat`, `slice`, `shuffle`, `range` - all return a new list                             |
| [`maps`](maps.md)        | `use maps;`     | full                                                  | `maps.keys`, `values`, `has`, `delete`, `merge` - all return a new map / list / bool                                                                                           |
| [`os`](os.md)            | `use os;`       | [partial](../technical/tinygo.md#tinygo-restrictions) | `os.getEnv`, `os.hasFlag`, `os.flag`, `os.run`, `os.spawn`, `os.wait`, `os.poll`, `os.kill`; constants `os.PLATFORM`, `os.ARCH`, `os.EOL`, `os.DIRSEP`, `os.PATHSEP`, `os.ARGS` |
| [`meta`](meta.md)        | `use meta;`     | full                                                  | `meta.VERSION`, `meta.BUILD` - interpreter-self-identity constants                                                                                                             |
| [`time`](time.md)        | `use time;`     | full                                                  | M15.5.1+.2: instant/duration arithmetic, calendar + Unix accessors, fixed-offset zones (`time.zone`, `time.inZone`, `time.UTC`, `time.local`), strftime format/parse, ISO round-trip; structs `time.Time`, `time.Duration`, `time.Zone` |
| [`hash`](hash.md)        | `use hash;`     | full                                                  | M15.6: `hash.compute(b, algo)` + streaming (`hash.stream`/`update`/`finalize`) for `"md5"`, `"sha1"`, `"sha256"`; struct `hash.Stream`                                                                                              |
| [`crc`](crc.md)          | `use crc;`      | full                                                  | M15.6: `crc.compute(b, algo)` + streaming (`crc.stream`/`update`/`finalize`) for `"crc32"`, `"crc64"`; output is big-endian bytes; struct `crc.Stream`                                                                              |
| [`encoding`](encoding.md) | `use encoding;` | full                                                  | M15.7: introspection (`isAscii`, `lenBytes`, `lenRunes`); binary-to-text `toText`/`fromText` for `"hex"`, `"base64"`, `"base64-url"`; character codecs `encode`/`decode` for `"ascii"`, `"latin-1"`, `"windows-1252"`, `"ebcdic"`     |
| [`task`](task.md)        | `use task;`     | full                                                  | M16.0: observe and join `task of T` handles produced by `spawn { ... }`. `task.wait`, `task.poll`, `task.discard`, `task.waitAll`, `task.waitAny`; pairs with the [user-guide concurrency tour](../user-guide/concurrency.md)        |
| [`fs`](fs.md)            | `use fs;`       | full                                                  | M16.1: filesystem I/O. Whole-file `readString`/`readBytes`/`writeString`/`writeBytes`/`appendString`/`appendBytes`; metadata `exists`/`isFile`/`isDir`/`stat`; dir ops `mkdir`/`mkdirAll`/`remove`/`removeAll`/`rename`/`list`/`walk`; handles `open`/`readLine`/`readChars`/`readBytes`/`writeString`/`writeBytes`/`eof`/`close`; structs `fs.Stat`, `fs.File` |
| [`net`](net.md)          | `use net;`      | [stubs only](../technical/tinygo.md#tinygo-restrictions) | M16.2: TCP `connect`/`listen`/`accept`/`readBytes`/`writeBytes`/`eof`/`address`, UDP `listenUDP`/`sendTo`/`recvFrom`, DNS `lookup`/`reverseLookup`, polymorphic `close`/`address`; structs `net.Conn`, `net.Listener`, `net.UDPSocket`, `net.Datagram`. TinyGo `jennifer` returns friendly errors; use `jennifer-go` for real net I/O. |

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

(Pre-M15.4 `core` was auto-loaded and exposed `len` /
`JENNIFER_VERSION` as bare globals via `RegisterGlobal` /
`RegisterGlobalConst`. M15.4 promoted `len` to a language built-in
keyword, moved version constants to `meta` (M15.1 already had),
and deleted `core`. The `RegisterGlobal*` API surface remains on
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
- Time / instants / durations -> `time` (M15.5+; formatting,
  parsing, and fixed-offset zones land in M15.5.2).
- Cryptographic-style digests (MD5, SHA-1, SHA-256) -> `hash`
  (M15.6). Non-cryptographic checksums (CRC-32, CRC-64) -> `crc`
  (M15.6). The split keeps "transport integrity" and "content
  addressing" visible at the import line.
- Byte / string introspection and character-set codecs (ASCII,
  Latin-1, Windows-1252, EBCDIC IBM-1047) plus hex / base64
  binary-to-text -> `encoding` (M15.7; long-tail codecs parked in M24+).
- Observing / joining background computations launched with `spawn`
  -> `task` (M16.0). Concurrency itself is a language feature
  (the `spawn` keyword + `task of T` type); the library is just the
  observation surface.
- Filesystem I/O (whole-file reads/writes, metadata, directory
  operations, buffered file handles) -> `fs` (M16.1). Blocking on
  purpose; non-blocking use composes with `spawn` from M16.0.
- Network I/O (TCP + UDP sockets, DNS lookups) -> `net` (M16.2).
  Blocking calls, same spawn-composition story as `fs`. The
  TinyGo `jennifer` binary returns friendly "use jennifer-go"
  errors; the network stack lives only in the standard-Go binary.
- A genuinely new topic with **five or more** functions / constants
  -> a new library. Fewer than five names fold into the most-related
  existing library (the non-crypto random helpers were the first
  case the rule caught - they live under `math.rand*` rather than
  getting their own library).
- A single function with no clear topic -> the most-related existing
  library.
- Genuinely polymorphic structural primitives that every program
  needs (`len`) -> language built-in keyword, not a library. The
  bar is intentionally high; `len` is the only one today.

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
