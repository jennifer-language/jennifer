# Changelog

User-visible changes to the Jennifer interpreter, its built-in libraries and
its bundled modules in the current release. Earlier releases are described in
the release notes of each tag at
<https://github.com/jennifer-language/jennifer/releases>. Versions are the git
tags (bare semver, no `v`). Each `(Mxx.n)` reference points at the entry in
[docs/roadmap.md](docs/roadmap.md) (or [docs/milestones.md](docs/milestones.md)
once the release is tagged) that explains the change in depth.


## 0.28.0

Milestone M29. The nine deprecated single-vendor modules leave the bundle for
decks (M29.33), every `@jennifer` deck moves to `@deckyard` (M29.55), and the
names and builtins deprecated in 0.27.0 are gone (M29.34, M29.35). The stance
review's fixes make the libraries agree: their own error kind for bad input
data (M29.21), verified TLS for `sql` (M29.36), default deadlines in `net`
(M29.37), one shape for caps, timeouts and closed handles (M29.44-46), and
validated UTF-8 from the OS (M29.50). A `Value` is 48 bytes instead of 272
(M29.11), module calls retag nothing (M29.14), and string scans on non-ASCII
text are linear (M29.13). New are `binary` writers (M29.54), bounded child
processes (M29.57), a per-task allocation budget (M29.3), a build-time install
prefix and resolver files (M29.4, M29.5), five more release targets (M29.6),
TrueType fonts in `doctree` PDFs (M29.20), a WSDL reader in `soap`, and a
profiler for servers (M29.30-32).

### Breaking changes

- **Nine bundled modules removed** (M29.33): `gotify`, `slack`, `discord`,
  `telegram`, `prometheus`, `statsd`, `influxdb`, `mikrotik` and `mactelnet`.
  Each is a deck with the same surface: add `@deckyard/<name>` to
  `deck.toml`, run `jvc install` and write `import "@deckyard/<name>/" as
  <name>;`. What the modules used stays in the bundle: `multipart`,
  `crypto.mtwei*`, `net.bindToDevice` and `http.requestRawBody` (the last
  deprecated, see below).
- **`@jennifer/*` decks are now `@deckyard/*`** (M29.55): discord, gotify,
  influxdb, mactelnet, mikrotik, oauthext, prometheus, slack, statsd and
  telegram. Imports read `@deckyard/<name>/`; the old versions are yanked and
  `jvc add` / `install` / `update` refuse a moved deck, naming the new one
  (jvc 0.3.0, which the container images now bundle). `@jennifer` keeps no
  decks.
- **Deprecated names removed** (M29.34): `orm.hasOne` / `hasMany` (use
  `oneToOne` / `oneToMany`), `validate.isInt` / `isFloat` / `isBool`
  (`integer` / `number` / `boolean`), `rest.insecure` / `graphql.insecure`,
  `soap` / `webdav` `withInsecure` (`withSkipVerify`) and `withCaCert`
  (`withCA`).
- **Deprecated builtins removed** (M29.35); `jennifer lint` reports a
  leftover call as `L302` with the replacement: `strings.substring`,
  `lists.slice` / `head` / `tail` / `pop`, `binary.slice` (slices),
  `strings.repeat` (`replicate`), `testing.runWith` / `assertThrowsWith`
  (`runArgs` / `assertThrowsArgs`), `convert.bytesFromString` /
  `stringFromBytes` (`encoding.encode` / `decode`), `crypto.hkdf` / `pbkdf2` /
  `argon2id` / `scrypt` (`deriveKey`), `convert.toString(n, radix)`
  (`"%d|base="`), the two-bound `lists.range(a, b)` (`a..b`), and `io.printf`
  / `sprintf` / `eprintf` with one non-string value (`"%v"`).
- **Network halves move** (M29.17): `kvstore.memcacheStore(mc)` /
  `redisStore(rc)` are `memcache.kvStore(mc)` / `redis.kvStore(rc)`, and the
  `Store.Memcache` / `Store.Redis` variants are gone. `webhook.send` is
  removed: `http.post(url, "application/json", payload,
  webhook.signedHeaders(payload, secret))`.
