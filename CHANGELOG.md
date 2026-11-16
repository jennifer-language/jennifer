# Changelog

User-visible changes to the Jennifer interpreter, its built-in libraries and
its bundled modules, one section per release. Versions are the git tags (bare
semver, no `v`). Each `(Mxx.n)` reference points at the entry in
[docs/milestones.md](docs/milestones.md) that explains the change in depth.

## 0.26.0 (unreleased)

A breaking release that tidies the standard library before 1.0: functions take
an algorithm, format or codec name first, `crypto` has one function per
operation, and errors carry kinds and structs (`"limit"`, `"net"`, `"closed"`,
`net.NetError`, `rest.HttpError`, ...) instead of message text. Parsers and
network clients are stricter throughout. New are the `decimal` and `images`
libraries, the `doctree`, `rst`, `myst`, `soap`, `webdav` and `zmq` modules, and
nine bundled vendor clients are deprecated in favour of `@jennifer/*` decks.

**Upgrading jvc.** jvc 0.1.0 does not run on 0.26.0, so it cannot update
itself afterwards (`jvc app update` fails with `bytesFromString: codec ... is
not supported`). Update jvc to 0.2.0 before installing 0.26.0, or afterwards run
0.2.0 once from its tag to replace the installed copy:

```sh
git clone --depth 1 --branch 0.2.0 https://github.com/jennifer-language/jvc /tmp/jvc
jennifer run /tmp/jvc/bin/jvc app update jvc
```

### Breaking changes

- **Name first.** `hash.compute(algo, b)`, `hash.hmac(algo, message, key)`,
  `crc.compute`, `compress.pack` / `unpack`, `encoding.toText` / `fromText` /
  `encode` / `decode`, `convert.bytesFromString` / `stringFromBytes`,
  `archive.pack` / `unpack` / `unpackWith`, `crypto.hkdf` / `pbkdf2`,
  `stats.proportionCi`, `testing.report`; in modules `jwt.sign(alg, claims,
  key)` and every `jwt.verify*`, `barcode.encode`, `feed.build`, `log.new` /
  `toStderr` / `toFile`, `ldap.password`, `ldap.bindSasl(conn, algo, ...)`,
  `sasl.scramStart`, `acme.challenge`, `webhook.timestamped*`. Move the name
  argument to the front (after a handle). (M28.41)
- **`crypto`: one function per operation**, the algorithm first: `encrypt` /
  `decrypt(algo, ..., key, nonce, aad)`, `sign` / `verify`, `keypair` /
  `publicKey`, `encapsulate` / `decapsulate`, `wrap` / `unwrap`. Use
  `encrypt("aes-gcm", ...)` with your own 12-byte nonce instead of the
  two-argument form, `keypair("ed25519")` instead of `signKeypair`,
  `sign("rsa-sha256", ...)` instead of `rsaSign` / `ecdsaSign` (and `verify` for
  their `*Verify`), `keypair("rsa2048")` / `keypair("p256")` instead of
  `rsaGenerateKey` / `ecGenerateKey`, `"mlkem768"` instead of `mlkem*`, and
  `hash.equal` instead of `hmacEqual`. Old `encrypt` boxes still open. (M28.40)
- **`markdown` renders through `doctree`.** `markdown.parse` returns a
  `doctree.Node`; use `doctree.render("html", doc)` instead of
  `markdown.render(doc, "html")`, and `doctree.renderPdf` / `renderPdfDoc` /
  `pageBreak` / `pdfDefaults` and the node accessors likewise. The `to*`
  shortcuts stay. The reader is now CommonMark 0.31.2, so some output differs.
- **Removed:** `oauth.google` / `oauth.microsoft` (use `@jennifer/oauthext` or
  `oauth.discover`); `feed.fetch` (use `http.get`, then `feed.parse`).
