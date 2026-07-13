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

The **TinyGo** column says whether a module runs on the constrained
`jennifer-tiny` binary. A `No` module needs the default `jennifer` binary: it
uses `net`, and the stock `jennifer-tiny` ships no network stack. A `Partial`
module runs on both binaries except for its network-backed calls, which need the
default binary. Every module
name links to its reference doc under [`docs/modules/`](../docs/modules/index.md);
a runnable demo for each lives in [`examples/modules/`](../examples/modules/).

| Module | TinyGo | Description |
| ------ | ------ | ----------- |
| [`ansi.j`](../docs/modules/ansi.md) | Yes | Terminal styling as explicit string wrappers: `color` / `bgColor` / `style` / `rgb` / `strip` plus per-colour and per-style shortcuts (`ansi.red`, `ansi.bold`, ...). TTY-aware: suppresses itself off a terminal or under `NO_COLOR`, forced on by `FORCE_COLOR`. |
| [`csv.j`](../docs/modules/csv.md) | Yes | RFC 4180 comma-separated values: `parse` / `format` (`parseWith` / `formatWith` for any delimiter, TSV too), and `toRecords` / `fromRecords` for header-keyed `map of string to string`. Quoting-aware. |
| [`docblock.j`](../docs/modules/docblock.md) | Yes | The Jennifer doc-comment format and its parser. `docblock.parse(source)` returns a typed `FileDoc` tree (module preamble, per-construct docs); reports doc drift (a `@param` / `@field` naming nothing real, a parameter with no `@param`) and orphaned comments as `Diagnostic`s. Over `regex` / `strings`. |
| [`flatdb.j`](../docs/modules/flatdb.md) | Yes | A file-backed JSON document store over `json` + `fs`: `open` a file into a value-semantic `DB`, query / edit by JSON Pointer (`get` / `has` / `keys` / `length`; fresh-`DB` `set` / `append` / `remove`), `save` with a crash-atomic temp + `rename`. Not a database engine - snapshotting of small data. |
| [`htmlwriter.j`](../docs/modules/htmlwriter.md) | Yes | Build an HTML element tree and render escaped HTML5: `element` / `text` / `raw` / `attr` constructors, `render` / `renderAll`, `escape`. Void-element aware. A writer, not a parser. |
| [`http.j`](../docs/modules/http.md) | No | An HTTP/1.1 client over `net` (`https://` via TLS): `request` plus `get` / `post` / `put` / `patch` / `delete` / `head` / `options` -> `Response` (`status`, `headers`, `body`), with `header` case-insensitive read. Content-Length + chunked; redirects returned, not followed. |
| [`gotify.j`](../docs/modules/gotify.md) | No | Push a notification to a [Gotify](https://gotify.net) server, on top of `http`: `push(cfg, title, message, priority)` POSTs the message form with the `X-Gotify-Key` header. Value-semantic `Config` (url + token), caller-supplied. |
| [`gpio.j`](../docs/modules/gpio.md) | Yes | Raspberry-Pi (and any Linux SBC) GPIO over sysfs, with `fs` as the backend. Stateless, pin-keyed: `setup(pin, "in"/"out")`, `write(pin, 0/1)`, `read(pin)`, `release(pin)`. Root `/sys/class/gpio`, overridable via `JENNIFER_GPIO_BASE`. Off a GPIO host, calls throw `Error{kind: "gpio"}` cleanly. |
| [`rest.j`](../docs/modules/rest.md) | No | An ergonomic REST layer over `http` + `json`: a value-semantic `Client` (base URL + headers) and `get` / `post` / `put` / `patch` / `delete` -> `Response`, plus `getJson` / `postJson` / `putJson` / `patchJson`. Base-URL joining, query strings, `bearer` / `basic` / `withHeader` auth. |
| [`oauth.j`](../docs/modules/oauth.md) | No | A generic OAuth2 client (the get-a-token half) over `http` + `json`: client-credentials, refresh-token, and device-authorization grants; `google` / `microsoft` presets; `isExpired` + on-disk token store. Tokens feed `sasl` XOAUTH2. |
| [`web.j`](../docs/modules/web.md) | No | A small HTTP framework over the `httpd` engine: register routes against handler methods by name (`get` / `post` / ...), `:param` capture, `before` middleware, `notFound`; a handler takes a `web.Context` (request accessors + response helpers), dispatched via `meta.callMain`. `web.run($app, addr)`; run with `jennifer serve app.j [--watch]`. |
| [`markdown.j`](../docs/modules/markdown.md) | Yes | Render a CommonMark subset to HTML (through `htmlwriter`) and to styled terminal text (through `ansi`) with `toHtml` / `toAnsi`; plus Markdown authoring helpers (`header` / `style` / `link` / `bullets` / `numbered` / `codeBlock` / `table`) and `tablePretty` to align table source. |
| [`memcache.j`](../docs/modules/memcache.md) | No | A memcached client (classic text protocol) over `net`: `set` / `add` / `get` / `delete` / `incr` / `decr` / `touch`, every store with a TTL. A volatile cache (for caches, sessions, counters, locks), not a system of record. |
| [`session.j`](../docs/modules/session.md) | No | Server-side sessions on `memcache`: a `map of string to string` under `sess:ID` with a sliding TTL. `create` / `load` / `save` / `touch` / `destroy`; UUID v4 ids, base64-wrapped JSON values so any UTF-8 round-trips. Volatile. |
| [`ratelimit.j`](../docs/modules/ratelimit.md) | No | A fixed-window rate limiter on `memcache` (atomic `incr` + per-key TTL): `allow(mc, key, limit, window)` -> bool, `remaining(mc, key, limit)`. The window resets on its own when it expires. |
| [`mime.j`](../docs/modules/mime.md) | Yes | Build and parse MIME messages (RFC 5322 headers, multipart, quoted-printable / base64, RFC 2047 encoded-words for non-ASCII headers): `text` / `attachment` / `multipart` / `encode` / `parse` (+ `headerValue` / `body` / `parts` / `contentType`). No networking; the foundation the mail clients build on. |
| [`sasl.j`](../docs/modules/sasl.md) | Yes | Crypto-free SASL auth mechanisms as base64 encoders, shared by the mail clients: `plain` / `loginUser` / `loginPass` / `bearer` (SASL XOAUTH2 - the use-a-token half of OAuth2). No networking, no crypto. |
| [`semver.j`](../docs/modules/semver.md) | Yes | Strict Semantic Versioning 2.0.0: `parse` / `isValid` / `toString`, `compare` / `lt` / `eq` / `gt`, `isStable` / `isPrerelease`, `incMajor` / `incMinor` / `incPatch`, `sort`; an exported `Version` struct. |
| [`smtp.j`](../docs/modules/smtp.md) | No | Send mail (SMTP client) over `net`: `send(opts, from, recipients, message)` runs the RFC 5321 dialogue (EHLO, optional STARTTLS / implicit TLS, `AUTH PLAIN`, `MAIL FROM` / `RCPT TO` / `DATA`); message built by `mime`. Throws `Error{kind: "smtp"}` on rejection. |
| [`imap.j`](../docs/modules/imap.md) | No | Receive mail (IMAP4rev1, RFC 3501) over `net`, a reading subset: `connect` -> `Session`, then `selectMailbox` / `search` / `fetch` / `logout`, plus `fetchAll`. Handles tagged responses and `{N}` literals; messages are strings for `mime.parse`. |
| [`idna.j`](../docs/modules/idna.md) | Yes | Internationalized domain names: `toAscii` / `toUnicode` over a Punycode (RFC 3492) core (`münchen.de` <-> `xn--mnchen-3ya.de`), plus `isAscii`. No networking; the mail clients IDNA-encode hosts and envelope domains through it. |
| [`pop.j`](../docs/modules/pop.md) | No | Receive mail (POP3, RFC 1939) over `net`: `connect` opens a session, then `stat` / `count` / `sizes` / `retrieve` / `deleteMessage` / `quit`, plus `fetchAll`. Messages are strings for `mime.parse`; throws `Error{kind: "pop3"}` on `-ERR`. |
| [`redis.j`](../docs/modules/redis.md) | No | A Redis client speaking RESP2 over `net`: typed helpers `get` / `set` / `del` / `exists` / `incr` / `decr` / `keys` / `ping`, plus a generic `command(session, args)` -> `Reply` for anything else. `connect` does optional `AUTH` / `SELECT`; a `-ERR` throws `Error{kind: "redis"}`. |
| [`resque.j`](../docs/modules/resque.md) | No | Background jobs on Redis, wire-compatible with Resque: `enqueue` onto named queues, `reserve` the next `Job` in priority order, plus `queueLength` / `queues` / `size` / `fail`. Interops with Ruby-resque / php-resque workers. Built on `redis` + `json`. |
| [`mqtt.j`](../docs/modules/mqtt.md) | No | An MQTT 3.1.1 pub/sub client over `net` (`mqtts` via TLS): `connect` -> `Client`, then `subscribe` / `publish` / `publishBytes` (QoS 0), blocking `receive` and `poll(client, timeoutMs)` (single-threaded, via `net.setDeadline`), `ping`, `disconnect`. Binary packet framing built with bitwise ops + `bytes`. |
| [`label.j`](../docs/modules/label.md) | Partial | Industrial label printing in a build / render / emit pipeline. Build a device-independent `Label` in millimetres (`new` / `text` / `barcode` / `box` / `image` / `quantity`; barcodes code128 / ean13 / itf / code39 / gs1-128 / datamatrix / qr), `render(label, device)` to a selectable dialect (`"zpl"` Zebra, `"cab"` cab JScript), then `send(host, port, rendered)` to a printer's raw `:9100` port. Build / render run on both binaries; `send` needs the default binary. |
| [`prometheus.j`](../docs/modules/prometheus.md) | Partial | Prometheus metrics in two halves. **Exposition** (`counter` / `gauge` / `observe` / `render`) builds a metric set and renders the text format - pure text, both binaries. **Retrieval** (`query` / `queryRange` -> `Result`) is a read client for the HTTP query API over `http` + `json`, so it needs the default binary. Strict name / label validation and escaping. |

A new module also earns a bullet in the **Module library** section of
[`JENNIFER.md`](../JENNIFER.md) so an AI assistant writing Jennifer discovers it.