- **Data errors carry the library's kind** (M29.21): bad input data read by a
  built-in library throws `"json"`, `"xml"`, `"toml"`, `"yaml"`, `"asn1"`,
  `"encoding"`, `"compress"`, `"archive"`, `"images"`, `"crypto"` (a failed
  `decrypt` / `unwrap` included), `"time"`, `"uuid"`, `"convert"`, `"kv"`, and
  `"fs"` / `"io"` / `"os"` for invalid UTF-8 read as text; it was
  `"runtime"`. Misuse stays `"runtime"`; messages are unchanged.
- **`web` answers a malformed body 400** (M29.29): `form`, `formValue`,
  `bodyJson` and `multipartForm` throw `web.WebError{kind: "web", message,
  status}` (was `"encoding"`, `"runtime"` or `"multipart"`); a body parser's
  cap is a `WebError` of kind `"limit"`, status 413. An escaped `WebError` is
  answered with its status. `webapi` reads a body only by its `Content-Type`
  (JSON or urlencoded form).
- **`sql`** (M29.36): `open` requires verified TLS (`sslmode=verify-full`,
  MySQL `tls=true`, `verify-ca` only with `sslrootcert`) and refuses a weaker
  mode or `allowFallbackToPlaintext` unless `openWith(driver, dsn,
  sql.Options{allowInsecure})`; a Unix socket and loopback are exempt,
  `localhost` is never looked up. Server errors are `sql.SqlError{kind:
  "sql", message, code, sqlState}`, deadlines `"timeout"`, transport failures
  `net.NetError`. `asFloat` refuses NaN and infinity, `asBool` takes only 0 /
  1.
- **`net`** (M29.37): a connect, TLS handshake or lookup without a timeout
  gives up after 30 s (`"timeout"`); `connect(addr, 0)` no longer means no
  deadline. Use `math.MAX_INT` for none.
- **`kv.incr`** on an absent or expired key starts at 0 and stores the result
  (was `-1`) (M29.38).
- **`ml.Model` is a value** (M29.39): no `$m.id`, no `ml.Model{...}` literal,
  `%v` prints `<ml.Model kind>`.
- **Integers outside int64** (M29.40): `json` and `yaml` keep every digit;
  `asInt` / `asInts` refuse them and `asFloat` / `asFloats` refuse any
  integer a float does not hold exactly, naming `asDecimal`.
- **Strict decoding:** `compress.unpack` zlib / deflate refuse any byte after
  the stream (M29.41); quoted-printable refuses a malformed `=` escape
  (M29.42); `yaml` refuses a float past float64's range and reads `!!float`
  as a float; `archive` refuses a drive-letter member name, a tar without its
  end marker and data after the end.
- **Caps:** a negative cap throws: `archive.unpackWith` (was unlimited),
  `http.Options` fields (`maxBytes: -1` was unlimited; also through the
  deprecated `requestWith` family, where a negative `timeoutMs` was none) and
  `webdav.get` (M29.43, M29.44).
- **Timeouts** (M29.45): `term.readByte` / `readByteTimeout` throw `"closed"`
  at the end of input and `readByteTimeout` `"timeout"` (were `-1` / `-2`);
  `time.sleep` with a negative Duration throws.
- **Closed handles** (M29.46): any use of a closed or consumed handle (`fs`,
  `kv`, `serial`, `i2c`, `spi`, `gpio`, `term`, `channel`, `crc`, `net`,
  `httpd`, `sql`, `compress`, `hash`, and an `os.Process` after `os.release`)
  throws `"closed"` (was `"runtime"` in most); a second `close` / `shutdown` / `discard` / `rollback` does nothing
  (threw in `fs`, `net`, `term`, `channel`); a repeated `finalize` or `commit`
  throws `"closed"`.
- **`toml.asDatetime`** refuses a local date, time or date-time; read it with
  `asLocal` (M29.47).