- **`webapi.limiter`** takes `func(key, limit) -> ratelimit.Result`, not `bool`.
- **Error kinds.** Match the kind, never the message:
  - a reached cap is `"limit"`: `http` body / header / chunk caps,
    `net.readAll`, the `json` / `toml` / `yaml` / `xml` / `asn1` nesting and node
    caps, and the receive caps and budgets of the bundled modules (M28.54,
    M28.55, M28.68); also the size and count caps of `binary` (`make`, `join`,
    `split`, `readInts` / `readFloats`), `asn1.encode`, `crypto.randBytes`,
    `regex` patterns and `strings.repeat`, the open-stream caps of `hash`,
    `crc` and `compress`, the per-call caps of `net.readN` / `readBytes` /
    `recvFrom` (256 MiB), `io.readBytes` / `readChars` / `readLine`, `fs`,
    `lists`, `linalg`, `ml` and `os.spawn`, `multipart.parse` (1000 parts),
    `memcache` (64 KiB line, 64 MiB value), `amqp` frames (64 MiB), `myst`
    csv-tables (20,000 cells) and `rst` substitutions (all were `"runtime"` or
    the module's kind);
  - a closed channel (`send`, `recv`, `recvTimeout`, `select`), a late
    `httpd` reply and a `memcache` close mid-reply are `"closed"`; a malformed
    `mqtt` packet is an `MqttError`; `snmp` and `webdav` pass the generic
    kinds on; no reply from an `ntp` server or an `snmp` agent is `"timeout"`
    (was `"ntp"` / `"snmp"`);
  - a socket failure (refused, reset, DNS, TLS) is `"net"`, always a
    `net.NetError` (was `"runtime"`); an `http` server closing before any reply
    is a `NetError` (was `"http"`), a reply cut short is `"closed"` (M28.65);
  - a cancelled task is `"cancelled"`; `net.readN` short of `n` bytes, and a
    peer closing mid-read in `amqp`, `mqtt`, `imap`, `pop`, `redis`,
    `websocket` and `mikrotik`, is `"closed"` (M28.73, M28.74);
  - `jsonrpc`, `oauth`, `soap` and `telegram` pass the generic kinds on instead
    of rewrapping them; `amqp`, `snmp`, `ntp` and `webdav` pass `"net"` on.
- **Module error structs.** The protocol clients (`acme amqp graphql imap
  jsonrpc ldap mcp memcache mqtt ntp oauth pop redis rest s3 smtp snmp soap
  webdav zmq` and the deprecated ones) throw one struct each (`rest.HttpError`,
  `smtp.SmtpError`, ...), kind and message unchanged. A parameter typed `Error`
  no longer accepts them; type it with the module's struct. (M28.51 to M28.53)
- **Value nesting cap.** A literal, index, field or append write nesting a value
  past 1000 container levels (64 on `jennifer-tiny`) throws `"limit"`. (M28.37)
- **Invalid UTF-8 is an error** where outside data becomes a `string`: `io` and
  `fs` text reads, `os.run` / `os.wait` output (use the new `os.runBytes` /
  `waitBytes`), `xml` and `yaml` input; `httpd` answers `400`.
- **Stricter parsing:** base32 / base64 `encoding.fromText` takes only
  canonical text (no line breaks, exact padding) (M28.43); `csv` refuses
  malformed quoting and ragged records, and writes CRLF; `yaml` refuses YAML
  1.1 numbers (`012`, `1_000`: quote them) and mismatched core tags; `xml`
  enforces well-formedness and namespace well-formedness (M28.34);
  `convert.toFloat` takes plain decimal only; `json.asInt` refuses `1.0`;
  `asn1.printableString` / `ia5String`, `uri.parse`, `validate.url` / `email`
  and `os.spawn` options are checked; `log` rejects an unknown format or level
  instead of falling back to text / info.
- **New caps:** `compress.unpack` output (256 MiB default), `fs.readString` /
  `readBytes` (256 MiB), `lists.push` / `concat` (4,194,304 elements), `regex`
  patterns (32 KiB, 50,000 instructions), `jsonrpc` batches (1000),
  `password.generate` (4096); 32 MiB for `binary`, `fs`, `archive` and
  `compress` on `jennifer-tiny`.
- **Refused for safety:** non-`http(s)` URLs and `https` to `http` redirects in
  `http` (opt in with `Options.allowInsecureRedirect`); headers outside a safe
  allowlist on a cross-origin redirect (M28.59); `jwt` HMAC keys under 32 / 48 /
  64 bytes; `totp` secrets under 80 bits; empty `web` CSRF and `webhook`
  secrets; `session` TTLs below 1; unsafe names in `html.render`; invalid `s3`
  bucket names and empty keys, and `presign` expiries outside 1..604800 s;
  negative `gpio` pins; `acme` challenge tokens that are not base64url; a
  `soap` DOCTYPE behind a leading comment; `.` / `..` members in `webdav`
  listings; an `mqtt` remaining length over 4 bytes; an `ES*` JWK on another
  curve in `jwt`; Unicode whitespace in `validate.email` / `url`; `xml`
  attributes without whitespace between them; `totp.generateSecretN` under 10
  bytes; a SCRAM success in `pop`, `smtp` and `imap` without a verified
  server-final (the server never proved it knows the password); a SCRAM
  iteration count below 4096 or an `m=` extension in `sasl` (SASLprep is not
  applied); an `ldap.bind` with a DN and an empty password (an unauthenticated
  bind; anonymous bind is unchanged); Unicode format characters (`\p{Cf}`:
  bidi controls, zero-width, BOM) in `validate.email` / `url`; an `ES*` PEM key
  on another curve and a `leeway` outside 0..86400 in `jwt`; a
  `# pragma-jennifer-version` bound with a pre-release or build suffix
  (`>=0.25.0+junk`, also lint L303).
- **Behaviour:** `asn1.encode` sorts `SET` children (DER); `ansi` wrappers
  reset only their own attribute; `jennifer test --filter` only narrows the
  `test*` functions; `webapi` ignores an `Accept` range whose `q` is not a
  qvalue in 0..1 (it counted as 1); the bundled modules that use a 0.26.0 feature now require
  `>=0.26.0`, so a copied one does not load on 0.25.x.

### Deprecated

- The bundled `discord`, `gotify`, `influxdb`, `mactelnet`, `mikrotik`,
  `prometheus`, `slack`, `statsd` and `telegram` modules, removed in 0.28.0.
  Each continues as a deck with the same surface: `jvc add "@jennifer/NAME"`,
  then `import "@jennifer/NAME/" as NAME;`. (M30.1)

### Language and interpreter

- `for (def b in $data)` iterates `bytes`, binding each byte as an `int`. (M28.49)
- A literal of a bundled module's struct may omit fields (`jwt.Policy{iss: $i}`).
- `# pragma-jennifer-version: >=0.25.0 <0.26.0` takes an exclusive upper
  bound; a `-dev` build passes every floor and fails every bound. (M28.42)
- Generic error kinds `"limit"`, `"timeout"`, `"closed"`, `"net"`,
  `"cancelled"`, `"value"`; `testing.assertThrows` sees a thrown struct's kind.
- `task.cancel` wakes `time.sleep`, channel operations, socket reads (TCP and UDP, and so
  `http` requests), `serial.read` and module code.
- Faster rune positions on ASCII strings; borrowing survives calls into module
  functions that cannot re-enter the host.

### Standard library

Libraries (`use NAME;`):

- **`binary`**: `bitXor` / `bitAnd` / `bitOr` / `bitNot`; `readInt` /
  `readUint` / `readFloat` / `readInts` / `readFloats(order, b, ...)`.
- **`channel`**: `recvTimeout(ch, ms)`. (M28.48)
- **`crypto`**: AES block modes (`blockEncrypt`), RSA-OAEP, X25519 `agree`,
  ChaCha20-Poly1305 / XChaCha20-Poly1305, `"nacl-sealedbox"`, `hashPassword` /
  `verifyPassword` (argon2id, bcrypt, scrypt), `argon2id` / `scrypt`. (M28.58)
- **`decimal`** (new): exact base-10 arithmetic with explicit rounding modes.
- **`fs`**: `mkfifo`; `writeNew` takes bytes, refuses a symlink, leaves no
  partial file. **`os`**: `unsetEnv`, `hasEnv`, `runBytes` / `waitBytes`.
- **`hash`**: SHA-3 and BLAKE2 in `compute`, `hmac` and `stream`.
- **`httpd`**: `rawQuery`; `etag` answers `412` to a conditional write.
- **`images`** (new): PNG / JPEG / GIF decode and encode, an immutable RGBA
  `images.Image`, `crop`, `resize`, `rotate`, `dither`, `draw`, pixel access.
- **`json`**: `encodeWith(v, EncodeOptions{indent, sortKeys, rawHtml,
  canonical})` (RFC 8785), `mergePatch` (RFC 7386), `decodeWith(s,
  DecodeOptions{maxDepth, maxNodes})`, enums encode by variant; `typeOf(v,
  ptr)` returns `"missing"` (also `toml`, `yaml`). (M28.68)
- **`kv`**: `keys(store[, prefix])`; `openFile` creates `0600`, one store per path.
  **`maps`**: `without(m, keys)`. **`strings`**: `count(s, sub)`.
- **`math` / `stats` / `ml`**: accuracy at large parameters (Loader's form for
  `tPdf`, `binomialPmf`, `poissonPmf`), exact `math.log` at integer powers.
- **`net`**: `NetError{op, reason, sent, replied}`, `abort(conn)`. (M28.65, M28.71)
- **`serial`**: `setReadDeadline` / `setWriteDeadline`. (M28.46)
- **`xml`**: namespaces (`localName`, `namespace`, `lookupPrefix`, `attrNS`,
  `elementNS`, `setAttrNS`, prefix maps and `*:name` in paths). (M28.34)

Modules (`import "NAME.j";`):

- **`doctree`** (new): the document tree behind `markdown`, `rst` and `myst`,
  with HTML, XHTML, ANSI and PDF renderers.
- **`markdown`**: CommonMark 0.31.2 plus GFM tables, task lists, footnotes,
  definition lists, alerts and attributes.
- **`rst`**, **`myst`** (new): reStructuredText and MyST readers with
  directives, roles and `check(src)` diagnostics.
- **`soap`** (new) SOAP 1.1 / 1.2 client and server; **`webdav`** (new) client;
  **`zmq`** (new) ZeroMQ over ZMTP 3.1 without libzmq.
- **`flatdb`**: `openWith`, `getOr`, `saveWith` (`pretty`, `backup`),
  `updateWith`, `lockWith`.
- **`graphql`**: `withTimeout`, `withRetries` (queries only), `withBackoff`,
  `errorCodes`.
- **`http`**: Digest auth (`sendDigest`, `digestAuthorization`), HTTP-date
  `Retry-After` (M28.69), stricter request and response framing.
- **`jwt`**: `signWithKid`, `jwksKeys`, `Policy.leeway`. **`ldap`**:
  `escapeFilterValue`. **`s3`**: `errorOf(resp)`. **`session`**: `exists`,
  `rotate`. **`validate`**: `checkValue`, `ensure` / `ensureValue`.
- **`oauth`**: Authorization Code with PKCE, `devicePoll`, `clientAssertion`,
  `jwtBearer`, OIDC `discover*`, `Config.tls`. (M28.47)
- **`pdf`**: GIF and native PNG decoding, `imageFrom(name, images.Image)`.
- **`web`** / **`webapi`**: `web.remoteHost`, `web.rawQuery`;
  `webapi.limitKey`, `negotiate`, `Retry-After` on 429.
- **`websocket`**: `connectOptions` with headers, subprotocols and TLS trust.

### Tooling

- **`lint`**: `L116` flags a string slot holding only a literal (M28.76);
  `L104` accepts error-shaped structs (M28.50); `L205` also catches struct and
  `bytes` accumulators; `L303` checks pragma ranges; a `catch` binding is
  untyped, so passing it to a struct-typed parameter is no `L110`.
- **`profile --allocs`** ranks copies by bytes with the copied type and reports
  module-boundary rebuilds.
- **CI**: `scripts/*.j` are formatted and linted; a release whose changelog
  heading still says "unreleased", in any case, fails.
- **Editors**: a Chroma lexer draft (Forgejo, Gitea, Hugo); the other grammars
  handle dotted names, docblocks, nested slots and `use`.

### jennifer-tiny

- A stack guard turns deep recursion into the catchable `"limit"` "call stack
  too deep" instead of corrupting memory. (M28.72)
- `jennifer-tiny-dev` adds `fmt`, `lint`, `profile`, `test`, `tokens` and
  `ast` to the TinyGo build (`make build-tinygo-dev`). (M28.75)
- The `golang.org/x/crypto` algorithms run here too. CI uses TinyGo 0.44.0.

### Packaging and images

- New `alpine` image (`:alpine`, `:<version>-alpine`). The `slim` and `alpine`
  images bundle `jvc` (`ARG JVC_REF`, `0.2.0`); every image carries
  `/usr/bin/jennifer-tiny-dev`, which the release archives and packages do not.
- A release is gated on jvc's suite against the candidate and published from
  GitHub only.
- Release notes are the tag's section of this file; a tag without one, or still
  marked "(unreleased)", fails the release. (M28.24)
- New dependency `golang.org/x/crypto`: +216 KiB on `jennifer`, +188 KiB on
  `jennifer-tiny`.

### Fixed

- `images`: the pixel cap follows the allocation cap (64 Mpx, 8 Mpx on
  `jennifer-tiny`, which allowed 256 MiB canvases before), and resizing keeps
  its filter buffer within 256 MiB (32 MiB on tiny) by working in strips,
  instead of up to about 1 GiB.
- `decimal`: parsing and arithmetic share one exact 100,000-digit limit, so
  every value's `toString` parses back (`1e100000` did not); over it is
  `"limit"`.
- `http`: a `Retry-After` of delay-seconds with leading zeros (`0000001`) is
  read correctly instead of as the 30 s cap.
- `html`: a refused tag or attribute name is quoted and escaped in the error
  message, so it cannot inject lines into a log.
- A thrown module struct keeps its `alias.Struct` identity. (M28.56)
- A module's function passed as a func value to a higher-order builtin
  (`lists.map`, ...) now runs in its module (it failed with `unknown struct`),
  and a struct thrown through a module func value keeps its module identity.
- `archive.unpack`: a member's declared size no longer preallocates beyond what
  the input can hold, so a tiny archive declaring a huge member fails without a
  large allocation.
- `lint`: a parameter that shares a top-level variable's name is typed as the
  parameter (a false `L110`).
- A module struct field typed with the module's own enum accepts that enum
  from the importing program.
- A timed-out `term.readByteTimeout` raced a later `io.readLine`; `term` and
  `io` now share one stdin reader. (M28.38)
- Deeply nested values crashed `jennifer-tiny` and overflowed the default
  binary's stack. (M28.37)
- Markdown PDF output put a space between inline markup and adjacent
  punctuation. (M28.77)
- `jennifer fmt` read the `-` after `]` as unary minus; `jennifer profile`
  counted `CUM` more than once for recursion and module calls.
- `smtp` normalises bare CR / LF before dot-stuffing (SMTP smuggling).
- `http` resolves a relative `Location` per RFC 3986 and closes a keep-alive
  socket after a failed `exchange`; a positive `timeoutMs` now also bounds the
  connect (it was always 30 s); a `Session` closes on `Connection: close` and
  on HTTP/1.0 without keep-alive; an uncapped body over 256 MiB is read in
  pieces.
- `yaml.encode` double-quotes the multi-line strings it wrote as a block it
  could not read back (a leading line break was lost).
- `mcp.serveStdio` answers a non-UTF-8 line with a parse error instead of
  exiting; `pop` CAPA no longer hides a close, timeout or cap; `oauth`
  compares `state` in constant time; `jwt` errors no longer echo the raw
  `kid`; `multipart.buildWith` quotes a non-token boundary; `s3` decodes XML
  entities in one pass.
- `markdown` parses long `*` / `_` runs in linear time (they were quadratic);
  `pdf.bookmark` / `addFont` / `addImage` no longer copy the whole document.
- `webapi.guard` checked a `HEAD` request against no route (letting it
  through) or another route's spec; it now uses the `GET` route `web` serves.
- `httpd.serveFile` listed a directory without an `index.html` even with
  `dirListing` off; `setHeader` refuses `Transfer-Encoding`, and `respond` a
  `Content-Length` that does not match the body.
- `ansi` nested wrappers lost the outer style; `kvstore` `incrWindow` was not
  atomic on redis; `flatdb` did not fsync before rename; `oauth.save` wrote the
  token before restricting the file to `0600`.
- `cron.next` missed schedules more than five years ahead; `ntp` misread
  timestamps past the 2036 rollover; `crypto.verify` threw on an Ed25519
  signature of the wrong length.
- `mikrotik` missed a cancelled stream's `!trap`; `mactelnet` leaked its
  socket on a timeout. (M28.74)
