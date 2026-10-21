# Jennifer libraries

Jennifer's standard library is split into topic-based libraries, each
enabled explicitly with `use NAME;` (nothing is auto-loaded). This
page catalogs every shipped library and links to its reference doc.

> **Looking for one specific function?** See the
> [cheatsheet](cheatsheet.md) - alphabetical list of every builtin
> with its library and a one-line description.

> **`len`** is a language built-in primary, not a library
> function: it needs no `use` statement and is polymorphic over
> string / list / map / bytes.

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
| [`asn1`](asn1.md)         | `use asn1;`     | full                                                    | ASN.1 BER decode / DER encode over an opaque `asn1.Value` (designed like `json`), the byte layer under LDAP / SNMP / PKI. Walk with `decode`/`typeOf`/`tagClass`/`tagNumber`/`get`/`asInt`/`asString`/`asOid`/...; build with `integer`/`octetString`/`oid`/`sequence`/`tagged`/`retag`/`encode`. Hand-rolled, dependency-free. |
| [`binary`](binary.md)     | `use binary;`   | full                                                    | bulk operations on `bytes` (the byte-data counterpart to `strings`/`lists`). `binary.make`, `concat`, `join`, `slice`, `indexOf`, `contains`, `split`, `startsWith`, `endsWith` - non-mutating, value-semantic; pushes per-byte loops into Go for throughput. Named `binary` because `bytes` is a reserved type keyword. |
| [`compress`](compress.md) | `use compress;` | full                                                    | byte-stream compression. `compress.pack`/`unpack` for `"gzip"`/`"zlib"`/`"deflate"` (`bytes` in/out, optional `"fast"`/`"default"`/`"best"` level) + streaming (`compress.stream`/`update`/`finalize`); struct `compress.Stream` |
| [`crc`](crc.md)           | `use crc;`      | full                                                    | `crc.compute(b, algo)` + streaming (`crc.stream`/`update`/`finalize`) for `"crc32"`, `"crc64"`; output is big-endian bytes; struct `crc.Stream`                          |
| [`crypto`](crypto.md)     | `use crypto;`   | full                                                    | security primitives. Crypto-grade random `crypto.randBytes`/`randInt`; constant-time `crypto.hmacEqual`; key derivation `crypto.hkdf` (HKDF) / `crypto.pbkdf2` (PBKDF2), `algo` `"sha1"`/`"sha256"`/`"sha512"`; AES-256-GCM `encrypt`/`decrypt`; Ed25519 `signKeypair`/`sign`/`verify`; RSA / ECDSA PEM signatures, key generation, JWK and CSR (default binary only); ML-KEM-768 `mlkemKeypair`/`mlkemEncapsulate`/`mlkemDecapsulate` (default binary only). Digests and HMAC live in [`hash`](hash.md). |
| [`encoding`](encoding.md) | `use encoding;` | full                                                    | introspection (`isAscii`, `lenBytes`, `lenRunes`); binary-to-text `toText`/`fromText` for `"hex"`, `"base64"`, `"base64-url"`; character codecs `encode`/`decode` for `"ascii"`, `"iso-8859-1"`, `"windows-1252"`, `"ebcdic"` |
| [`fs`](fs.md)             | `use fs;`       | full                                                    | filesystem I/O. Whole-file `readString`/`readBytes`/`writeString`/`writeBytes`/`appendString`/`appendBytes`; metadata `exists`/`isFile`/`isDir`/`stat`; dir ops `mkdir`/`mkdirAll`/`remove`/`removeAll`/`rename`/`list`/`walk`; handles `open`/`readLine`/`readChars`/`readBytes`/`writeString`/`writeBytes`/`eof`/`close`; structs `fs.Stat`, `fs.File` |
| [`hash`](hash.md)         | `use hash;`     | full                                                    | `hash.compute(b, algo)`, `hash.hmac`, constant-time `hash.equal`, + streaming (`hash.stream`/`update`/`finalize`/`discard`) for `"md5"`, `"sha1"`, `"sha256"`, `"sha384"`, `"sha512"`; struct `hash.Stream`                                          |
| [`httpd`](httpd.md)       | `use httpd;`    | default only                                            | HTTP/1.1 server engine over `net/http` (TLS, HTTP/2, graceful shutdown). Pull loop: `httpd.listen`/`accept`/`respond`; request accessors `method`/`path`/`query`/`header`/`body`; `serveFile`/`serveDir`/`shutdown`. `jennifer-tiny` returns a friendly stub. |
| [`intl`](intl.md)         | `use intl;`     | full                                                    | internationalization: `intl.load(lang, catalog)` a `map of string to string`, `intl.setLocale`/`locale`, `intl.tr(key[, params])` with `%name%` interpolation and a locale -> base-language -> default -> key fallback. Named `intl` (letters-only, like JS `Intl`), not `i18n`. |
| [`io`](io.md)             | `use io;`       | full                                                    | `io.printf`, `io.sprintf`, `io.readLine`, `io.eof`, plus the format-verb mini-language                                                                                         |
| [`kv`](kv.md)             | `use kv;`       | full                                                    | in-process key/value store with per-key TTL (the no-server local counterpart to `memcache` / `redis`): `kv.open` / `openFile` (persisted) -> `kv.Store`, then `set` / `add` / `get` / `has` / `delete` / `touch` / `incr` / `close`. Shared handle (safe for `spawn`). Backs `kvstore`'s in-process option. |
| [`json`](json.md)         | `use json;`     | full                                                    | RFC 8259 `json.encode`/`encodePretty`/`decode`. Structs and `map of string to V` map to objects, `bytes` to base64, integral numbers decode to `int` else `float`. Decode yields an opaque `json.Value` walked by JSON Pointer accessors (`typeOf`/`get`/`keys`/`asInt`/...) with a non-mutating write surface; no map-to-struct coercion. |
| [`linalg`](linalg.md)     | `use linalg;`   | full                                                    | linear algebra over `list of float` (vectors) and `list of list of float` (matrices), the companion to `stats`: `linalg.dot`/`distance`/`cross`/`normalize` + `norm`/`scale`/`add`/`sub` (polymorphic over vector or matrix) + `matmul`/`transpose`/`trace`/`determinant`/`inverse`/`solve`/`identity`/`zeros`/`shape`. Direct algorithms (no `gonum`), pure-value; a singular / non-finite result is a catchable error. |
| [`lists`](lists.md)       | `use lists;`    | full                                                    | `lists.push`, `pop`, `first`, `last`, `head`, `tail`, `reverse`, `sort`, `contains`, `indexOf`, `mismatch`, `concat`, `slice`, `shuffle`, `range`, plus `map` `filter` `reduce` `find` `any` `all` `sortBy`; the list-returning ones return a new list |
| [`maps`](maps.md)         | `use maps;`     | full                                                    | `maps.keys`, `values`, `has`, `delete`, `merge` - all return a new map / list / bool                                                                                           |
| [`math`](math.md)         | `use math;`     | full                                                    | `math.abs`, `min`, `max`, `sqrt`, `pow`, `floor`, `ceil`, `round`, `rand`, `randInt`, `randSeed`; constants `math.PI`, `math.E`                                                |
| [`meta`](meta.md)         | `use meta;`     | full                                                    | constants `meta.VERSION`, `BUILD`, `SYSMODDIR`, `CAPABILITIES` + `hasCapability`; reflection `call`/`defined`, `callMain`/`definedMain` (entry-program functions by name)                                                                                                             |
| [`net`](net.md)           | `use net;`      | [stubs only](../technical/tinygo.md#tinygo-restrictions) | TCP `connect`/`listen`/`accept`/`readBytes`/`writeBytes`/`eof`/`address`, UDP `listenUDP`/`sendTo`/`recvFrom`, DNS `lookup`/`reverseLookup`, polymorphic `close`/`address`; structs `net.Conn`, `net.Listener`, `net.UDPSocket`, `net.Datagram`. `jennifer-tiny` returns friendly errors; use the default `jennifer` binary for real net I/O. |
| [`os`](os.md)             | `use os;`       | [partial](../technical/tinygo.md#tinygo-restrictions)   | `os.getEnv`/`setEnv`, `hasFlag`/`flag`, `isTerminal`, `cwd`/`homeDir`/`tempDir`; processes `run`, `spawn`/`wait`/`poll`/`kill`/`release`; signals `catchSignal`/`gotSignal`; constants `os.PLATFORM`, `ARCH`, `NCPU`, `EOL`, `DIRSEP`, `PATHSEP`, `ARGS` |
| [`path`](path.md)         | `use path;`     | full                                                    | OS-aware filesystem path manipulation (the string layer paired with `fs`): `path.base`, `dir`, `ext`, `stem`, `join`, `clean`, `isAbs`, `split`. Pure string transforms, no I/O. |
| [`regex`](regex.md)       | `use regex;`    | full                                                    | regular expressions over `string` (RE2 syntax). `regex.matches`/`find`/`findAll`/`replace`/`split`/`escape` + `regex.Match` struct with positional and named captures. Implicit LRU cache for compiled patterns. |
| [`stats`](stats.md)       | `use stats;`    | full                                                    | descriptive statistics over `list of int` / `list of float`: `stats.mean`/`median`/`mode`/`variance`/`stddev`/`percentile`/`min`/`max`/`sum`/`correlation`. Pure-value, dependency-free; `variance`/`stddev` are population. |
| [`ml`](ml.md)             | `use ml;`       | full                                                    | classical / predictive machine learning on tabular data (scikit-learn-lite): fit / predict models `ml.linearRegression`/`ridge`/`kNN`/`naiveBayes`/`logisticRegression`/`decisionTree`/`randomForest`/`kMeans`/`pca` + scalers, applied with `ml.predict`/`transform`; `trainTestSplit`/`kFold`; metrics `accuracy`/`precision`/`recall`/`f1`/`confusionMatrix`/`rocAuc`/`rmse`/`mae`/`r2`. Over `stats`/`linalg`; not a deep-learning framework. |
| [`strings`](strings.md)   | `use strings;`  | full                                                    | `strings.upper`, `lower`, `contains`, `startsWith`, `endsWith`, `indexOf`, `trim`, `trimLeft`, `trimRight`, `replace`, `repeat`, `substring`, `split`, `chars`, `join`         |
| [`task`](task.md)         | `use task;`     | full                                                    | observe and join `task of T` handles produced by `spawn { ... }`. `task.wait`, `task.poll`, `task.discard`, `task.waitAll`, `task.waitAny`, cancellation (`cancel`/`cancelled`), bounded waits (`waitTimeout`/`waitAnyTimeout`); pairs with the [user-guide concurrency tour](../user-guide/concurrency.md) |
| [`channel`](channel.md)   | `use channel;`  | full                                                    | CSP channels between goroutines: `channel.make(capacity)`, `send` (copies the value in), `recv`, `close`, `select` (fan-in), `len`, `capacity`. The streaming counterpart to `task`; a shared conduit that copies the data through it. `channel` is a contextual keyword |
| [`term`](term.md)         | `use term;`     | default only                                            | terminal control for interactive TUIs: raw mode (`term.makeRaw`/`restore` -> `term.State`), `term.size` -> `term.Size{rows, cols}`, raw single-byte reads (`term.readByte` -> int, `-1` at EOF). Over `golang.org/x/term`; `jennifer-tiny` returns a friendly stub. Output-only TUIs need only `ansi` + `os.isTerminal`. |
| [`testing`](testing.md)   | `use testing;`  | full                                                    | test-runner primitives. `testing.run`/`results`/`reset`/`report` + `testing.Result` struct. Catches runtime errors, throws, and (uniquely) `exit` inside test bodies. Three report formats: `"text"`, `"tap"`, `"junit"`. Foundation for the .j-side test framework. |
| [`serial`](serial.md)     | `use serial;`   | default only (Linux)                                    | blocking serial-port I/O with termios line config: `serial.open(path, baud)` / `openWith(path, Options)` -> `serial.Port`, `read`/`write`/`flush`/`close`. Linux `/dev` + ioctl; `jennifer-tiny` and non-Linux return a friendly stub. |
| [`spi`](spi.md)           | `use spi;`      | default only (Linux)                                    | SPI devices: `spi.open(path)` -> `spi.Device`, `configure(dev, mode, speedHz)`, full-duplex `transfer(dev, bytes) -> bytes`, `close`. Linux-only; stub elsewhere. |
| [`i2c`](i2c.md)           | `use i2c;`      | default only (Linux)                                    | the I2C bus: `i2c.open(path, addr)` -> `i2c.Bus`, `read`/`write`/`readReg`/`writeReg`/`close`. Linux-only; stub elsewhere. |
| [`gpio`](gpio.md)         | `use gpio;`     | default only (Linux)                                    | GPIO over `/dev/gpiochipN` (GPIO v2 ioctls), pin-keyed `setup`/`read`/`write`/`release` + `IN`/`OUT` (mirrors the sysfs `gpio` module). Linux-only; stub elsewhere. |
| [`sql`](sql.md)           | `use sql;`      | default only                                            | relational-database client over `database/sql`: MySQL / MariaDB + PostgreSQL (pure-Go drivers). `open`/`query`/`exec`/`next` + typed row accessors, transactions, prepared statements; placeholder-only binding. First heavyweight library dependency; `jennifer-tiny` stubs it. |
| [`time`](time.md)         | `use time;`     | full                                                    | instant/duration arithmetic, calendar + Unix accessors, fixed-offset zones (`time.zone`, `time.inZone`, `time.UTC`, `time.local`), strftime format/parse, ISO round-trip; structs `time.Time`, `time.Duration`, `time.Zone` |
| [`toml`](toml.md)         | `use toml;`     | full                                                    | RFC-conformant TOML 1.0 `toml.encode`/`encodePretty`/`decode`. Same opaque `toml.Value` + read / walk / write surface as `json`, name for name, addressed by JSON Pointer; adds `toml.asDatetime` (backed by `time.Time`) for TOML's native date-times. |
| [`uuid`](uuid.md)         | `use uuid;`     | full                                                    | RFC 9562 UUIDs. `uuid.v4()` (random) / `uuid.v7()` (time-ordered) + `parse`/`isValid`/`version` + constant `NIL`. The version is a function; draws from the `crypto` library's crypto-grade random source (unguessable, safe as a security token). |
| [`xml`](xml.md)           | `use xml;`      | full                                                    | XML `xml.decode`/`encode`/`encodePretty` over an opaque `xml.Value`, designed like `json`/`toml`. Element tree with ordered attributes + children + mixed text; accessors `tag`/`text`/`attr`/`attrs`/`children`, XPath-style `get`/`findAll`/`has`, and a build surface `element`/`setAttr`/`setText`/`append`. |
| [`yaml`](yaml.md)         | `use yaml;`     | full                                                    | YAML 1.2 `yaml.decode`/`decodeAll`/`encode`/`encodePretty` over an opaque `yaml.Value`, same read / walk / write surface as `json`/`toml`, addressed by JSON Pointer. Anchors / aliases / `<<` merge keys, multi-document streams, `yaml.asDatetime` for timestamps; flow (compact) vs block (pretty) encode. Backed by `gopkg.in/yaml.v3` (TinyGo-clean). |

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
is also freed for use as an ordinary identifier, like Python's
`import foo as bar`.

(The removed `core` library exposed `len` / `JENNIFER_VERSION` as bare
globals via `RegisterGlobal` / `RegisterGlobalConst`; `len` is now a
language built-in keyword and the version constants live in `meta`.
The `RegisterGlobal*` API remains on `Interpreter`, unused by any
shipping library, pending removal.)

## How libraries are organized

The standard library favors many small, focused libraries over a few
large ones. Where a new name belongs:

- Touches I/O (stdin/stdout/files/network/clock) -> `io`; files, sockets, and the
  clock have their own libraries (`fs`, `net`, `time`, below).
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
- Cryptographic-style digests (MD5, SHA-1, SHA-2) and HMAC -> `hash`.
  Non-cryptographic checksums (CRC-32, CRC-64) -> `crc`.
  The split keeps "transport integrity" and "content
  addressing" visible at the import line.
- Crypto-grade randomness, constant-time comparison, and key
  derivation (HKDF, PBKDF2) -> `crypto`. The security primitives
  that need a secure source or a timing-safe operation, kept
  distinct from the digests in `hash`.
- Byte / string introspection and character-set codecs (ASCII,
  ISO-8859-1, Windows-1252, EBCDIC IBM-1047) plus hex / base64
  binary-to-text -> `encoding` (long-tail codecs parked for later).
- Observing / joining background computations launched with `spawn`
  -> `task`. Concurrency itself is a language feature
  (the `spawn` keyword + `task of T` type); the library is only the
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
- Test-runner primitives (name-based function dispatch,
  per-process result accumulator, format dispatcher for
  text/TAP/JUnit) -> `testing`. `testing.run` takes the
  function name as a string so a runner can dispatch discovered
  `test*` functions; the `.j`-side
  assertion vocabulary and CLI harness ship on top
  of these primitives.
- A new topic with **five or more** functions / constants
  -> a new library. Fewer than five names fold into the most-related
  existing library (the non-crypto random helpers were the first
  case the rule caught - they live under `math.rand*` rather than
  getting their own library).
- A single function with no clear topic -> the most-related existing
  library.
- Polymorphic structural primitives that every program needs
  (`len`) -> language built-in keyword, not a library. The bar is
  high; `len` is the only one.

## Naming convention

`strings` is plural but `math` is singular. The rule:

- **Plural for count nouns**: when the library operates on instances of
  something you can have multiples of. `strings`, `lists`, `maps`,
  `bytes`, `files`.
- **Singular for mass nouns and conceptual wholes**: `math`, `meta`,
  `time`, `regex`.
- **Bare verb when the library is named for what it does**, not what
  it touches: `convert`.
- **Idiomatic abbreviations are fine**: `os`, `fs`, `net`, `regex`.

Three practical constraints reinforce the count/mass rule:

1. Type keywords are reserved. `string`, `int`, `float`, `bool`,
   `list`, `map`, `null` cannot be library names because they
   tokenize as type tokens, not IDENTs. The plural form (`strings`,
   `lists`, `maps`) sidesteps this naturally.
2. The rule matches Go's stdlib (the interpreter's host language):
   `strings` and `bytes` are plural; `math`, `io`, `os` are singular.

3. Within a library, function names are lowercase / camelCase
   (`upper`, `startsWith`, `typeOf`). Constants are uppercase
   (`PI`, `E`, `VERSION`, `PLATFORM`).

For implementation notes on how libraries register themselves with the
interpreter (`RegisterNamespaced`, the `use`-gated lookup), see
[../technical/interpreter.md > Builtins and libraries](../technical/interpreter.md#builtins-and-libraries).

For canonical terminology (library vs module, function vs method,
list vs array, ...), see [../glossary.md](../glossary.md). This page
uses the terms in that table.
