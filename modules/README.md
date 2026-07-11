# Jennifer modules

Jennifer-coded library modules (`.j` files) live here. Unlike the Go
system libraries (`internal/lib/*`, enabled with `use NAME;`), a module is
distributable Jennifer source, brought in with `import "NAME.j" as NAME;`.

Distribution packages install these to the system module directory
(`/usr/share/jennifer/modules/` by default; see `jennifer version -v`),
so `import "NAME.j";` resolves without a path. Local modules resolve with
`import "./NAME.j";`, and extra search directories are added with
`jennifer run -I DIR ...`.

## Available modules

- **`ansi.j`** - terminal styling as explicit string wrappers.
  `ansi.color(s, name)` / `ansi.bgColor(s, name)` / `ansi.style(s, name)`
  (bold / dim / italic / underline / reverse) / `ansi.rgb(s, r, g, b)`,
  `ansi.strip(s)` to remove escapes, plus per-colour and per-style
  shortcuts (`ansi.red(s)`, `ansi.bold(s)`, ...). Stateless and TTY-aware:
  styling suppresses itself when stdout is not a terminal or `NO_COLOR` is
  set, and is forced on by `FORCE_COLOR`. See
  [`examples/modules/ansi_demo.j`](../examples/modules/ansi_demo.j).
- **`csv.j`** - RFC 4180 comma-separated values: parse text into rows of
  string fields and format rows back to text, with a quoting-aware scanner.
  `csv.parse(s)` / `csv.format(rows)` (and `parseWith` / `formatWith` for any
  single-character delimiter, so TSV too), plus `csv.toRecords(rows)` /
  `csv.fromRecords(header, records)` for header-keyed `map of string to
  string` records. Pure Jennifer over `strings` and `maps`. See
  [`examples/modules/csv_demo.j`](../examples/modules/csv_demo.j).
- **`htmlwriter.j`** - build an HTML element tree and render it to escaped
  HTML5. `html.element(tag, attrs, children)` / `html.text(s)` / `html.raw(s)`
  / `html.attr(name, value)` constructors, `html.render(node)` /
  `html.renderAll(nodes)`, and `html.escape(s)`; text and attribute values are
  escaped automatically, void elements (`br`, `img`, ...) render without a
  closing tag. A writer, not a parser; pure Jennifer over `strings` and
  `lists`. See
  [`examples/modules/htmlwriter_demo.j`](../examples/modules/htmlwriter_demo.j).
- **`http.j`** - an HTTP/1.1 client over `net` (`https://` via TLS):
  `http.request(method, url, headers, body)` (method-agnostic) and the
  `http.get` / `post` / `put` / `patch` / `delete` / `head` / `options`
  shortcuts return a `Response` (`status`, `statusText`,
  lowercased `headers`, `body`), with `http.header(resp, name)` for a
  case-insensitive read. Handles Content-Length and chunked framing; text
  (UTF-8) bodies. Redirects are returned (3xx), not followed. Uses `net`, so the
  **default `jennifer` binary only**. See
  [`examples/modules/http_demo.j`](../examples/modules/http_demo.j).
- **`markdown.j`** - render a small CommonMark subset (headings, bold /
  italic, inline code, links, fenced code blocks, ordered / unordered lists)
  to HTML and to styled terminal text. `markdown.toHtml(md)` renders through
  the `htmlwriter` module (so escaping is automatic); `markdown.toAnsi(md)`
  renders through the `ansi` module. Also authors Markdown text with
  `markdown.header` / `style` / `link` / `bullets` / `numbered` / `codeBlock`,
  and `markdown.table(headings, aligns, rows)` for GFM tables, plus
  `markdown.tablePretty(md)` to align handcrafted table columns.
  Pure Jennifer; the first module that imports sibling modules. See
  [`examples/modules/markdown_demo.j`](../examples/modules/markdown_demo.j).