- **Hardware** (M29.49): a `serial` byte with a parity or framing error
  throws `"serial"` (was `0x00`); `i2c` reads and writes over 8192 bytes throw
  `"limit"` (were clamped); `serial.write` returns `null`.
- **Outside text is UTF-8** (M29.50): `os.getEnv`, `os.ARGS`, `fs.list`,
  `walk`, `readlink`, `realpath` and `sql.asString` throw for invalid bytes,
  naming the offset; the bytes forms read them.
- **`crypto.jwkToPem`** returns `bytes` (M29.51).
- **Consistency fixes** (M29.52): `strings.replace` with an empty `old`
  throws; `intl.setLocale` needs a loaded catalog and kind `"intl"` is gone
  (misuse is `"runtime"`); `meta.hasCapability` throws on an unknown name; a
  `kv.openFile` flush throws `"kv"` when another writer changed the file;
  `httpd.serveDir` answers 404 for a symlink out of `root` unless
  `Options.followSymlinks`; `%f|prec` rounds the shortest decimal half to
  even (`1002.675` prints `1002.68`, `2.345` prints `2.34`).
- **Scope** (M29.9): reading a global through a hoisted function before its
  `def` ran throws (was `null`); writing to a `match` binder is a parse
  error.
- A module's own struct prints with its stem inside the module too
  (`geo.Point{x: 1}`) (M29.14).
- **Lower caps:** the lexer takes 1,048,576 tokens per file (was 4,194,304);
  on `jennifer-tiny` split / chars / `readLines` parts 262,144 (was
  1,048,576) and Argon2id / scrypt memory 128 MiB; scrypt `p` at most 4 (was
  16; `verifyPassword` still takes a stored hash up to 16); `regex.findAll` at most 838,860 matches for a group-less pattern (was
  2,097,152); `html` nesting 256 (was 512); `markdown`, `rst` and `myst`
  sources over 4,194,304 characters, `rst` over 100,000 nodes (was
  200,000); `barcode` `quiet` 0-16 (was 0-100).

### Deprecated

Removed in 0.29.0 (M30.25); `jennifer lint` reports each use as `L301`:

| Deprecated | Use |
|---|---|
| `http.requestWith`, `requestTls`, `requestWithTls`, `requestRawBody`, `requestRawBodyTls`, `requestBytes`, `requestWithBytes` | `send` / `sendBytes` (M29.43) |
| `http.Options.tls`, `http.TlsOptions` (docs only) | `Options.tlsOptions as net.TLSOptions` (M29.43) |
| `net.connect` / `connectTLS` / `startTLS` trailing timeout | `connectWith` / `startTLSWith` (M29.37) |
| `net.readAll` / `readN` trailing idle timeout | `net.setIdleTimeout` (M29.45) |
| `net.eof` | an empty `readBytes` (M29.46) |
| `sql` drivers `mariadb` / `postgresql`, `sql.setQueryTimeout` | `mysql` / `postgres`, `Options.queryTimeoutMs` (M29.36) |
| positional caps of `compress.unpack` / `images.decode`, `archive` `"tgz"` | `unpackWith` / `decodeWith`, `"tar.gz"` (M29.44) |
| `fs.watch` interval, `os.spawn` options map | `fs.watchWith`, `os.spawnWith` (M29.45) |
| two-argument `xml.decode` | `xml.decodeWith` (M29.47) |
| `ml.free` (a no-op), the trailing hyper-parameters | the `*With` trainers (M29.39) |
| `stats.multipleRegression`, `convert.toInt(float)`, `os.hasFlag` / `flag`, `decimal.toString`, `images.rgba` | `ml.linearRegression`, `math.trunc` / `round` / `floor` / `ceil`, the `args` module, `convert.toString` / `%v`, `images.Color{...}` (M29.48) |
| `httpd.listenTLS` / `listenTLSWith` / `serveFileEtag` / `serveDirEtag` | `Options.certPem` / `keyPem` / `etag` (M29.48) |
| the `gpio` module | the `gpio` library's `Chip` (M29.48) |
| `spi.configure`, `gpio.chip`, `JENNIFER_GPIO_CHIP`, pin-only `gpio` calls | `spi.openWith`, `gpio.open` (M29.49) |
| `crypto` `"rsa-sha256"` / `384` / `512`, a `string` password | `"rsa-pkcs1-sha*"`, `bytes` (M29.51) |
| `time.fromUnixMillis` / `unixMillis` | `fromUnixMilliseconds` / `unixMilliseconds` (M29.52) |

