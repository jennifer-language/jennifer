# Jennifer libraries

Jennifer's standard library is split into topic-based libraries, each
enabled explicitly with `use NAME;` (nothing is auto-loaded). This
page catalogs every shipped library and links to its reference doc.

> **Looking for one specific function?** See the
> [cheatsheet](cheatsheet.md): every builtin with its library and a
> one-line description.

> **`len`** is a language built-in primary, not a library
> function: it needs no `use` statement and is polymorphic over
> string / list / map / bytes.

The **TinyGo** column reports whether the library runs in full on
the constrained `jennifer-tiny` binary (TinyGo-built). A `partial`
entry links to [../technical/tinygo.md](../technical/tinygo.md) for
the restriction list; the default `jennifer` binary (standard Go)
always supports the full surface.

`net`'s "stubs only" is about the **stock** `jennifer-tiny` build,
which ships no network driver, not a TinyGo limitation. A tiny build
rebuilt with a network stack runs `net` too; see the
[note on `net` and TinyGo](../technical/tinygo.md#net-on-tinygo-is-a-build-choice-not-a-hard-limit).

| Library                   | Enable with     | TinyGo                                                  | Contents                                                                                                                                                                       |
| ------------------------- | --------------- | ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| [`convert`](convert.md)   | `use convert;`  | full                                                    | `convert.toInt`, `convert.toFloat`, `convert.toString`, `convert.toBool`, `convert.typeOf`, `objectType`, `toCodepoint` / `fromCodepoint`: explicit casts; canonical-only `toBool` conversion                                |
| [`archive`](archive.md)   | `use archive;`  | full                                                    | tar / zip containers over `bytes` (no `fs`). `archive.pack`/`unpack`/`unpackWith` with format `"tar"`/`"zip"`/`"tar.gz"`; bundle is a `list of archive.Entry` `{name, data, mode, mtime}` |
| [`asn1`](asn1.md)         | `use asn1;`     | full                                                    | ASN.1 BER decode / DER encode over an opaque `asn1.Value` (designed like `json`), the byte layer under LDAP / SNMP / PKI. Walk with `decode`/`typeOf`/`tagClass`/`tagNumber`/`get`/`asInt`/`asString`/`asOid`/...; build with `integer`/`octetString`/`oid`/`sequence`/`tagged`/`retag`/`encode`. Hand-rolled. |
| [`binary`](binary.md)     | `use binary;`   | full                                                    | bulk operations on `bytes` (the byte-data counterpart to `strings`/`lists`). `binary.make`, `concat`, `join`, `indexOf`, `contains`, `split`, `startsWith`, `endsWith`, `bitXor`/`bitAnd`/`bitOr`/`bitNot`, fixed-width number reads `readInt`/`readUint`/`readFloat`/`readInts`/`readFloats`: non-mutating, value-semantic; pushes per-byte loops into Go. Named `binary` because `bytes` is a reserved type keyword. |
| [`compress`](compress.md) | `use compress;` | full                                                    | byte-stream compression. `compress.pack`/`unpack` for `"gzip"`/`"zlib"`/`"deflate"` (`bytes` in/out, optional `"fast"`/`"default"`/`"best"` level) + streaming (`compress.stream`/`update`/`finalize`/`discard`); struct `compress.Stream` |
| [`crc`](crc.md)           | `use crc;`      | full                                                    | `crc.compute(algo, b)` + streaming (`crc.stream`/`update`/`finalize`/`discard`) for `"crc32"`, `"crc64"`; output is big-endian bytes; struct `crc.Stream`                          |
| [`crypto`](crypto.md)     | `use crypto;`   | full                                                    | security primitives. Crypto-grade random `crypto.randBytes`/`randInt`; key derivation `crypto.deriveKey` (HKDF, PBKDF2, Argon2id, scrypt); one function per operation, the algorithm first: `encrypt`/`decrypt` (AES-GCM, ChaCha20-Poly1305), `blockEncrypt`/`blockDecrypt` (AES block modes, interop only), `sign`/`verify` (Ed25519, RSA PKCS#1 v1.5 and PSS, ECDSA), `keypair`/`publicKey`, `agree` (X25519), `encapsulate`/`decapsulate` (ML-KEM-768), `wrap`/`unwrap` (RSA-OAEP, sealed box); password hashing `hashPassword`/`verifyPassword`; JWK and CSR. RSA, ECDSA and ML-KEM default binary only. Digests and HMAC live in [`hash`](hash.md). |
| [`decimal`](decimal.md)   | `use decimal;`  | full                                                    | exact base-10 arithmetic over an opaque `decimal.Decimal` (`math/big` coefficient + scale): exact `add` / `sub` / `mul` / `sum` / `pow`, `div` exact or at a named scale, `round` with 7 modes (half-even default), `compare`, `==` by value; operands may be Decimals, ints or decimal strings |
| [`encoding`](encoding.md) | `use encoding;` | full                                                    | introspection (`isAscii`, `lenBytes`, `lenRunes`); binary-to-text `toText`/`fromText` (hex, base32, base64, base85, quoted-printable, URI percent / form); character codecs `encode`/`decode` (UTF-8/16/32, ASCII, ISO-8859-*, Windows-125x, EBCDIC); bounded request parsers `parseForm`, `parseCookies`, `parseMultipart` |
| [`fs`](fs.md)             | `use fs;`       | full                                                    | filesystem I/O. Whole-file `readString`/`readBytes`/`writeString`/`writeBytes`/`appendString`/`appendBytes`/`writeNew`, `readSecretBytes`/`readSecretString`; metadata `exists`/`isFile`/`isDir`/`stat`/`realpath`/`readlink`/`symlink`/`mkfifo`/`chmod`/`chown`; dir ops `mkdir`/`mkdirAll`/`remove`/`removeAll`/`rename`/`list`/`walk`; temp `makeTempFile`/`makeTempDir`; handles `open`/`readLine`/`readChars`/`readBytes`/`writeString`/`writeBytes`/`sync`/`eof`/`close`; `watch`/`next`/`hasEvent`; structs `fs.Stat`, `fs.File`, `fs.Watcher`, `fs.Event` |
| [`hash`](hash.md)         | `use hash;`     | full                                                    | `hash.compute(algo, b)`, `hash.hmac`, constant-time `hash.equal`, + streaming (`hash.stream`/`update`/`finalize`/`discard`) for MD5, SHA-1, SHA-2, SHA-3 and BLAKE2; struct `hash.Stream`                                          |
| [`httpd`](httpd.md)       | `use httpd;`    | default only                                            | HTTP/1.1 server engine over `net/http` (TLS, HTTP/2, graceful shutdown). Pull loop: `httpd.listen`/`accept`/`respond`; request accessors `method`/`path`/`query`/`header`/`body`; `serveFile`/`serveDir`/`shutdown`. `jennifer-tiny` stubs it. |
| [`images`](images.md)     | `use images;`   | full                                                    | raster images over an opaque `images.Image` (pixels kept in Go): `decode` / `encode` PNG, JPEG, GIF (+ header-only `info`, a pixel cap against decode bombs); `crop` / `resize` / `fit` / `rotate` / `flip`; `grayscale` / `threshold` / `dither`; `fill` / `draw` compositing; raw RGBA via `pixels` / `fromPixels` |
| [`intl`](intl.md)         | `use intl;`     | full                                                    | internationalization: `intl.load(lang, catalog)` a `map of string to string`, `intl.setLocale`/`locale`, `intl.tr(key[, params])` with `%name%` interpolation and a locale -> base-language -> default -> key fallback; `intl.formatNumber(locale, value, digits)` with CLDR separators and grouping. Named `intl` (letters-only, like JS `Intl`), not `i18n`. |
| [`io`](io.md)             | `use io;`       | full                                                    | `io.printf`, `io.eprintf`, `io.sprintf`, `io.readLine`, `io.readLines`, `io.readChars`, `io.readBytes`, `io.eof`, plus the format-verb mini-language                                                                                         |
| [`kv`](kv.md)             | `use kv;`       | full                                                    | in-process key/value store with per-key TTL (the no-server local counterpart to `memcache` / `redis`): `kv.open` / `openFile` (persisted) -> `kv.Store`, then `set` / `add` / `get` / `has` / `delete` / `touch` / `incr` / `keys` / `close`, plus `setValue` / `getValue`. Shared handle (safe for `spawn`). Backs `kvstore`'s in-process option. |
| [`json`](json.md)         | `use json;`     | full                                                    | RFC 8259 `json.encode`/`encodePretty`/`decode`. Structs and `map of string to V` map to objects, `bytes` to base64, integral numbers decode to `int` else `float`. Decode yields an opaque `json.Value` walked by JSON Pointer accessors (`typeOf`/`get`/`keys`/`asInt`/...) with a non-mutating write surface; no map-to-struct coercion. |
| [`linalg`](linalg.md)     | `use linalg;`   | full                                                    | linear algebra over `list of float` (vectors) and `list of list of float` (matrices), the companion to `stats`: `linalg.dot`/`distance`/`cross`/`normalize` + `norm`/`scale`/`add`/`sub` (polymorphic over vector or matrix) + `matmul`/`transpose`/`trace`/`determinant`/`inverse`/`solve`/`identity`/`zeros`/`shape`. Direct algorithms (no `gonum`), pure-value; a singular / non-finite result is a catchable error. |
| [`lists`](lists.md)       | `use lists;`    | full                                                    | `lists.push`, `first`, `last`, `reverse`, `sort`, `contains`, `indexOf`, `mismatch`, `concat`, `replicate`, `shuffle`, `range`, plus `map` `filter` `reduce` `find` `any` `all` `sortBy` `sortWith`; the list-returning ones return a new list |
| [`maps`](maps.md)         | `use maps;`     | full                                                    | `maps.keys`, `values`, `has`, `delete`, `without`, `merge`: all return a new map / list / bool                                                                                |
| [`math`](math.md)         | `use math;`     | full                                                    | `math.abs`, `min`, `max`, `sqrt`, `pow`, `floor`, `ceil`, `round`, trigonometry, logarithms, special functions (`gamma`, `erf`, `beta`, ...), `gcd`/`lcm`, `rand`, `randInt`, `randSeed`; constants `math.PI`, `math.E`, `math.TAU`                                                |
| [`meta`](meta.md)         | `use meta;`     | full                                                    | constants `meta.VERSION`, `BUILD`, `SYSMODDIR`, `CAPABILITIES` + `hasCapability`; reflection `call`/`defined`, `callMain`/`definedMain` (entry-program functions by name)                                                                                                             |
| [`net`](net.md)           | `use net;`      | [stubs only](../technical/tinygo.md#tinygo-restrictions) | TCP `connect`/`listen`/`accept`/`readBytes`/`writeBytes`/`eof`/`address`, UDP `listenUDP`/`sendTo`/`recvFrom`, DNS `lookup`/`reverseLookup`, polymorphic `close`/`address`; structs `net.Conn`, `net.Listener`, `net.UDPSocket`, `net.Datagram`. `jennifer-tiny` returns errors; use the default `jennifer` binary for real net I/O. |
| [`os`](os.md)             | `use os;`       | [partial](../technical/tinygo.md#tinygo-restrictions)   | `os.getEnv`/`getEnvBytes`/`setEnv`/`hasEnv`/`unsetEnv`, `argsBytes`, `isTerminal`, `cwd`/`homeDir`/`tempDir`; processes `run`/`runBytes`, `spawn`/`wait`/`waitBytes`/`poll`/`kill`/`release`; signals `catchSignal`/`gotSignal`; constants `os.PLATFORM`, `ARCH`, `NCPU`, `EOL`, `DIRSEP`, `PATHSEP`, `ARGS` |
| [`path`](path.md)         | `use path;`     | full                                                    | OS-aware filesystem path manipulation (the string layer paired with `fs`): `path.base`, `dir`, `ext`, `stem`, `join`, `clean`, `isAbs`, `split`. Pure string transforms, no I/O. |
| [`regex`](regex.md)       | `use regex;`    | full                                                    | regular expressions over `string` (RE2 syntax). `regex.matches`/`find`/`findAll`/`replace`/`split`/`escape` + `regex.Match` struct with positional and named captures. Implicit LRU cache for compiled patterns. |
| [`stats`](stats.md)       | `use stats;`    | full                                                    | descriptive statistics over `list of int` / `list of float`: `stats.mean`/`median`/`mode`/`variance`/`stddev`/`percentile`/`min`/`max`/`sum`/`correlation`, plus distributions and hypothesis tests. Pure-value; `variance`/`stddev` are population (`sampleVariance`/`sampleStddev` for samples). |
| [`ml`](ml.md)             | `use ml;`       | full                                                    | classical / predictive machine learning on tabular data (scikit-learn-lite): fit / predict models `ml.linearRegression`/`ridge`/`kNN`/`naiveBayes`/`logisticRegression`/`decisionTree`/`randomForest`/`kMeans`/`pca` + scalers, applied with `ml.predict`/`transform`; `trainTestSplit`/`kFold`; metrics `accuracy`/`precision`/`recall`/`f1`/`confusionMatrix`/`rocAuc`/`rmse`/`mae`/`r2`. Over `stats`/`linalg`; not a deep-learning framework. |
| [`strings`](strings.md)   | `use strings;`  | full                                                    | `strings.upper`, `lower`, `fold`, `contains`, `count`, `startsWith`, `endsWith`, `indexOf`, `lastIndexOf`, `trim`, `trimLeft`, `trimRight`, `trimPrefix`, `trimSuffix`, `trimSet`, `trimLeftSet`, `trimRightSet`, `replace`, `replicate`, `split`, `chars`, `join`         |
| [`task`](task.md)         | `use task;`     | full                                                    | observe and join `task of T` handles produced by `spawn { ... }`. `task.wait`, `task.poll`, `task.discard`, `task.waitAll`, `task.waitAny`, cancellation (`cancel`/`cancelled`), bounded waits (`waitTimeout`/`waitAnyTimeout`/`waitAllTimeout`), an opt-in allocation budget (`setAllocBudget`/`allocated`); pairs with the [user-guide concurrency tour](../user-guide/concurrency.md) |
| [`channel`](channel.md)   | `use channel;`  | full                                                    | CSP channels between goroutines: `channel.make(capacity)`, `send` (copies the value in), `recv`, `close`, `select` (fan-in), `len`, `capacity`. The streaming counterpart to `task`; a shared conduit that copies the data through it. `channel` is a contextual keyword |
| [`term`](term.md)         | `use term;`     | default only                                            | terminal control for interactive TUIs: raw mode (`term.makeRaw`/`restore` -> `term.State`), `term.size` -> `term.Size{rows, cols}`, raw single-byte reads (`term.readByte` / `readByteTimeout` -> int; `"closed"` at end of input, `"timeout"`). Over `golang.org/x/term`; `jennifer-tiny` stubs it. Output-only TUIs need only `ansi` + `os.isTerminal`. |
| [`testing`](testing.md)   | `use testing;`  | full                                                    | test-runner primitives. `testing.run`/`runArgs`/`results`/`reset`/`report`, assertions `assertEqual`/`assertTrue`/`assertThrows`/..., + `testing.Result` struct. Catches runtime errors, throws, and (uniquely) `exit` inside test bodies. Three report formats: `"text"`, `"tap"`, `"junit"`. Foundation for the .j-side test framework. |
| [`serial`](serial.md)     | `use serial;`   | default only (Linux)                                    | blocking serial-port I/O with termios line config: `serial.open(path, baud)` / `openWith(path, Options)` -> `serial.Port`, `read`/`write`/`flush`/`close`. Linux `/dev` + ioctl; `jennifer-tiny` and non-Linux stub it. |
| [`spi`](spi.md)           | `use spi;`      | default only (Linux)                                    | SPI devices: `spi.open(path)` / `openWith(path, spi.Options)` -> `spi.Device`, full-duplex `transfer(dev, bytes) -> bytes`, `close`. Linux-only; stub elsewhere. |
| [`i2c`](i2c.md)           | `use i2c;`      | default only (Linux)                                    | the I2C bus: `i2c.open(path, addr)` -> `i2c.Bus`, `read`/`write`/`readReg`/`writeReg`/`writeRead`/`close`. Linux-only; stub elsewhere. |
| [`gpio`](gpio.md)         | `use gpio;`     | default only (Linux)                                    | GPIO over `/dev/gpiochipN` (GPIO v2 ioctls): `gpio.open(path)` -> `gpio.Chip`, `setup`/`read`/`write`/`release(chip, pin, ...)`, `close`, + `IN`/`OUT`. Linux-only; stub elsewhere. |
| [`sql`](sql.md)           | `use sql;`      | default only                                            | relational-database client over `database/sql`: MySQL / MariaDB + PostgreSQL (pure-Go drivers). `open`/`query`/`exec`/`next` + typed row accessors, transactions, prepared statements; placeholder-only binding. `jennifer-tiny` stubs it. |
| [`time`](time.md)         | `use time;`     | full                                                    | instant/duration arithmetic, calendar + Unix accessors, fixed-offset zones (`time.zone`, `time.inZone`, `time.UTC`, `time.utc`, `time.local`), strftime format/parse, ISO round-trip; structs `time.Time`, `time.Duration`, `time.Zone` |
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
- Interpreter self-identity (version, build, capabilities) and reflection
  -> `meta`.
- Time / instants / durations -> `time` (formatting,
  parsing, and fixed-offset zones).
- Cryptographic-style digests (MD5, SHA-1, SHA-2, SHA-3, BLAKE2), HMAC and
  constant-time comparison -> `hash`.
  Non-cryptographic checksums (CRC-32, CRC-64) -> `crc`.
  The split keeps "transport integrity" and "content
  addressing" visible at the import line.
- Crypto-grade randomness, key derivation, password hashing, encryption,
  signatures and key exchange -> `crypto`, kept distinct from the digests in
  `hash`.
- Byte / string introspection, character-set codecs (UTF-8/16/32, ASCII,
  ISO-8859-*, Windows-125x, Mac OS Roman, EBCDIC) and binary-to-text (hex, base32, base64,
  ...) -> `encoding`.
- Observing / joining background computations launched with `spawn`
  -> `task`. Concurrency itself is a language feature
  (the `spawn` keyword + `task of T` type); the library is only the
  observation surface.
- Filesystem I/O (whole-file reads/writes, metadata, directory
  operations, buffered file handles) -> `fs`. Blocking on
  purpose; non-blocking use composes with `spawn`.
- Network I/O (TCP + UDP sockets, DNS lookups) -> `net`.
  Blocking calls, composed with `spawn` like `fs`. The **stock**
  `jennifer-tiny` binary returns "use the default `jennifer`" errors
  because it ships no network driver (a build choice, not a TinyGo
  limitation); a tiny build with a network stack runs `net` too.
- Regular expressions over `string` -> `regex`. RE2
  syntax (Go's `regexp` engine); implicit LRU cache. Pure
  string processing, no other library dependencies.
- Test-runner primitives (name-based function dispatch,
  per-process result accumulator, format dispatcher for
  text/TAP/JUnit) -> `testing`. `testing.run` takes the
  function name as a string so a runner can dispatch discovered
  `test*` functions; `jennifer test` builds on these primitives.
- A new topic with **five or more** functions / constants
  -> a new library. Fewer than five names fold into the most-related
  existing library (the non-crypto random helpers live under
  `math.rand*` rather than in their own library).
- A single function with no clear topic -> the most-related existing
  library.
- Polymorphic structural primitives that every program needs
  (`len`) -> language built-in keyword, not a library. The bar is
  high; `len` is the only one.

## Naming convention

`strings` is plural but `math` is singular. The rule:

- **Plural for count nouns**: when the library operates on instances of
  something you can have multiples of: `strings`, `lists`, `maps`.
- **Singular for mass nouns and conceptual wholes**: `math`, `meta`,
  `time`, `regex`.
- **Bare verb when the library is named for what it does**, not what
  it touches: `convert`.
- **Idiomatic abbreviations are fine**: `os`, `fs`, `net`, `regex`.

Three practical constraints reinforce the count/mass rule:

1. Type keywords are reserved. `string`, `int`, `float`, `bool`, `bytes`,
   `list`, `map`, `null` cannot be library names because they
   tokenize as keywords, not identifiers. The plural form (`strings`,
   `lists`, `maps`) sidesteps this.
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