- **`memcache.j`** - a memcached client over `net` speaking the classic text
  protocol: `memcache.set` / `add` (store-if-absent) / `get` / `delete` /
  `incr` / `decr` / `touch`, every store with a TTL (`exptime` seconds). A
  volatile cache (keys expire and the server evicts under pressure), so it
  suits caches, sessions, counters, and locks, not a system of record. A
  protocol error throws `Error` (kind `"memcache"`). Uses `net`, so the
  **default `jennifer` binary only**. See
  [`examples/modules/memcache_demo.j`](../examples/modules/memcache_demo.j).
- **`session.j`** - server-side sessions on the `memcache` module, the
  canonical memcached use: a `map of string to string` held under `sess:ID`
  with a sliding TTL. `session.create(mc, ttl)` -> id (UUID v4), `load(mc, id)`
  (empty map when absent / expired), `save(mc, id, data, ttl)`, `touch(mc, id,
  ttl)`, `destroy(mc, id)`. Threads `memcache` + `uuid` + `json`; the data map
  is stored base64-wrapped JSON, so any UTF-8 value round-trips. Volatile (a
  cache, not a store of record). Uses `net` (via `memcache`), so the **default
  `jennifer` binary only**. See
  [`examples/modules/session_demo.j`](../examples/modules/session_demo.j).
- **`ratelimit.j`** - a fixed-window rate limiter on the `memcache` module, the
  sharpest use of memcached's atomic `incr` + per-key TTL:
  `ratelimit.allow(mc, key, limit, window)` -> bool records a hit and reports
  whether it is within `limit` for the current `window` (seconds);
  `ratelimit.remaining(mc, key, limit)` reports the budget left. The counter is
  created with the window TTL on the first hit and expires on its own; the
  incr-then-add pair closes the create race. Uses `net` (via `memcache`), so the
  **default `jennifer` binary only**. See
  [`examples/modules/ratelimit_demo.j`](../examples/modules/ratelimit_demo.j).
- **`mime.j`** - build and parse MIME messages (RFC 5322 headers, multipart,
  quoted-printable / base64 transfer encodings). `mime.text` / `attachment` /
  `multipart` / `withHeader` build a `Part` tree, `mime.encode` serializes it,
  `mime.parse` reads it back, and `mime.headerValue` / `body` / `parts` /
  `contentType` / `address` read it. A non-ASCII `Subject` / display name is
  auto-encoded as an RFC 2047 encoded-word on `encode` and decoded on `parse`
  (`mime.encodeWord` / `decodeWord` exposed for manual use). Pure Jennifer over
  `strings` / `convert` / `encoding` / `regex`; no networking, so it is the
  foundation the mail clients build on. See
  [`examples/modules/mime_demo.j`](../examples/modules/mime_demo.j).
- **`sasl.j`** - the crypto-free SASL auth mechanisms as pure base64 encoders,
  shared by the mail clients: `sasl.plain(user, pass)`, `sasl.loginUser` /
  `sasl.loginPass`, and `sasl.bearer(user, token)` (SASL XOAUTH2 - the
  "use a token" half of OAuth2, how Google / Microsoft 365 authenticate mail).
  No networking, no crypto (SCRAM / CRAM-MD5 join it with the `crypto` library).
  Consumed by `smtp` / `pop` / `imap` via `Options.auth = "xoauth2"`.
- **`semver.j`** - strict Semantic Versioning 2.0.0: parse, compare, sort,
  and increment version numbers. `semver.parse(s)` / `isValid(s)` /
  `toString(v)`, `compare(a, b)` / `lt` / `eq` / `gt`, `isStable(v)` /
  `isPrerelease(v)`, `incMajor` / `incMinor` / `incPatch(v)`, and
  `sort(vs)`, over an exported `Version` struct. Pure Jennifer; parsing
  uses the canonical SemVer regex, precedence and sort are hand-written.
  See [`examples/modules/semver_demo.j`](../examples/modules/semver_demo.j).