### Language and interpreter

- `Value` is 48 bytes (was 272): a 921,600-element `list of float` peaks at
  170-200 MB instead of about 1 GB, and the benchmark suite runs in half the
  time (M29.11). The element caps rise to 16,777,216 (`MaxRangeElements`) and
  4,194,304 (channel capacity, matrix elements). On `jennifer-tiny` the
  latter two are 262,144 and its call depth cap is 400 (was 120); the default
  binary's stays 10,000.
- A module's struct has one identity on both sides of its boundary, so a call
  rebuilds nothing: 200 lookups on a 6,400-node module tree went from 1.1 s
  to 0 ms (M29.14).
- `strings.indexOf(s, sub, from)` and slices of a non-ASCII string resume
  from the last position: a forward scan is linear (M29.13).
- A parameter may shadow a global (M29.9).
- The interpreter sets Go's soft memory limit to 90% of the machine or cgroup
  limit (`GOMEMLIMIT` wins) (M29.3).
- `make PREFIX=<dir>` sets the module, docs and `etc` paths; `jennifer
  version -v` prints them with their source (M29.4).
- Name resolution reads `JENNIFER_RESOLV_CONF` / `JENNIFER_HOSTS`, then
  `<PREFIX>/etc`, then `/etc`; with no configuration a failed lookup names the
  files looked for (M29.5).

### Standard library

Libraries (`use NAME;`):

- **`binary`**: `fromInt` / `fromUint` / `fromFloat`, `fromInts` /
  `fromFloats` and `setInt` / `setUint` / `setFloat` write fixed-width
  numbers, mirroring the readers (M29.54).
- **`os`**: `SpawnOptions` gains `dir`, `env`, `cleanEnv` and `group` (a new
  process group); `signal(p, name)` signals a child or its group (M29.57).
  `waitTimeout` / `waitBytesTimeout`, `spawnWith` (M29.45); `getEnvBytes`,
  `argsBytes()` (M29.50).
- **`fs`**: `syncDir(path)`; `seek(f, offset)` / `tell(f)` (M29.57).
  `watchWith(path, WatchOptions)` (M29.45); `listBytes`, `walkBytes`
  (M29.50).
- **`net`**: `connectWith`, `startTLSWith` with `ConnectOptions{timeoutMs,
  tls, tlsOptions}`, `lookupWith`, `reverseLookupWith` with
  `LookupOptions{timeoutMs, dnsServers}` (M29.37, M29.5); `setIdleTimeout(conn, ms)` (M29.45). A name in a
  chosen hosts file wins over the search list; a `dnsServers` reply must
  echo the query.
- **`sql`**: `openWith(driver, dsn, Options{allowInsecure, queryTimeoutMs})`,
  `SqlError` (M29.36).
- **`task`**: `setAllocBudget(bytes)` / `allocatedBytes()`, a catchable `"limit"`
  past it (M29.3); `waitAllTimeout`. **`channel`**: `selectTimeout` (M29.45).
- **`json`, `toml`, `yaml`**: `asDecimal(v[, ptr])` (M29.40). **`toml`,
  `yaml`, `xml`, `asn1`**: `decodeWith(s, DecodeOptions)`; `yaml.asBytes`,
  `toml.asLocal` (M29.47).
- **`compress`**: `unpackWith(..., UnpackOptions)`. **`images`**:
  `decodeWith(..., DecodeOptions)`. **`math`**: `MAX_INT`, `MIN_INT` (M29.44).
