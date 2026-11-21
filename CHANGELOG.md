# Changelog

User-visible changes to the Jennifer interpreter, its built-in libraries and
its bundled modules in the current release. Earlier releases are described in
the release notes of each tag at
<https://github.com/jennifer-language/jennifer/releases>. Versions are the git
tags (bare semver, no `v`). Each `(Mxx.n)` reference points at the entry in
[docs/roadmap.md](docs/roadmap.md) (or [docs/milestones.md](docs/milestones.md)
once the release is tagged) that explains the change in depth.

## 0.27.0

A release without a milestone: it ships what was done since 0.26.0. A second
audit of the bundled modules makes parsers and protocol clients stricter
(malformed input throws, the module's error struct or a generic kind),
refuses credentials over plain `http://` and plaintext logins unless opted in,
and fixes lost messages, races and quadratic paths. New are
`fs.readSecretBytes` (M29.22), UTF-16 and UTF-32 in `encoding`,
`intl.formatNumber` and `i2c.writeRead`; the first round of the conventions
survey renames thirteen names and five fields (M29.23). From the
next milestone come bounded request parsing in Go (M29.2), the remaining caps
as kind `"limit"` (M29.1), new `strings`, `lists`, `json` and `decimal`
functions, printf and `time.format` width modifiers, and `match` on an indexed
subject (M29.7, .8, .19, .24-28).

### Breaking changes

- **Stance 8 renames, old name kept.** The old names still work, marked
  `@deprecated`, and are removed in 0.28.0 (M29.23, M29.34):
  - `orm.hasOne` / `hasMany` -> `oneToOne` / `oneToMany`;
  - `validate.isInt` / `isFloat` / `isBool` -> `integer` / `number` /
    `boolean`. **The rule kinds change with them**, also through the old
    names: `Failure.rule`, `localize` keys and `webapi`'s 422 failures read
    `"integer"` / `"number"` / `"boolean"`;
  - `rest.insecure`, `graphql.insecure`, `mikrotik.insecure`,
    `soap.withInsecure`, `webdav.withInsecure` -> `withSkipVerify`;
  - `soap.withCaCert`, `webdav.withCaCert` -> `withCA`.
- **Stance 8 renames, no old name.** Code that names these fields must change
  (M29.23): `memcache.Session.timeout`, `redis.Session.timeout` and
  `s3.Client.timeout` -> `timeoutMs`; `totp.Options.period` -> `periodSec`;
  `oauth.DeviceAuth.interval` -> `intervalSec`. Renamed parameters
  (positional, so calls are unaffected): `ttl` -> `ttlSec` in `kvstore` and
  `session`, `timeout` -> `timeoutSec` in `telegram.getUpdates`.
- **Credentials need TLS unless opted in.** Refused before anything is sent:
  - an `Authorization` / `Proxy-Authorization` header (a `Cookie` too in
    `webdav`) over `http://` to a host other than loopback in `graphql`,
    `jsonrpc`, `mcp`, `soap` and `webdav` (reason / status `"insecure"` or 0),
    every signed `s3` request, every `gotify` push, and every `acme` request
    (RFC 8555); opt in with `allowInsecureHttp` on the client, `Config` or
    `acme.connectWith(url, key, acme.Options{...})`;
  - `imap` `LOGIN` / XOAUTH2, `pop` `USER` / `PASS` / XOAUTH2, an `ldap`
    simple bind or `passwordModify` with a password, and a `mikrotik` login
    over plain TCP; opt in with `allowInsecureAuth` (on `ldap.Conn`, on the
    others' `Options`);
  - `mactelnet` speaks EC-SRP only; MD5 (RouterOS < 6.43) needs
    `connectWith` with `allowLegacyMd5`;
  - `rest.paginate` throws on a `rel="next"` link to another origin
    (`allowCrossOriginRedirect` follows it).
- **Strict parsing.** Malformed input throws instead of being skipped,
  defaulted or repaired:
  - `ical.parse` and `vcard.parse` throw the new `ical.IcalError` /
    `vcard.VcardError` (`kind`, `message`, `line`) for broken structure,
    escapes, dates, durations, `RRULE`s, a missing `FN`, an unknown `VERSION`;
  - `mime.parse` (header lines, boundaries, truncated multiparts, bad base64 /
    QP, bytes invalid in the declared charset; nesting past 100 is `"limit"`);
    `multipart.parse` (a part without `form-data` and `name`, a line starting
    with the boundary, no close delimiter);
  - `feed.parse` throws kind `"value"` for an unparseable date on any item, a
    bad enclosure `length`, an `<rss>` without exactly one `<channel>` or a
    `<feed>` outside the Atom namespace;
  - `dotenv` (a line without `=`, an empty key, unknown escapes, text after a
    closing quote); `jsonl` (Unicode whitespace that is not JSON's);
    `semver.satisfies` / `maxSatisfying` / `minSatisfying` / `simplifyRange`
    throw `"value"` for an invalid range; `cron` refuses a value past the
    field's range; `dot.render` refuses control characters;
  - `uri.parse` / `resolve` refuse a malformed port and text after an IP
    literal's `]`; `validate.url` checks `%` escapes and IP literals; `idna`
    refuses non-canonical `xn--` labels and invisible / control characters;
  - replies: `http` (status line, header syntax, obs-fold, control characters,
    `Transfer-Encoding` other than a lone `chunked`, conflicting
    `Content-Length`), `jsonrpc` and `mcp` (not a JSON-RPC 2.0 response),
    `graphql` (not a GraphQL response, also from `tryQuery`), `acme`, `imap`,
    `pop` (kind `"pop3"`), `webdav.parseMultistatus`, `snmp` unsigned values
    out of range, `s3` `<Error>` bodies under a 200;
  - `kv.openFile` refuses a file that is not a kv store or is damaged.
- **Options and arguments are checked, never clamped:** a negative TTL in `kv`
  and `kvstore`; `totp` negative `periodSec` or counter, `digits` outside
  6..8, and a secret `totp.uri` could not use; `password` `minLength` below 1 or a negative
  minimum; `orm.limit` / `offset` below 0 and WHERE / HAVING connectors other
  than `AND` / `OR`; `log.at` with an unknown level (`"log"`); `args`
  duplicate or shadowing definitions and an option value that looks like an
  option (`--name --quiet`; write `--name=--quiet`); `web` `sameSite` other
  than `Lax` / `Strict` / `None` (`None` needs `secure`); `ansi` colour indexes
  and channels outside 0..255 (`"value"`, were clamped); `barcode` sizes, QR
  EC level, empty 1D data and GS1-128 element strings; `label` sizes,
  rotations, quantities and barcode data; `plot` marks, shapes, `legendPos`,
  `barMode` and geometry; `pdf` page sizes, colours, an empty document and a
  bookmark to a missing page; `zmq.poll` with a negative timeout;
  `doctree` selectors with other brackets.
- **Error kinds.** `"limit"` for a reached cap in `flatdb.openWith` (was
  `"flatdb"`), `ipnet` `hosts` / `hostCount` / `split` (was `"ipnet"`),
  the `doctree` readers' caps (was `"markdown"` / `"rst"`), `pdf` image
  pixel caps; `"closed"` for an `smtp` reply cut off or a `sendOn` on a closed
  session (was `"smtp"` / code -1); `"pdf"` for text outside WinAnsi (was
  `"runtime"`). `oauth.load`, `uri`, `font`, `pdf`, `rst` and `myst` pass the
  generic kinds on. `validate.custom` passes a predicate's `"runtime"` and
  generic kinds on instead of failing the field; a non-`bool` result is
  `"value"`.
- **Behaviour:**
  - `gpio.j` maps a pin through the Raspberry Pi header chip's sysfs base
    (512+ since Linux 6.2), found by label; another board throws.
    `gpio.useChip(gpio.GLOBAL)` keeps the old global numbering;
  - `serial.open` / `openWith` claim the port exclusively (`shared: true`
    opts out) and reset every line setting a previous program left;
  - `cron`: a `*/n` day field restricts, and with one day field starting `*`
    a day must match both (Vixie cron);
  - `session.save` returns `bool` and writes only a live session (never one
    destroyed, rotated, expired or never created);
  - `web` names the CSRF cookie `__Host-csrf` with Secure cookies;
  - `acme.pollAuthorization` throws an `AcmeError` with the challenge's
    problem unless the authorization is `valid`;
  - `webapi`: `install` makes the guard every route's handler; rate limiting
    runs before the 401 / 403 / 422 checks; bad `limit` / `offset` is a 400;
    a handler throw is a 500 envelope, then rethrown to `web.onError`;
  - `jsonrpc.notify` throws on a non-2xx answer;
  - `barcode` EAN / UPC `text` carries the check digit; code39 / code93
    encode lowercase as uppercase;
  - `spi.transfer` over spidev's `bufsiz` throws `"limit"` instead of failing
    in the kernel; it is never split.
- **Caps throw `"limit"`, not `"runtime"`** (M29.1), messages unchanged: a
  range materialised into a list past the element cap (`0..n`),
  `channel.make` past its capacity cap, a `crypto.pbkdf2` / `argon2id` /
  `scrypt` `keyLen` over 1 MiB, `pbkdf2` work over 1e8, the `argon2id` time,
  threads and memory caps, the `scrypt` memory and `p` caps, and the same
  caps and the bcrypt cost cap in `crypto.verifyPassword`, `serial.read` / `i2c.read` / `readReg`
  over 256 MiB, `intl.tr` output over 1 MiB, an `io.printf` field value over
  4096 (`pad`, `max`, `prec`, `group`, `depth`), and the handle caps of `net`
  (connections, listeners, UDP sockets), `httpd` servers, `sql` connections,
  rows, transactions and statements, and `term.makeRaw`.
- **`time.format` / `time.parse` layouts:** a `|` right after a code starts a
  modifier list; write `||` for a literal `|` there (M29.24).
- **Request parsing caps** (M29.2), kind `"limit"`: more than 10,000 form or
  query pairs (`web.form`, `uri.parseQuery`), a multipart part header over
  16 KiB or a part name / filename over 1 KiB. A multipart part header that
  is not UTF-8 throws `"multipart"` (was `"runtime"`).
- **`web.form` / `formValue` are strict** (M29.2): a `%` without two hex
  digits or a decoded value that is not UTF-8 throws kind `"encoding"`, and a
  body that is not UTF-8 `"runtime"`; before, such bytes stayed literal or
  were read as ISO-8859-1. Unhandled, the request is answered 500 until M29.29
  answers it 400. Browsers send neither from a page served as UTF-8.
- **One cap for every result a call builds:** a string built by
  `strings.replicate` / `replace` / `join` or `regex.replace` now stops at
  256 MiB (32 MiB on `jennifer-tiny`), as bytes did; it was 1 GiB (128 MiB).
- **`web` routing:** runs of `/` in a request path count as one, and a path
  of more than 128 segments matches no route (a 404), wildcards included.
  `webapi` reads the first 64 ranges of an `Accept` header and the first 16
  `;` pieces of each.

### Deprecated

Removed in 0.28.0 (M29.35); `jennifer lint` reports each use as `L301`
(info, so a gate keeps passing while code migrates):

- `strings.substring`, `lists.slice` / `head` / `tail` / `pop` and
  `binary.slice`: the language's slices do their job (`$s[a..b]`, `$xs[..n]`,
  `$xs[len($xs) - n..]`, `$xs[..len($xs) - 1]`, `$b[a..b]`).
- `io.printf` / `sprintf` / `eprintf` with one non-string value: write
  `io.printf("%v", $v)`.
- `strings.repeat`: renamed `strings.replicate`, because `repeat` is a
  keyword.
- `testing.runWith` / `assertThrowsWith`: renamed `runArgs` /
  `assertThrowsArgs`, because `With` means an options struct everywhere
  else.
- `crypto.hkdf` / `pbkdf2` / `argon2id` / `scrypt`: use
  `crypto.deriveKey(algo, secret, salt, length, crypto.KdfParams{...})`, one
  function for every key derivation with the algorithm first, like the rest
  of `crypto` (`"hkdf-sha256"`, `"pbkdf2-sha256"`, `"argon2id"`, `"scrypt"`).
  It has no default costs: a parameter the algorithm needs must be set.
- `lists.range(a, b)`: use `a..b`; one that counted down (`range(3, 0)`) is
  `lists.range(3, 0, -1)`. The stepped `range(a, b, step)` stays.
- `convert.toString(n, radix)`: use `io.sprintf("%d|base=16", n)`, the same
  digits; rendering a number is presentation, which printf owns.
- `convert.bytesFromString` / `stringFromBytes`: use `encoding.encode` /
  `decode`, same arguments (`encoding.encode("utf-8", s)`), which now take
  UTF-8, UTF-16 and UTF-32 as well as the single-byte codecs.

The bundled modules, the examples and the docs already use slices,
`replicate`, `"%v"` and `encoding.encode` / `decode`; the bundled modules
that do now require `>=0.27.0`.

### Language and interpreter

- `match` destructures an indexed subject: `match ($queue[$head])` over a
  `list of Tree`, a `map of K to Tree` value, and chains such as
  `$grid[$i][$j]` or `$byName["x"].tree`, with exhaustiveness checked when the
  file loads, inside `spawn` too. A call result is still refused (M29.28).

### Standard library

Libraries (`use NAME;`):

- **`strings`**: `replicate(s, n)`, the new name of `repeat`, which is spelled
  like a keyword (the pair matches `lists.replicate`). `join(parts)` without a
  separator joins with `""`, as `binary.join` does. `trimPrefix` /
  `trimSuffix` remove one exact affix;
  `trimSet` / `trimLeftSet` / `trimRightSet(s, cutset)` strip any rune of a
  cutset. The whitespace `trim*` are unchanged (M29.7).
- **`lists`**: `replicate(xs, n)`, `n` deep copies of `xs` concatenated (over the
  `concat` cap is `"limit"`, checked before anything is built; M29.25);
  `sortWith(xs, cmp)`, a stable sort by an `int` comparator for orders on
  several keys (M29.27).
- **`json`, `toml`, `yaml`**: `asFloats(v[, ptr])` / `asInts` / `asStrings`
  read a whole list in one call, each element as `asFloat` / `asInt` /
  `asString` reads it; about 10x faster than a per-element loop on a
  3,072-number embedding (M29.8).
- **`decimal`**: the digit cap rises from 100,000 to 1,000,000 on the default
  build (100,000 on `jennifer-tiny`), so 5^4^3^2 fits; parsing a long decimal
  string is no longer quadratic (1,000,000 digits: about 2 s to 0.2 s)
  (M29.26).
- **`crypto`**: `deriveKey(algo, secret, salt, length, KdfParams)` with
  `"hkdf-*"`, `"pbkdf2-*"` (`sha1` / `sha256` / `sha512`), `"argon2id"` and
  `"scrypt"`, the checks and caps of the functions it replaces.
- **`encoding`**: `encode` / `decode` take `"utf-8"`, `"utf-16"` / `"utf-32"`
  (a byte-order mark; big-endian without one) and their `le` / `be` forms
  (fixed order, no mark). Decoding is strict: invalid UTF-8, a truncated
  unit, an unpaired surrogate or a value outside Unicode is an error naming
  the byte offset. `codecs()` lists them first.
- **`encoding`**: `parseForm(text)`, `parseCookies(header)` and
  `parseMultipart(contentType, body) -> list of FormPart` parse request text
  in Go with bounded memory, on both binaries (M29.2). They are strict:
  malformed input is kind `"encoding"`, a reached cap `"limit"`.
- **`io`**: `%f` takes `fill=0` and `group` / `sep` on the integer part, as
  `%d` does: `%f|prec=3|pad=10|fill=0|sign=always` prints `+00003.142`
  (M29.19).
- **`time`**: the numeric codes take printf's `pad` modifier: `%d|pad=0`
  prints the day unpadded, `time.parse` reads the same layouts (M29.24).

- **`fs`**: `readSecretBytes(path) -> bytes` / `readSecretString` read a key or
  token file only when no group or other permission bit is set and the owner
  is the effective user or root (on a read-only mount, such as a container
  secret, only group / other write is refused); over 1 MiB is `"limit"`
  (M29.22). OS errors name each path once (`fs.FUNC: PATH: CAUSE`).
- **`intl`**: `formatNumber(locale, value, digits)` for `int`, `float` and
  `decimal.Decimal` with CLDR separators and grouping, half-even rounding.
- **`i2c`**: `writeRead(bus, data, n)`, a combined transfer with a repeated
  start (`I2C_RDWR`); errors carry hints for the usual errno values, an
  adapter timeout is `"timeout"`.
- **`serial`**: `Options.flowControl` (`"none"`, `"rtscts"`, `"xonxoff"`) and
  `shared`; full raw mode, rates up to 4,000,000, input parity checking with
  parity on; a hangup is `"closed"`; errors name the path with a hint.
- **`spi`**: calls on one device are serialised, `close` waits for a transfer
  in flight. **`gpio`**: errno hints.
- **`kv`**: errors documented as kind `"runtime"`. **`math`** / **`stats`** /
  **`decimal`**: messages name the function (`math.sqrt: ...`).

Modules (`import "NAME.j";`):

- **`amqp`**: `qos(c, prefetchCount)`, `publishMandatory` (a returned message
  makes `waitConfirm` throw scope `"return"`, code 312), bad credentials are
  an `AmqpError` with `replyCode` 403; `close` discards unread confirms.
- **`http`**: `isCleartextRemote(url)`; a reused keep-alive socket that the
  server closed resends an idempotent request (or one with `Idempotency-Key`)
  once on a new connection.
- **`imap`**: `fetchBytes`, `fetchPartialBytes`; `fetch*` and `flags` read the
  FETCH carrying the requested UID and throw when there is none. **`pop`**:
  `retrieveBytes`, `topBytes`, `listSizes -> list of MessageSize` (a
  `sizes` position is not the message number once one is deleted).
- **`jsonrpc`**: `parseReply(body, status, id)`; the server answers a
  malformed request `-32600`.
- **`ldap`**: `connectWith(address, security, net.TLSOptions)` /
  `startTlsWith` for a private CA; the server never returns or matches
  `userPassword`, answers writes with code 53, serialises edits; search
  replies are capped (16 MiB a message, 64 MiB a search, `"limit"`).
- **`mcp`**: the HTTP client sends `MCP-Protocol-Version` and reads event
  stream replies; the stdio client stops a server after `stdioTimeoutMs`
  (default 60 s, `"timeout"`) and shuts it down with SIGTERM / SIGKILL; stdout
  past 16 MiB is `"limit"`.
- **`mqtt`**: a message arriving while `publishQos1` / `subscribe` wait for an
  ack is queued on the `Client` and returned first by `receive` / `poll` (over
  1000: `"limit"`).
- **`redis`**: `subscribe` / `psubscribe` / `unsubscribe` / `punsubscribe`
  return once the server confirmed; messages read meanwhile stay queued.
- **`s3`**: replies other than object downloads capped at 16 MiB; the
  endpoint must be a bare `scheme://host[:port]`.
- **`sqlmigrate`**: `migrate` / `rollbackMigrations` hold a per-database
  advisory lock (60 s wait, then `"timeout"`).
- **`vcard`**: reads vCard 2.1 quoted-printable values in their `CHARSET` and
  group prefixes (`item1.EMAIL`). **`ical`**: `DURATION`, several
  `VCALENDAR`s, RFC 5545 recurrence; `occurrences` past 100,000 steps is
  `"limit"`.
- **`webapi`**: `deprecatedSince` (`Deprecation` header) beside `deprecate`
  (`Sunset`), `identity`, `onError`.
- **`label`**: ZPL writes `^CI28` (UTF-8) and `^PW` / `^LL` from the label size.
- **`log`**: control characters in values and text-form messages are escaped;
  a field named `time` / `level` / `msg` is renamed in every format.
- **`screen`**: `waitKey` returns `"eof"` on every call after end of input.
- **`tengine`**: `add` replaces a name already in the set.
- **`web`, `uri`, `multipart`**: form bodies, query strings, Cookie headers
  and multipart bodies are parsed in Go (M29.2). A 3 MB `&&&...` form body
  peaks at 26 MiB instead of 3.2 GB, a 1 MiB part header at 27 MiB instead of
  1.6 GB; results for well-formed input are unchanged.
- **`web`, `webapi`**: route patterns are split once when registered, and the
  guard matches against a precompiled table, so routing no longer re-splits
  every pattern per request (the registry's `/` served about 40% more
  requests a second).

### Tooling

- **`lint`**: `L204` no longer flags `len(...)` in a string slot; `L205` no
  longer flags an accumulator whose every call, recursive ones included,
  updates in place (`$v = f(..., $v, ...)` on a function local outside a
  `try`) or passes a constant literal such as `[]`.
- **`args`**: help text in one aligned column across positionals, options
  and subcommands.
- **`scripts/conventions-survey.j`** lists every deviation from the
  conventions catalogue; its findings are kept in
  `docs/technical/conventions-findings.md` (M29.23).

### Docs

- `docs/technical/conventions.md`, the stance 8 catalogue, and the recorded
  exceptions in `design-decisions.md` (M29.23).
- Module doc examples run as written: those of the audited modules, `html`,
  `sasl`, and the mail examples (now with `transport.Security`).
- Editors: Zed and Helix need a tree-sitter grammar; JetBrains IDEs take the
  TextMate bundle. The interpreter notes say a `try` body has its own frame;
  the profiler docs say rows sort by `CUM`, then `SELF`.

### Fixed

- The zero `func` value (`def f as func;`) is null: `$f == null` was false,
  so a program could not test whether a callback was set.
- `io.printf` `%f` with `sign=always` printed `+-0.0` for `-0.0`.
- `images.decode` / `info` on data that is no image say "not a PNG, JPEG or
  GIF image" instead of `image: unknown format`; `compress.unpack` on a
  truncated stream, or one followed by bytes that are no stream, says so
  instead of `unexpected EOF`.
- `crypto.hkdf` past RFC 5869's maximum (255 x the hash size) names the
  maximum in its message instead of `hkdf: hkdf: requested key length too
  large`.

- `mqtt` and `redis` dropped messages that arrived during an ack or
  subscribe wait; `redis` reads were quadratic in the reply size, and a
  `timeoutMs` of 0 left an earlier read deadline armed.
- `amqp.waitConfirm` counted a `multiple` ack as one confirm.
- `flatdb.save` could lose an update between its check and the rename; it now
  rechecks under a short save lock (`<path>.savelock`).
- `ntp` reported a Kiss-o'-Death sent with leap indicator 3 as unsynchronised,
  and failed on a stray datagram instead of waiting for the reply.
- `snmp.walk` on v1 did not stop at `noSuchName`.
- `zmq.poll(sock, 0)` blocked.
- `webhook`: with an empty secret a verify call could return `false` before
  throwing; it now throws first.
- `ldap`: a filter with trailing text (`(uid=a)x`) or an unescaped `(` was
  accepted.
- `smtp`: an address could close its angle brackets and add ESMTP parameters
  (`a@example.com> NOTIFY=NEVER`).
- `imap`: bytes a server sent behind its STARTTLS `OK` were read after the
  handshake; untagged lines during `AUTHENTICATE` broke the exchange.
- `rest`: `Link` headers are parsed per RFC 8288 (commas inside `<...>`,
  several `rel` types, relative targets).
- `font`: a font with an out-of-range `unitsPerEm` or `numberOfHMetrics`, or
  a cmap glyph id past `numGlyphs`, is refused as malformed.
- `bloom.optimal` caps `k` at 255 (a tiny `fpr` gave a filter `new` refused);
  a deserialized filter's size is checked.
- `tengine` closes its render budget store.
- Linear instead of quadratic: the `csv` streaming reader's quote check,
  `dotenv` multi-line values, `mime` RFC 2231 parameters, `rst` unmatched
  inline start-strings, `doctree` PDF quote fills, `plot` bars, `pdf`
  outlines.
- `doctree` PDF: deep quote / list nesting drew text past the page edge, and
  wide tables overflowed; both are now `"limit"` or sized columns.
- `markdown`: inline nesting past 32 levels dropped formatting silently; it is
  now `"limit"`.
- `rst`: a malformed `csv-table` header is reported, a limit message is no
  longer doubled, an escape after a start-string is content. `myst`: a
  backtick in an info string, an indented fence line, fences in nested list
  items and fences over 1000 characters.
- `mactelnet`: `send` and `recv` keep their deadlines while other hosts send
  datagrams; console output is capped at 1 Mi characters (`"limit"`).
  `mikrotik`: replies are capped (`"limit"`).
- `oauth.load` turned a generic kind from reading the file into `"oauth"`.