- **`smtp.j`** - send mail (SMTP client) over `net`: `smtp.send(opts, from,
  recipients, message)` runs the RFC 5321 dialogue (EHLO, optional STARTTLS /
  implicit TLS, `AUTH PLAIN`, `MAIL FROM` / `RCPT TO` / `DATA`), with the
  message built by `mime`. Throws a catchable `Error` (kind `"smtp"`) on
  rejection. Uses `net`, so the **default `jennifer` binary only**
  (`jennifer-tiny` has no network stack). See
  [`examples/modules/smtp_demo.j`](../examples/modules/smtp_demo.j).

- **`imap.j`** - receive mail (IMAP4rev1, RFC 3501) over `net`, a reading
  subset: `imap.connect(opts)` -> `Session`, then `selectMailbox(name)` (message
  count), `search()` (sequence numbers), `fetch(n)` (a whole message), `logout`,
  with `fetchAll(opts, mailbox)` for the common case. Handles tagged responses
  and `{N}` literals; retrieved messages are strings for `mime.parse`. Throws
  `Error` (kind `"imap"`) on `NO` / `BAD`. Uses `net`, so the **default
  `jennifer` binary only**. See
  [`examples/modules/imap_demo.j`](../examples/modules/imap_demo.j).
- **`idna.j`** - internationalized domain names: `idna.toAscii(domain)` /
  `idna.toUnicode(domain)` over a Punycode (RFC 3492) core
  (`münchen.de` <-> `xn--mnchen-3ya.de`), plus `idna.isAscii`. Pure Jennifer
  over `strings` / `convert` / `encoding` (uses `convert.toCodepoint` /
  `fromCodepoint`); no networking. The mail clients IDNA-encode hosts and SMTP
  envelope domains through it. See
  [`examples/modules/idna_demo.j`](../examples/modules/idna_demo.j).
- **`pop.j`** - receive mail (POP3, RFC 1939) over `net`: `pop.connect(opts)`
  opens a session, then `stat` / `count` / `sizes` / `retrieve(n)` /
  `deleteMessage(n)` / `quit`, with `fetchAll(opts)` for the common "get every
  message" case. Retrieved messages are strings for `mime.parse`. Named `pop`
  (a namespace can't hold a digit); throws `Error` (kind `"pop3"`) on `-ERR`.
  Uses `net`, so the **default `jennifer` binary only**. See
  [`examples/modules/pop_demo.j`](../examples/modules/pop_demo.j).
- **`redis.j`** - a Redis client speaking RESP2 over `net`: commands go out as
  RESP arrays of bulk strings, replies (`+OK` / `-ERR` / `:int` / `$bulk` /
  `*array`) parse into a `Reply`. Typed helpers `redis.get` / `set` / `del` /
  `exists` / `incr` / `decr` / `keys` / `ping`, plus a generic
  `redis.command(session, args)` returning the raw `Reply` (walked like a
  `json.Value`) for anything without a helper. `connect` does optional `AUTH` /
  `SELECT`; a `-ERR` reply throws `Error` (kind `"redis"`). Uses `net`, so the
  **default `jennifer` binary only**. See
  [`examples/modules/redis_demo.j`](../examples/modules/redis_demo.j).
- **`resque.j`** - background jobs on Redis, wire-compatible with Resque:
  schedule work onto named queues and process it from a worker later, over the
  `redis` module. `resque.enqueue(session, queue, class, args)` registers the
  queue and pushes the JSON envelope `{"class","args"}` onto `resque:queue:NAME`;
  `resque.reserve(session, queues)` pops the next `Job` (`queue` / `class` /
  `args`) from the first non-empty queue in priority order; plus `queueLength` /
  `queues` / `size` / `fail`. The Redis layout is the de-facto Resque standard,
  so a Ruby-resque / php-resque worker can process Jennifer's jobs and vice
  versa; the worker's class dispatch is your code. Built on `redis` + `json`, so
  the **default `jennifer` binary only**. See
  [`examples/modules/resque_demo.j`](../examples/modules/resque_demo.j).

Reference docs for each module live under
[`docs/modules/`](../docs/modules/index.md). A new module also earns a bullet
in the **Module library** section of [`JENNIFER.md`](../JENNIFER.md) so an AI
assistant writing Jennifer discovers it.