- **`encoding`**: `"macintosh"` (Mac OS Roman) (M29.56).
- **`crypto`**: `"rsa-pkcs1-sha*"` and `"rsa-pss-sha*"` in `sign` /
  `verify`; `deriveKey`'s HKDF / PBKDF2 take the SHA-2, SHA-3 and BLAKE2
  names; weak algorithms are tagged `(weak)` in name lists (M29.51).
- **`time`**: `fromUnixMilliseconds`, `unixMilliseconds` (M29.52).
- **`httpd`**: `Options.certPem` / `keyPem` / `etag` (M29.48),
  `followSymlinks` (M29.52).
- **`spi`**: `openWith(path, Options)`. **`gpio`**: `open(path) -> Chip`,
  chip-first calls, `close` (M29.49).
- **`linalg`**: `matmul`, `determinant`, `inverse`, `solve` stop on
  `task.cancel`; **`ml`**: `pca` too, at most 256 features.

Modules (`import "NAME.j";`):

- **`kvstore`, `session`, `ratelimit`, `webhook`** load on `jennifer-tiny`;
  new `s3presign` (presigning without `net`), `webhook.signedHeaders`
  (M29.17).
- **`http`**: `sendBytes`; `Options.tlsOptions` (M29.43).
- **`web`**: `allocBudget($app, bytes)` per request (M29.3);
  `onServerError($app, handler)`, called only for errors answered 5xx.
- **`soap`**: a WSDL 1.1 reader: `parseWsdl`, `parseWsdlWith`, `wsdlCall`,
  `wsdlCallAt`, `wsdlCallTo`, `wsdlConnect`, `wsdlRequest`. `parse` and the
  server refuse a misplaced `Header`, a header block in no namespace and
  text in the Envelope or Body.
- **`doctree`**: TrueType faces per role with fallbacks; `layoutPdf` returns
  the glyphs no face has (M29.20). Bookmarks, images, fonts and pages go to
  `pdf` once per document: 8.4 GiB to 337 MiB of copies on 500 pages
  (M29.18).
- **`jwt`**: `PS256` / `PS384` / `PS512` (M29.51).
- **`html`**: `parse` slices the source instead of a character list: about a
  third faster, 40% less memory (M29.15).
- **`flatdb`**, **`oauth.save`**: fsync the directory after the rename
  (M29.57).
- **`screen`**: C1 control characters are drawn as spaces too.
- **`mqtt`**: messages queued during an ack wait are capped at 64 MiB.
  **`amqp`**: a frame over the negotiated frame-max is `"limit"`.

### Tooling

- **`profile`**: `--out FILE`; SIGINT / SIGTERM write the report, SIGUSR1 a
  snapshot (M29.30). Rows name the function and source line, `--by=function`,
  `--top N`, `--focus PATTERN` (M29.31). A wait column for blocking builtins;
  the header gives wall time and task count (M29.32). `--allocs` counts copies
  into literals (M29.53).
- **`lint`**: `L117` checks literal `printf` formats (M29.10); `L302` reports
  removed API (M29.35), an import of a removed bundled module with the deck
  that replaced it (M29.33), and a removed module function (M29.17, M29.34); `L301` reads `@deprecated` from module docblocks
  (M29.43). One module imported under two prefixes is one type.
- **Releases** add `linux/arm` (armv7, armv6), `linux/386`, `linux/riscv64`
  and an unsupported `android/arm64` build (M29.6).
- **Editors**: a Pygments lexer (`editors/pygments/`).

### Docs

- The API pages show signatures as code with a contents table, and each module
  guide links to its API page.
- `docs/trademarks.md`.

### Fixed

- 32-bit targets: `profile --trace` panicked on unaligned atomics; counts were
  narrowed to `int` before their range checks (M29.6).
- The borrow analysis missed that `testing.runArgs` / `assertThrowsArgs` run
  `.j` code (M29.35).
- `kv`: a writer whose lock was broken as stale could still write; it now
  throws `"kv"`.
- `crypto.jwkToPem` checks that an EC point is on the curve.
- `yaml`: a timestamp written in flow style round-trips as a `datetime`.
